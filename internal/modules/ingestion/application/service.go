package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

const (
	defaultIngestRunLimit = 50
	maximumIngestRunLimit = 200
	unknownIngestionCode  = "ingestion_failed"
	evidenceLockNamespace = "hotkey.ingestion.evidence/v1"
)

// Dependencies contains only the persisted Source capture boundary and
// ingestion-owned persistence/object-store ports. In particular, Service has
// no Connector dependency and cannot initiate another upstream fetch.
type Dependencies struct {
	Runtime  *database.Runtime
	Captures sourcedomain.CapturedItemReader
	Contents ingestiondomain.ContentRepository
	Evidence ingestiondomain.EvidenceStore
}

// Service independently processes durable captured items. Object bytes are
// written before the database transaction; the Content asset and Source bind
// are then made atomic in one Runtime transaction.
type Service struct {
	runtime  *database.Runtime
	captures sourcedomain.CapturedItemReader
	contents ingestiondomain.ContentRepository
	evidence ingestiondomain.EvidenceStore
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Runtime == nil || dependencies.Captures == nil || dependencies.Contents == nil || dependencies.Evidence == nil {
		return nil, errors.New("ingestion application dependencies are required")
	}
	return &Service{runtime: dependencies.Runtime, captures: dependencies.Captures, contents: dependencies.Contents, evidence: dependencies.Evidence}, nil
}

// IngestRun processes one bounded page of Source-owned captures. A failed
// item is classified on that item and does not prevent later captures in the
// same run from progressing.
type IngestRunInput struct {
	RunID int64
	Limit int
}

type IngestRunResult struct {
	Processed  int
	Bound      int
	Failed     int
	Uploaded   int
	NextCursor string
}

func (service *Service) IngestRun(ctx context.Context, input IngestRunInput) (IngestRunResult, error) {
	if service == nil || service.runtime == nil || service.captures == nil || service.contents == nil || service.evidence == nil {
		return IngestRunResult{}, errors.New("ingestion service is not initialized")
	}
	if input.RunID <= 0 {
		return IngestRunResult{}, errors.New("ingestion run id is required")
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultIngestRunLimit
	}
	if limit < 1 || limit > maximumIngestRunLimit {
		return IngestRunResult{}, fmt.Errorf("ingestion run limit must be between 1 and %d", maximumIngestRunLimit)
	}

	page, err := service.captures.ListUnboundCaptured(ctx, sourcedomain.CapturedItemQuery{RunID: input.RunID, Limit: limit})
	if err != nil {
		return IngestRunResult{}, fmt.Errorf("list unbound captured items: %w", err)
	}
	result := IngestRunResult{NextCursor: page.NextCursor}
	for _, captured := range page.Items {
		result.Processed++
		uploaded, err := service.ingestCaptured(ctx, captured)
		if err == nil {
			result.Bound++
			if uploaded {
				result.Uploaded++
			}
			continue
		}
		failure := sourcedomain.CapturedIngestionFailure{
			CollectionItemID: captured.ID, RunID: captured.RunID, SourceConnectionID: captured.SourceConnectionID,
			Code: ingestionFailureCode(err),
		}
		if markErr := service.captures.MarkIngestionFailure(ctx, failure); markErr != nil {
			return result, fmt.Errorf("mark ingestion failure for captured item %d: %w", captured.ID, markErr)
		}
		result.Failed++
	}
	return result, nil
}

func (service *Service) ingestCaptured(ctx context.Context, captured sourcedomain.CapturedCollectionItem) (bool, error) {
	content, err := NormalizeCapturedItem(captured.Item, captured.SourceConnectionID)
	if err != nil {
		return false, err
	}
	candidates, err := service.contentCandidates(ctx)
	if err != nil {
		return false, err
	}
	decision, err := DecideDuplicate(content, candidates)
	if err != nil {
		return false, err
	}
	if content.Body == "" {
		return service.persistCaptured(ctx, captured, content, decision)
	}
	var uploaded bool
	err = service.withSourceEvidenceLock(ctx, content.SourceConnectionID, func(lockedCtx context.Context) error {
		var persistErr error
		uploaded, persistErr = service.persistCaptured(lockedCtx, captured, content, decision)
		return persistErr
	})
	if err != nil {
		return false, err
	}
	return uploaded, nil
}

