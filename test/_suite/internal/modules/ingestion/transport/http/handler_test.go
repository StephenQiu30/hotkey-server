package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestContentRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	zero := int64(0)
	content := ingestiondomain.Content{
		ID: 7, SourceConnectionID: 3, ExternalID: "item-7", ContentType: "article",
		Title: "Safe content title", Excerpt: "private-body-not-for-api", CanonicalURL: "https://example.test/items/7",
		Language: "en", PublishedAt: time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC),
		FetchedAt: time.Date(2026, time.July, 17, 8, 1, 0, 0, time.UTC),
		Metrics:   sourcedomain.SourceMetrics{ViewCount: &zero}, Status: ingestiondomain.ContentStatusActive,
	}

	t.Run("unauthenticated request is rejected before application", func(t *testing.T) {
		service := &contentQueryServiceStub{page: ingestiondomain.ContentPage{Items: []ingestiondomain.Content{content}}}
		router := newContentRouter(t, service, httptransport.RoleViewer)
		response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents", "")
		if response.Code != stdhttp.StatusUnauthorized {
			t.Fatalf("status = %d, want %d: %s", response.Code, stdhttp.StatusUnauthorized, response.Body.String())
		}
		if service.listCalls != 0 || service.getCalls != 0 {
			t.Fatalf("application calls = list=%d get=%d, want zero", service.listCalls, service.getCalls)
		}
	})

	for _, test := range []struct {
		name   string
		role   httptransport.Role
		path   string
		method string
	}{
		{name: "viewer lists active content", role: httptransport.RoleViewer, method: stdhttp.MethodGet, path: "/api/v1/contents"},
		{name: "admin gets active content", role: httptransport.RoleAdmin, method: stdhttp.MethodGet, path: "/api/v1/contents/7"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &contentQueryServiceStub{page: ingestiondomain.ContentPage{Items: []ingestiondomain.Content{content}}, content: content}
			router := newContentRouter(t, service, test.role)
			response := performContentRequest(router, test.method, test.path, "member")
			if response.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			assertContentSuccessResponse(t, response)
			if strings.Contains(response.Body.String(), "private-body-not-for-api") {
				t.Fatalf("safe DTO leaked Content excerpt: %s", response.Body.String())
			}
		})
	}

	t.Run("invalid cursor is a safe bad request", func(t *testing.T) {
		service := &contentQueryServiceStub{listError: sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "")}
		router := newContentRouter(t, service, httptransport.RoleViewer)
		response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents?cursor=not-a-content-cursor", "viewer")
		if response.Code != stdhttp.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
		}
		assertContentErrorResponse(t, response, sharederrors.CodeInvalidRequest)
		if service.lastQuery.Cursor != "not-a-content-cursor" {
			t.Fatalf("cursor passed to application = %q", service.lastQuery.Cursor)
		}
	})

	for _, test := range []struct {
		name string
		id   int64
	}{
		{name: "missing content", id: 404},
		{name: "deleted content", id: 410},
	} {
		t.Run(test.name+" is not found", func(t *testing.T) {
			service := &contentQueryServiceStub{getErrors: map[int64]error{
				test.id: sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, ""),
			}}
			router := newContentRouter(t, service, httptransport.RoleViewer)
			response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents/"+strconvFormat(test.id), "viewer")
			if response.Code != stdhttp.StatusNotFound {
				t.Fatalf("status = %d, want 404: %s", response.Code, response.Body.String())
			}
			assertContentErrorResponse(t, response, sharederrors.CodeNotFound)
		})
	}

	t.Run("errors never disclose evidence or object-store internals", func(t *testing.T) {
		const sensitive = "evidence/v1/3/aa/private-object.txt minio.internal:9000 access-key private-body stacktrace"
		service := &contentQueryServiceStub{getErrors: map[int64]error{
			13: sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", errors.New(sensitive)),
		}}
		router := newContentRouter(t, service, httptransport.RoleViewer)
		response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents/13", "viewer")
		if response.Code != stdhttp.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503: %s", response.Code, response.Body.String())
		}
		assertContentErrorResponse(t, response, sharederrors.CodeUnavailable)
		for _, value := range []string{"evidence/v1/", "minio.internal", "access-key", "private-body", "stacktrace"} {
			if strings.Contains(response.Body.String(), value) {
				t.Fatalf("error response leaked %q: %s", value, response.Body.String())
			}
		}
	})

	t.Run("list skips a deleted source but preserves the safe live content", func(t *testing.T) {
		deletedSourceContent := content
		deletedSourceContent.ID, deletedSourceContent.SourceConnectionID = 8, 4
		deletedSourceContent.Excerpt = "private-deleted-source-body"
		service, err := ingestionapplication.NewContentQueryService(ingestionapplication.ContentQueryDependencies{
			Contents: &contentTransportRepository{page: ingestiondomain.ContentPage{Items: []ingestiondomain.Content{content, deletedSourceContent}, NextCursor: "opaque-repository-cursor"}},
			Sources: contentTransportSourceReader{references: map[int64]sourcedomain.ContentSourceReference{
				3: {Name: "Live RSS", SourceType: sourcedomain.SourceTypeRSS},
				4: {Name: "Removed RSS", SourceType: sourcedomain.SourceTypeRSS, Deleted: true},
			}},
		})
		if err != nil {
			t.Fatalf("NewContentQueryService() error = %v", err)
		}
		router := newContentRouter(t, service, httptransport.RoleViewer)
		response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents", "viewer")
		if response.Code != stdhttp.StatusOK {
			t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
		}
		assertContentSuccessResponse(t, response)
		if strings.Contains(response.Body.String(), "private-deleted-source-body") || strings.Contains(response.Body.String(), "Removed RSS") {
			t.Fatalf("skipped source leaked unsafe content: %s", response.Body.String())
		}
	})
}

