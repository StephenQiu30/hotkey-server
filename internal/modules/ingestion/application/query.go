package application

import (
	"context"
	"errors"
	"fmt"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// ContentQueryDependencies explicitly joins only ingestion-owned Content
// reads with Source's safe application read port. No transport or repository
// reaches across into source_connections directly.
type ContentQueryDependencies struct {
	Contents ingestiondomain.ContentRepository
	Sources  sourcedomain.ContentSourceReader
}

// ContentQueryService exposes the active Content read use cases consumed by
// the HTTP transport. It has no object-store dependency, so evidence object
// keys and provider credentials cannot enter its result model.
type ContentQueryService struct {
	contents ingestiondomain.ContentRepository
	sources  sourcedomain.ContentSourceReader
}

func NewContentQueryService(dependencies ContentQueryDependencies) (*ContentQueryService, error) {
	if dependencies.Contents == nil || dependencies.Sources == nil {
		return nil, errors.New("content query dependencies are required")
	}
	return &ContentQueryService{contents: dependencies.Contents, sources: dependencies.Sources}, nil
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
