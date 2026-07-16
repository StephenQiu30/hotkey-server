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

func TestContentQueryServiceListSkipsDeletedOrUnavailableSourceAndKeepsCursor(t *testing.T) {
	t.Parallel()
	live := queryTestContent(7, 3)
	deleted := queryTestContent(8, 4)
	for _, test := range []struct {
		name       string
		references map[int64]sourcedomain.ContentSourceReference
		errors     map[int64]error
	}{
		{
			name: "deleted source",
			references: map[int64]sourcedomain.ContentSourceReference{
				3: {Name: "live RSS", SourceType: sourcedomain.SourceTypeRSS},
				4: {Name: "removed RSS", SourceType: sourcedomain.SourceTypeRSS, Deleted: true},
			},
		},
		{
			name: "unavailable source",
			references: map[int64]sourcedomain.ContentSourceReference{
				3: {Name: "live RSS", SourceType: sourcedomain.SourceTypeRSS},
			},
			errors: map[int64]error{
				4: sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", errors.New("private source diagnostic")),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewContentQueryService(ContentQueryDependencies{
				Contents: &contentQueryRepositoryStub{page: ingestiondomain.ContentPage{Items: []ingestiondomain.Content{live, deleted}, NextCursor: "repository-cursor"}},
				Sources:  &contentSourceReaderStub{references: test.references, errors: test.errors},
			})
			if err != nil {
				t.Fatalf("NewContentQueryService() error = %v", err)
			}
			page, err := service.ListActive(context.Background(), ingestiondomain.ContentListQuery{Limit: 2})
			if err != nil {
				t.Fatalf("ListActive() error = %v, want live item", err)
			}
			if len(page.Items) != 1 || page.Items[0].ID != live.ID || page.Items[0].SourceName != "live RSS" || page.NextCursor != "repository-cursor" {
				t.Fatalf("ListActive() page = %#v, want only live item and unchanged cursor", page)
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
	errors     map[int64]error
}

func (reader *contentSourceReaderStub) FindForContent(_ context.Context, id int64) (sourcedomain.ContentSourceReference, error) {
	if err := reader.errors[id]; err != nil {
		return sourcedomain.ContentSourceReference{}, err
	}
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