func (service *Service) persistCaptured(ctx context.Context, captured sourcedomain.CapturedCollectionItem, content ingestiondomain.NormalizedContent, decision ingestiondomain.DedupeDecision) (bool, error) {

	var receipt ingestiondomain.EvidenceReceipt
	hasEvidence := content.Body != ""
	createEvidenceAsset := false
	if hasEvidence {
		object := evidenceObject(content)
		known, err := service.assetObjectKnown(ctx, content.SourceConnectionID, object.ObjectKey)
		if err != nil {
			return false, err
		}
		receipt, err = service.evidence.PutText(ctx, object)
		if err != nil {
			return false, fmt.Errorf("put evidence: %w", err)
		}
		createEvidenceAsset = !known
	}

	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		stored, _, err := service.contents.Upsert(transactionCtx, content, decision)
		if err != nil {
			return fmt.Errorf("upsert normalized content: %w", err)
		}
		if createEvidenceAsset {
			asset := ingestiondomain.ContentAsset{
				ContentID: stored.ID, AssetType: "text", ObjectKey: receipt.ObjectKey, OriginalURL: content.CanonicalURL,
				MIMEType: "text/plain; charset=utf-8", SHA256: receipt.SHA256, SizeBytes: receipt.SizeBytes,
				CapturedAt: content.FetchedAt, Status: ingestiondomain.AssetStatusAvailable,
			}
			if err := service.contents.CreateAsset(transactionCtx, asset); err != nil {
				return fmt.Errorf("create content evidence asset: %w", err)
			}
		}
		binding := sourcedomain.CapturedContentBinding{
			CollectionItemID: captured.ID, RunID: captured.RunID, SourceConnectionID: captured.SourceConnectionID, ContentID: stored.ID,
		}
		if err := service.captures.BindContent(transactionCtx, binding); err != nil {
			return fmt.Errorf("bind captured content: %w", err)
		}
		return nil
	})
	if err != nil {
		if createEvidenceAsset && service.shouldCompensateEvidence(ctx, content.SourceConnectionID, receipt.ObjectKey) {
			service.compensateEvidence(ctx, receipt.ObjectKey)
		}
		return false, err
	}
	return hasEvidence, nil
}

func (service *Service) assetObjectKnown(ctx context.Context, sourceConnectionID int64, objectKey string) (bool, error) {
	keys, err := service.contents.ListAssetObjectKeys(ctx, sourceConnectionID)
	if err != nil {
		return false, fmt.Errorf("list known evidence assets: %w", err)
	}
	for _, key := range keys {
		if key == objectKey {
			return true, nil
		}
	}
	return false, nil
}

func (service *Service) shouldCompensateEvidence(ctx context.Context, sourceConnectionID int64, objectKey string) bool {
	known, err := service.assetObjectKnown(ctx, sourceConnectionID, objectKey)
	return err == nil && !known
}

func (service *Service) contentCandidates(ctx context.Context) ([]ingestiondomain.ContentCandidate, error) {
	const pageLimit = 200
	candidates := make([]ingestiondomain.ContentCandidate, 0)
	cursor := ""
	for {
		page, err := service.contents.ListActive(ctx, ingestiondomain.ContentListQuery{Cursor: cursor, Limit: pageLimit})
		if err != nil {
			return nil, fmt.Errorf("list duplicate candidates: %w", err)
		}
		for _, content := range page.Items {
			candidates = append(candidates, ingestiondomain.ContentCandidate{
				ID: content.ID, SourceConnectionID: content.SourceConnectionID, PublishedAt: content.PublishedAt,
				TitleTokens: tokenize(content.Title), BodyTokens: tokenize(content.Excerpt), CanonicalURL: content.CanonicalURL,
				DedupeKey: content.ContentHash, Completeness: contentCompleteness(content), SourceExternalIDStable: true,
			})
		}
		if page.NextCursor == "" {
			return candidates, nil
		}
		cursor = page.NextCursor
	}
}