func TestContentDocumentRouteAllowsAuthenticatedRolesAndReturnsSafeProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	document := ingestiondomain.ContentDocument{
		ContentID: 7, Title: "Archived title", SourceName: "RSS feed", CanonicalURL: "https://example.test/items/7",
		Language: "zh", PublishedAt: time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC),
		Availability: ingestiondomain.ContentDocumentReady, Markdown: "# 正文\n", SHA256: strings.Repeat("a", 64),
		CapturedAt: time.Date(2026, time.July, 17, 8, 1, 0, 0, time.UTC),
	}
	for _, role := range []httptransport.Role{httptransport.RoleViewer, httptransport.RoleEditor, httptransport.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			service := &contentQueryServiceStub{document: document}
			router := newContentRouter(t, service, role)
			response := performContentRequest(router, stdhttp.MethodGet, "/api/v1/contents/7/document", "member")
			if response.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"availability":"ready"`) || !strings.Contains(response.Body.String(), `"markdown":"# 正文\n"`) {
				t.Fatalf("document response = %s", response.Body.String())
			}
			for _, forbidden := range []string{"object_key", "minio", "access_key"} {
				if strings.Contains(response.Body.String(), forbidden) {
					t.Fatalf("document response leaked %q: %s", forbidden, response.Body.String())
				}
			}
		})
	}

	service := &contentQueryServiceStub{documentErrors: map[int64]error{
		13:  sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, ""),
		404: sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, ""),
	}}
	router := newContentRouter(t, service, httptransport.RoleViewer)
	for _, test := range []struct {
		path       string
		wantStatus int
		wantCode   int
	}{
		{path: "/api/v1/contents/not-a-number/document", wantStatus: stdhttp.StatusBadRequest, wantCode: sharederrors.CodeInvalidRequest},
		{path: "/api/v1/contents/404/document", wantStatus: stdhttp.StatusNotFound, wantCode: sharederrors.CodeNotFound},
		{path: "/api/v1/contents/13/document", wantStatus: stdhttp.StatusServiceUnavailable, wantCode: sharederrors.CodeUnavailable},
	} {
		response := performContentRequest(router, stdhttp.MethodGet, test.path, "viewer")
		if response.Code != test.wantStatus {
			t.Fatalf("%s status = %d, want %d: %s", test.path, response.Code, test.wantStatus, response.Body.String())
		}
		assertContentErrorResponse(t, response, test.wantCode)
	}
}

func TestContentDeleteRouteRequiresEditorAndReturnsEmptySuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		role       httptransport.Role
		wantStatus int
		wantCalls  int
	}{
		{name: "viewer cannot delete", role: httptransport.RoleViewer, wantStatus: stdhttp.StatusForbidden, wantCalls: 0},
		{name: "editor can delete", role: httptransport.RoleEditor, wantStatus: stdhttp.StatusOK, wantCalls: 1},
		{name: "admin can delete", role: httptransport.RoleAdmin, wantStatus: stdhttp.StatusOK, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &contentQueryServiceStub{
				deleteResult: ingestionapplication.DeleteBySourceItemResult{ContentChanged: true},
			}
			router := newContentRouter(t, service, test.role)
			response := performContentRequest(router, stdhttp.MethodDelete, "/api/v1/contents/7", "member")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if service.deleteCalls != test.wantCalls {
				t.Fatalf("delete calls = %d, want %d", service.deleteCalls, test.wantCalls)
			}
			if test.wantStatus == stdhttp.StatusOK {
				assertContentErrorResponse(t, response, 0)
				if service.lastDeletedID != 7 {
					t.Fatalf("deleted content id = %d, want 7", service.lastDeletedID)
				}
			}
		})
	}

	t.Run("invalid id is rejected before application", func(t *testing.T) {
		service := &contentQueryServiceStub{}
		router := newContentRouter(t, service, httptransport.RoleEditor)
		response := performContentRequest(router, stdhttp.MethodDelete, "/api/v1/contents/not-a-number", "editor")
		if response.Code != stdhttp.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", response.Code, response.Body.String())
		}
		assertContentErrorResponse(t, response, sharederrors.CodeInvalidRequest)
		if service.deleteCalls != 0 {
			t.Fatalf("delete calls = %d, want zero", service.deleteCalls)
		}
	})

	t.Run("storage failure is safe", func(t *testing.T) {
		service := &contentQueryServiceStub{
			deleteErrors: map[int64]error{
				7: sharederrors.Wrap(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "", errors.New("minio.internal evidence/v1/private-body")),
			},
		}
		router := newContentRouter(t, service, httptransport.RoleEditor)
		response := performContentRequest(router, stdhttp.MethodDelete, "/api/v1/contents/7", "editor")
		if response.Code != stdhttp.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503: %s", response.Code, response.Body.String())
		}
		assertContentErrorResponse(t, response, sharederrors.CodeUnavailable)
		if strings.Contains(response.Body.String(), "minio.internal") || strings.Contains(response.Body.String(), "private-body") {
			t.Fatalf("delete error leaked storage details: %s", response.Body.String())
		}
	})
}

type contentQueryServiceStub struct {
	page           ingestiondomain.ContentPage
	content        ingestiondomain.Content
	listError      error
	getErrors      map[int64]error
	document       ingestiondomain.ContentDocument
	documentErrors map[int64]error
	deleteResult   ingestionapplication.DeleteBySourceItemResult
	deleteErrors   map[int64]error
	listCalls      int
	getCalls       int
	deleteCalls    int
	lastDeletedID  int64
	lastQuery      ingestiondomain.ContentListQuery
}

func (service *contentQueryServiceStub) ListActive(_ context.Context, query ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error) {
	service.listCalls++
	service.lastQuery = query
	return service.page, service.listError
}

func (service *contentQueryServiceStub) GetActive(_ context.Context, id int64) (ingestiondomain.Content, error) {
	service.getCalls++
	if err := service.getErrors[id]; err != nil {
		return ingestiondomain.Content{}, err
	}
	return service.content, nil
}

func (service *contentQueryServiceStub) GetDocument(_ context.Context, id int64) (ingestiondomain.ContentDocument, error) {
	if err := service.documentErrors[id]; err != nil {
		return ingestiondomain.ContentDocument{}, err
	}
	return service.document, nil
}

func (service *contentQueryServiceStub) DeleteContent(_ context.Context, id int64) (ingestionapplication.DeleteBySourceItemResult, error) {
	service.deleteCalls++
	service.lastDeletedID = id
	if err := service.deleteErrors[id]; err != nil {
		return ingestionapplication.DeleteBySourceItemResult{}, err
	}
	return service.deleteResult, nil
}

type contentAuthenticator struct{ role httptransport.Role }

func (authenticator contentAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 2, Role: authenticator.role}, nil
}

func newContentRouter(t *testing.T, service contentQueryService, role httptransport.Role) *gin.Engine {
	t.Helper()
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics(): %v", err)
	}
	router := gin.New()
	RegisterRoutes(router, service, contentAuthenticator{role: role}, metrics)
	return router
}

type contentTransportSourceReader struct {
	references map[int64]sourcedomain.ContentSourceReference
}

func (reader contentTransportSourceReader) FindForContent(_ context.Context, id int64) (sourcedomain.ContentSourceReference, error) {
	return reader.references[id], nil
}

// contentTransportRepository embeds the full domain port so this focused
// handler test supplies only the active-list behavior it exercises.
type contentTransportRepository struct {
	ingestiondomain.ContentRepository
	page ingestiondomain.ContentPage
}

func (repository *contentTransportRepository) ListActive(context.Context, ingestiondomain.ContentListQuery) (ingestiondomain.ContentPage, error) {
	return repository.page, nil
}

func performContentRequest(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertContentSuccessResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("business code = %d, want 0", envelope.Code)
	}
	var page struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(envelope.Data, &page); err != nil {
		t.Fatalf("decode content data: %v", err)
	}
	if page.Items != nil {
		if len(page.Items) != 1 {
			t.Fatalf("list content items = %d, want one", len(page.Items))
		}
		assertContentDTOAllowlist(t, page.Items[0])
		return
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &item); err != nil {
		t.Fatalf("decode content item: %v", err)
	}
	assertContentDTOAllowlist(t, item)
}

func assertContentErrorResponse(t *testing.T, response *httptest.ResponseRecorder, wantCode int) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if envelope.Code != wantCode || string(envelope.Data) != "null" {
		t.Fatalf("error response = %#v, want code %d and data null", envelope, wantCode)
	}
}

func assertContentDTOAllowlist(t *testing.T, item map[string]json.RawMessage) {
	t.Helper()
	want := map[string]bool{
		"id": true, "source_type": true, "source_name": true, "external_id": true,
		"content_type": true, "title": true, "canonical_url": true, "language": true,
		"published_at": true, "fetched_at": true, "metrics": true, "dedupe_status": true,
		"dedupe_reason": true, "dedupe_version": true,
	}
	if len(item) != len(want) {
		t.Fatalf("content DTO fields = %v, want allowlist %v", mapKeys(item), mapKeysBool(want))
	}
	for field := range item {
		if !want[field] {
			t.Fatalf("content DTO exposes forbidden field %q", field)
		}
	}
	var metrics map[string]json.RawMessage
	if err := json.Unmarshal(item["metrics"], &metrics); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	for _, field := range []string{"view_count", "like_count", "comment_count", "share_count"} {
		if _, exists := metrics[field]; !exists {
			t.Errorf("nullable metrics misses %q", field)
		}
	}
}

func mapKeys(values map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func mapKeysBool(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func strconvFormat(value int64) string {
	return strings.TrimSpace(fmt.Sprintf("%d", value))
}
