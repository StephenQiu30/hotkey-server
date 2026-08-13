package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// ContentQueryDependencies explicitly joins only ingestion-owned Content
// reads with Source's safe application read port. No transport or repository
// reaches across into source_connections directly.
type ContentQueryDependencies struct {
	Contents  ingestiondomain.ContentRepository
	Sources   sourcedomain.ContentSourceReader
	Evidence  ingestiondomain.EvidenceStore
	Lifecycle ContentLifecycle
}

// ContentLifecycle is the narrow mutation boundary consumed by the public
// Content management route. Keeping it separate from the read repository
// prevents HTTP from learning how source identities or evidence are stored.
type ContentLifecycle interface {
	DeleteContent(context.Context, int64) (DeleteBySourceItemResult, error)
}

// ContentQueryService exposes active Content reads and a verified evidence
// projection. Object-store keys and provider details never enter its result.
type ContentQueryService struct {
	contents  ingestiondomain.ContentRepository
	sources   sourcedomain.ContentSourceReader
	evidence  ingestiondomain.EvidenceStore
	lifecycle ContentLifecycle
}

func NewContentQueryService(dependencies ContentQueryDependencies) (*ContentQueryService, error) {
	if dependencies.Contents == nil || dependencies.Sources == nil {
		return nil, errors.New("content query dependencies are required")
	}
	return &ContentQueryService{contents: dependencies.Contents, sources: dependencies.Sources, evidence: dependencies.Evidence, lifecycle: dependencies.Lifecycle}, nil
}

func (service *ContentQueryService) DeleteContent(ctx context.Context, contentID int64) (DeleteBySourceItemResult, error) {
	if service == nil || service.lifecycle == nil {
		return DeleteBySourceItemResult{}, sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return service.lifecycle.DeleteContent(ctx, contentID)
}

const contentDocumentMaximumBytes int64 = 4 << 20

func (service *ContentQueryService) GetDocument(ctx context.Context, contentID int64) (ingestiondomain.ContentDocument, error) {
	content, err := service.GetActive(ctx, contentID)
	if err != nil {
		return ingestiondomain.ContentDocument{}, err
	}
	document := ingestiondomain.ContentDocument{
		ContentID: content.ID, Title: content.Title, SourceName: content.SourceName,
		CanonicalURL: content.CanonicalURL, Language: content.Language, PublishedAt: content.PublishedAt,
		Availability: ingestiondomain.ContentDocumentNotCaptured,
	}
	assets, err := service.contents.ListEvidenceAssets(ctx, content.SourceConnectionID, content.ID)
	if err != nil {
		return ingestiondomain.ContentDocument{}, contentQueryReadError(err)
	}
	markdownAssets := make([]ingestiondomain.ContentAsset, 0, len(assets))
	for _, asset := range assets {
		if asset.AssetType == "text" && asset.MIMEType == markdownMIMEType {
			markdownAssets = append(markdownAssets, asset)
		}
	}
	if len(markdownAssets) == 0 {
		return document, nil
	}
	sort.SliceStable(markdownAssets, func(left, right int) bool {
		leftAvailable := markdownAssets[left].Status == ingestiondomain.AssetStatusAvailable
		rightAvailable := markdownAssets[right].Status == ingestiondomain.AssetStatusAvailable
		if leftAvailable != rightAvailable {
			return leftAvailable
		}
		if markdownAssets[left].CapturedAt.Equal(markdownAssets[right].CapturedAt) {
			return markdownAssets[left].ID > markdownAssets[right].ID
		}
		return markdownAssets[left].CapturedAt.After(markdownAssets[right].CapturedAt)
	})
	asset := markdownAssets[0]
	if asset.Status != ingestiondomain.AssetStatusAvailable {
		document.Availability = ingestiondomain.ContentDocumentUnavailable
		document.UnavailableReason = contentDocumentAssetReason(asset.Status)
		return document, nil
	}
	if service.evidence == nil {
		document.Availability = ingestiondomain.ContentDocumentUnavailable
		document.UnavailableReason = ingestiondomain.ContentDocumentReasonReadFailed
		return document, nil
	}
	read, err := service.evidence.ReadText(ctx, asset.ObjectKey, contentDocumentMaximumBytes)
	if err != nil {
		document.Availability = ingestiondomain.ContentDocumentUnavailable
		document.UnavailableReason = ingestiondomain.ContentDocumentReasonReadFailed
		return document, nil
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(read.Text)))
	if read.Text == "" || read.MIMEType != markdownMIMEType || read.MIMEType != asset.MIMEType || read.SHA256 != asset.SHA256 || digest != asset.SHA256 || read.SizeBytes != asset.SizeBytes || read.SizeBytes != int64(len(read.Text)) {
		document.Availability = ingestiondomain.ContentDocumentUnavailable
		document.UnavailableReason = ingestiondomain.ContentDocumentReasonIntegrityFailed
		return document, nil
	}
	document.Availability = ingestiondomain.ContentDocumentReady
	document.UnavailableReason = ""
	document.Markdown = read.Text
	document.SHA256 = read.SHA256
	document.CapturedAt = asset.CapturedAt
	return document, nil
}