func contentCompleteness(content ingestiondomain.Content) int {
	complete := 0
	for _, value := range []string{content.Title, content.Excerpt, content.CanonicalURL, content.Language, content.Author.DisplayName} {
		if strings.TrimSpace(value) != "" {
			complete++
		}
	}
	return complete
}

func evidenceObject(content ingestiondomain.NormalizedContent) ingestiondomain.EvidenceObject {
	digest := sha256.Sum256([]byte(content.Body))
	sha := hex.EncodeToString(digest[:])
	return ingestiondomain.EvidenceObject{
		SourceConnectionID: content.SourceConnectionID,
		ObjectKey:          fmt.Sprintf("evidence/v1/%d/%s/%s.txt", content.SourceConnectionID, sha[:2], sha),
		Text:               content.Body,
		SHA256:             sha,
	}
}

func ingestionFailureCode(err error) string {
	if code, found := ingestiondomain.ErrorCodeOf(err); found {
		return string(code)
	}
	return unknownIngestionCode
}

// compensateEvidence immediately removes an object whose database reference
// rolled back. A temporary delete outage intentionally leaves an unreferenced
// object; ReconcileObjects derives durable asset references before deleting it.
func (service *Service) compensateEvidence(ctx context.Context, objectKey string) {
	_ = service.evidence.Delete(ctx, objectKey)
}

// ReconcileObjects removes unreferenced evidence objects for one source while
// preserving every non-deleted durable asset reference. It intentionally does
// not change Content or asset lifecycle state; the follow-on lifecycle slice
// owns those commands.
func (service *Service) ReconcileObjects(ctx context.Context, sourceConnectionID int64) (int, error) {
	if service == nil || service.evidence == nil || service.contents == nil || sourceConnectionID <= 0 {
		return 0, errors.New("content repository, evidence store, and source connection id are required")
	}
	deleted := 0
	err := service.withSourceEvidenceLock(ctx, sourceConnectionID, func(lockedCtx context.Context) error {
		prefix := fmt.Sprintf("evidence/v1/%d/", sourceConnectionID)
		keys, err := service.contents.ListAssetObjectKeys(lockedCtx, sourceConnectionID)
		if err != nil {
			return fmt.Errorf("list durable evidence assets: %w", err)
		}
		known := make(map[string]struct{}, len(keys))
		for _, key := range keys {
			known[key] = struct{}{}
		}
		receipts, err := service.evidence.ListPrefix(lockedCtx, prefix)
		if err != nil {
			return fmt.Errorf("list evidence objects: %w", err)
		}
		for _, receipt := range receipts {
			if _, found := known[receipt.ObjectKey]; found {
				continue
			}
			if err := service.evidence.Delete(lockedCtx, receipt.ObjectKey); err != nil {
				return fmt.Errorf("delete orphan evidence %q: %w", receipt.ObjectKey, err)
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		return deleted, err
	}
	return deleted, nil
}

// withSourceEvidenceLock serializes source-scoped object writes with object
// reconciliation. The lock is session-scoped, so it is held by a dedicated
// acquired pool connection while the ordinary Runtime transaction remains
// free to use its existing SQL callback context.
func (service *Service) withSourceEvidenceLock(ctx context.Context, sourceConnectionID int64, fn func(context.Context) error) (returned error) {
	if service == nil || service.runtime == nil || service.runtime.Pool == nil || sourceConnectionID <= 0 || fn == nil {
		return errors.New("database runtime, source connection id, and lock callback are required")
	}
	connection, err := service.runtime.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire evidence advisory-lock session: %w", err)
	}
	lockName := fmt.Sprintf("%s/%d", evidenceLockNamespace, sourceConnectionID)
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0::bigint))`, lockName); err != nil {
		connection.Release()
		return fmt.Errorf("acquire source evidence advisory lock: %w", err)
	}
	defer func() {
		var unlocked bool
		unlockErr := connection.QueryRow(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0::bigint))`, lockName).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			_ = connection.Conn().Close(context.Background())
			if returned == nil {
				if unlockErr != nil {
					returned = fmt.Errorf("release source evidence advisory lock: %w", unlockErr)
				} else {
					returned = errors.New("release source evidence advisory lock: lock was not held")
				}
			}
		}
		connection.Release()
	}()
	return fn(ctx)
}
