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
	defaultIngestRunLimit  = 50
	maximumIngestRunLimit  = 200
	unknownIngestionCode   = "ingestion_failed"
	evidenceReceiptRetries = 2
)

var errEvidenceReceiptMissing = errors.New("evidence receipt disappeared before asset transaction")

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
	return service.persistCaptured(ctx, captured, content, decision)
}

func (service *Service) persistCaptured(ctx context.Context, captured sourcedomain.CapturedCollectionItem, content ingestiondomain.NormalizedContent, decision ingestiondomain.DedupeDecision) (bool, error) {
	if content.Body == "" {
		return false, service.persistContent(ctx, captured, content, decision, ingestiondomain.EvidenceReceipt{}, false)
	}
	object := evidenceObject(content)
	for attempt := 0; attempt < evidenceReceiptRetries; attempt++ {
		receipt, err := service.evidence.PutText(ctx, object)
		if err != nil {
			return false, fmt.Errorf("put evidence: %w", err)
		}
		err = service.persistContent(ctx, captured, content, decision, receipt, true)
		if errors.Is(err, errEvidenceReceiptMissing) {
			continue
		}
		if err != nil {
			service.compensateEvidence(ctx, content.SourceConnectionID, receipt.ObjectKey)
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("evidence receipt remained unavailable after %d object writes: %w", evidenceReceiptRetries, errEvidenceReceiptMissing)
}

func (service *Service) persistContent(ctx context.Context, captured sourcedomain.CapturedCollectionItem, content ingestiondomain.NormalizedContent, decision ingestiondomain.DedupeDecision, receipt ingestiondomain.EvidenceReceipt, hasEvidence bool) error {
	return service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		// The same source-scoped lock serializes a delete tombstone with every
		// Content upsert, asset write, and Source bind. It is required even for
		// title-only captures, which have no EvidenceStore operation.
		if err := lockSourceEvidenceTransaction(transactionCtx, transaction, content.SourceConnectionID); err != nil {
			return err
		}
		createEvidenceAsset := false
		if hasEvidence {
			available, err := service.receiptAvailable(transactionCtx, content.SourceConnectionID, receipt)
			if err != nil {
				return err
			}
			if !available {
				return errEvidenceReceiptMissing
			}
			known, err := service.assetObjectKnown(transactionCtx, content.SourceConnectionID, receipt.ObjectKey)
			if err != nil {
				return err
			}
			createEvidenceAsset = !known
		}
		stored, _, err := service.contents.Upsert(transactionCtx, content, decision)
		if err != nil {
			return fmt.Errorf("upsert normalized content: %w", err)
		}
		if stored.Status == ingestiondomain.ContentStatusDeleted || stored.DeletedAt != nil {
			return ingestiondomain.NewError(ingestiondomain.ErrorCodeContentDeleted)
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

// compensateEvidence serializes the no-reference check with reconciliation
// before deleting an object from a failed Content/asset/bind transaction.
// Delete failures intentionally leave an orphan for ReconcileObjects.
func (service *Service) compensateEvidence(ctx context.Context, sourceConnectionID int64, objectKey string) {
	_ = service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
		if err := lockSourceEvidenceTransaction(transactionCtx, transaction, sourceConnectionID); err != nil {
			return err
		}
		known, err := service.assetObjectKnown(transactionCtx, sourceConnectionID, objectKey)
		if err != nil || known {
			return err
		}
		return service.evidence.Delete(transactionCtx, objectKey)
	})
}

func (service *Service) receiptAvailable(ctx context.Context, sourceConnectionID int64, receipt ingestiondomain.EvidenceReceipt) (bool, error) {
	receipts, err := service.evidence.ListPrefix(ctx, fmt.Sprintf("evidence/v1/%d/", sourceConnectionID))
	if err != nil {
		return false, fmt.Errorf("list evidence receipts: %w", err)
	}
	for _, current := range receipts {
		if current == receipt {
			return true, nil
		}
	}
	return false, nil
}

func lockSourceEvidenceTransaction(ctx context.Context, transaction database.Transaction, sourceConnectionID int64) error {
	if transaction.SQL == nil || sourceConnectionID <= 0 {
		return errors.New("transaction and source connection id are required for evidence lock")
	}
	if _, err := transaction.SQL.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0::bigint))`, fmt.Sprintf("hotkey.ingestion.evidence/v1/%d", sourceConnectionID)); err != nil {
		return fmt.Errorf("acquire source evidence transaction lock: %w", err)
	}
	return nil
}
