package application

import (
	"context"
	"errors"
	stdhttp "net/http"
	"testing"
	"time"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestContentQueryServiceEnrichesOnlyActiveContentWithSafeSourceReference(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{page: ingestiondomain.ContentPage{Items: []ingestiondomain.Content{content}}},
		Sources:  &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{3: {Name: "RSS feed", SourceType: sourcedomain.SourceTypeRSS}}},
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}

	page, err := service.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListActive() error = %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].SourceName != "RSS feed" || page.Items[0].SourceType != sourcedomain.SourceTypeRSS {
		t.Fatalf("safe source projection = %#v, want type/name only", page.Items)
	}
}

func TestContentQueryServiceMapsInvalidCursorAndMissingContent(t *testing.T) {
	t.Parallel()
	service, err := NewContentQueryService(ContentQueryDependencies{
		Contents: &contentQueryRepositoryStub{listError: sharedrepository.ErrInvalidInput, getError: sharedrepository.ErrNotFound},
		Sources:  &contentSourceReaderStub{},
	})
	if err != nil {
		t.Fatalf("NewContentQueryService() error = %v", err)
	}
	if _, err := service.ListActive(context.Background(), ingestiondomain.ContentListQuery{Cursor: "malformed"}); !isAppCode(err, sharederrors.CodeInvalidRequest) {
		t.Fatalf("ListActive(malformed) error = %v, want invalid request", err)
	}
	if _, err := service.GetActive(context.Background(), 9); !isAppCode(err, sharederrors.CodeNotFound) {
		t.Fatalf("GetActive(missing) error = %v, want not found", err)
	}
}

func TestContentQueryServiceDoesNotReturnContentForDeletedOrUnavailableSource(t *testing.T) {
	t.Parallel()
	content := queryTestContent(7, 3)
	for _, test := range []struct {
		name      string
		reference sourcedomain.ContentSourceReference
		err       error
		wantCode  int
	}{
		{name: "deleted", reference: sourcedomain.ContentSourceReference{Name: "removed", SourceType: sourcedomain.SourceTypeRSS, Deleted: true}, wantCode: sharederrors.CodeNotFound},
		{name: "unavailable", err: sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", errors.New("minio private endpoint")), wantCode: sharederrors.CodeUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewContentQueryService(ContentQueryDependencies{
				Contents: &contentQueryRepositoryStub{content: content},
				Sources:  &contentSourceReaderStub{references: map[int64]sourcedomain.ContentSourceReference{3: test.reference}, err: test.err},
			})
			if err != nil {
				t.Fatalf("NewContentQueryService() error = %v", err)
			}
			if got, err := service.GetActive(context.Background(), content.ID); err == nil || !isAppCode(err, test.wantCode) || got.ID != 0 {
				t.Fatalf("GetActive() content/error = %#v / %v, want empty content and app code %d", got, err, test.wantCode)
			}
		})
	}
}

type contentQueryRepositoryStub struct {
	page      ingestiondomain.ContentPage
	content   ingestiondomain.Content
	listError error
	getError  error
}

func (repository *contentQueryRepositoryStub) Upsert(context.Context, ingestiondomain.NormalizedContent, ingestiondomain.DedupeDecision) (ingestiondomain.Content, bool, error) {
	return ingestiondomain.Content{}, false, errors.New("not used")
}
func (repository *contentQueryRepositoryStub) AppendMetricSnapshot(context.Context, int64, time.Time, sourcedomain.SourceMetrics) error {
	return errors.New("not used")
}
func (repository *contentQueryRepositoryStub) CreateAsset(context.Context, ingestiondomain.ContentAsset) error {
	return errors.New("not used")
}
func (repository *contentQueryRepositoryStub) MarkAssetStatus(context.Context, string, ingestiondomain.AssetStatus) error {
	return errors.New("not used")
}
func (repository *contentQueryRepositoryStub) ListEvidenceAssets(context.Context, int64, int64) ([]ingestiondomain.ContentAsset, error) {
	return nil, errors.New("not used")
}
func (repository *contentQueryRepositoryStub) ListAssetObjectKeys(context.Context, int64) ([]string, error) {
	return nil, errors.New("not used")
}
func (repository *contentQueryRepositoryStub) ListActive(context.Context, ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error) {
	return repository.page, repository.listError
}
func (repository *contentQueryRepositoryStub) GetActive(context.Context, int64) (ingestiondomain.Content, error) {
	return repository.content, repository.getError
}
func (repository *contentQueryRepositoryStub) MarkDeleted(context.Context, int64, string) (ingestiondomain.Content, bool, error) {
	return ingestiondomain.Content{}, false, errors.New("not used")
}
func (repository *contentQueryRepositoryStub) ExpireBefore(context.Context, time.Time) (int, error) {
	return 0, errors.New("not used")
}

type contentSourceReaderStub struct {
	references map[int64]sourcedomain.ContentSourceReference
	err        error
}

func (reader *contentSourceReaderStub) FindForContent(_ context.Context, id int64) (sourcedomain.ContentSourceReference, error) {
	if reader.err != nil {
		return sourcedomain.ContentSourceReference{}, reader.err
	}
	return reader.references[id], nil
}

func queryTestContent(id, sourceID int64) ingestiondomain.Content {
	return ingestiondomain.Content{ID: id, SourceConnectionID: sourceID, ExternalID: "external-id", ContentType: "article", Title: "safe", CanonicalURL: "https://example.test/item", PublishedAt: time.Now().UTC(), FetchedAt: time.Now().UTC(), Status: ingestiondomain.ContentStatusActive}
}

func isAppCode(err error, want int) bool {
	var appError *sharederrors.AppError
	return errors.As(err, &appError) && appError.Code == want
}