func contentDocumentAssetReason(status ingestiondomain.AssetStatus) ingestiondomain.ContentDocumentUnavailableReason {
	switch status {
	case ingestiondomain.AssetStatusPending:
		return ingestiondomain.ContentDocumentReasonPending
	case ingestiondomain.AssetStatusMissing:
		return ingestiondomain.ContentDocumentReasonMissing
	case ingestiondomain.AssetStatusDeletePending, ingestiondomain.AssetStatusDeleted:
		return ingestiondomain.ContentDocumentReasonDeleting
	default:
		return ingestiondomain.ContentDocumentReasonReadFailed
	}
}

func (service *ContentQueryService) ListActive(ctx context.Context, query ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error) {
	if service == nil || service.contents == nil || service.sources == nil {
		return ingestiondomain.ContentPage{}, sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	page, err := service.contents.ListActive(ctx, query)
	if err != nil {
		return ingestiondomain.ContentPage{}, contentQueryReadError(err)
	}
	items, err := service.withSources(ctx, page.Items, true)
	if err != nil {
		return ingestiondomain.ContentPage{}, err
	}
	page.Items = items
	return page, nil
}

func (service *ContentQueryService) GetActive(ctx context.Context, contentID int64) (ingestiondomain.Content, error) {
	if service == nil || service.contents == nil || service.sources == nil {
		return ingestiondomain.Content{}, sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	content, err := service.contents.GetActive(ctx, contentID)
	if err != nil {
		return ingestiondomain.Content{}, contentQueryReadError(err)
	}
	items, err := service.withSources(ctx, []ingestiondomain.Content{content}, false)
	if err != nil {
		return ingestiondomain.Content{}, err
	}
	return items[0], nil
}

// withSources enriches Content through Source's safe application port. List
// reads skip a source that was deleted or cannot be read, so one stale source
// cannot turn an otherwise valid cursor page into a 404/503. A detail read
// instead preserves a not-found/unavailable result for that exact Content.
func (service *ContentQueryService) withSources(ctx context.Context, contents []ingestiondomain.Content, skipUnavailable bool) ([]ingestiondomain.Content, error) {
	items := make([]ingestiondomain.Content, 0, len(contents))
	references := make(map[int64]sourcedomain.ContentSourceReference, len(contents))
	for _, content := range contents {
		reference, found := references[content.SourceConnectionID]
		if !found {
			var err error
			reference, err = service.sources.FindForContent(ctx, content.SourceConnectionID)
			if err != nil {
				readErr := contentQueryReadError(err)
				if skipUnavailable && skippableSourceReadError(readErr) {
					continue
				}
				return nil, readErr
			}
			references[content.SourceConnectionID] = reference
		}
		if reference.Deleted {
			if skipUnavailable {
				continue
			}
			return nil, sharederrors.New(sharederrors.CodeNotFound, 404, "")
		}
		content.SourceType, content.SourceName = reference.SourceType, reference.Name
		items = append(items, content)
	}
	return items, nil
}

func skippableSourceReadError(err error) bool {
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) {
		return false
	}
	return appError.Code == sharederrors.CodeNotFound ||
		appError.Code == sharederrors.CodeUnavailable ||
		appError.Code == sharederrors.CodeSourceConnectionUnavailable
}

func contentQueryReadError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, 404, "")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	default:
		return fmt.Errorf("read active content: %w", err)
	}
}
