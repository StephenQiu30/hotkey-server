package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	searchapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/application"
	searchdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/search/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type searchServiceStub struct {
	request searchapplication.Request
	query   searchdomain.Query
	result  searchapplication.Result
	err     error
}

func (service *searchServiceStub) Search(_ context.Context, request searchapplication.Request) (searchapplication.Result, error) {
	service.request = request
	service.query = request.Query
	return service.result, service.err
}

type viewerAuthenticator struct{}

func (viewerAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 7, SessionID: 11, Role: httptransport.RoleViewer}, nil
}

func TestSearchRouteForwardsOpaqueCursorAndReturnsNextCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &searchServiceStub{result: searchapplication.Result{NextCursor: "next.opaque"}}
	router := gin.New()
	RegisterRoutes(router, service, viewerAuthenticator{})
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/search?q=release&cursor=current.opaque&limit=1", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
	if service.request.Cursor != "current.opaque" || service.request.Query.Limit != 1 {
		t.Fatalf("search request = %#v", service.request)
	}
	if body := response.Body.String(); !strings.Contains(body, `"next_cursor":"next.opaque"`) {
		t.Fatalf("response omitted opaque next cursor: %s", body)
	}
}

func TestSearchRouteParsesFiltersAndReturnsOnlyBoundedDTO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, 8, 28, 8, 0, 0, 0, time.UTC)
	service := &searchServiceStub{result: searchapplication.Result{Items: []searchapplication.Item{{
		Type: searchdomain.ResourceKnowledge, ID: 9, Title: "Release", Snippet: "Summary",
		TitleHighlight: "<mark>Release</mark>", SnippetHighlight: "Summary", Status: "active", OccurredAt: now, Score: 0.8,
	}}}}
	router := gin.New()
	RegisterRoutes(router, service, viewerAuthenticator{})
	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/search?q=release&types=knowledge,event&source_connection_id=2&monitor_id=3&entity=Acme-42&status=active&sort=latest&from=2026-08-27T00:00:00Z&to=2026-08-29T00:00:00Z&limit=15", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
	if service.query.Keyword != "release" || service.query.Sort != searchdomain.SortLatest || service.query.Limit != 15 || service.query.SourceConnectionID == nil || *service.query.SourceConnectionID != 2 || service.query.MonitorID == nil || *service.query.MonitorID != 3 || len(service.query.Types) != 2 {
		t.Fatalf("query = %#v", service.query)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != 0 || len(payload.Data.Items) != 1 || payload.Data.Items[0]["title_highlight"] != "<mark>Release</mark>" {
		t.Fatalf("payload = %#v", payload)
	}
	for _, forbidden := range []string{"vault_path", "object_key", "body", "provider", "embedding"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaks %s: %s", forbidden, response.Body.String())
		}
	}
}

func TestSearchRouteRequiresAuthenticationAndRejectsInvalidQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &searchServiceStub{}
	router := gin.New()
	RegisterRoutes(router, service, viewerAuthenticator{})
	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(stdhttp.MethodGet, "/api/v1/search?q=release", nil))
	if unauthenticated.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	for _, target := range []string{
		"/api/v1/search", "/api/v1/search?q=release&limit=101", "/api/v1/search?q=release&types=embedding",
		"/api/v1/search?q=release&from=not-a-time", "/api/v1/search?q=release&sort=semantic",
	} {
		request := httptest.NewRequest(stdhttp.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusBadRequest {
			t.Fatalf("GET %s status/body = %d/%s", target, response.Code, response.Body.String())
		}
	}
}
