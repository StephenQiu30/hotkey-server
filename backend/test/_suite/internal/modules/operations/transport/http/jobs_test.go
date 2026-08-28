package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type jobsServiceFake struct {
	query operationsdomain.JobListQuery
	page  operationsdomain.JobPage
}

func (fake *jobsServiceFake) ListJobs(_ context.Context, query operationsdomain.JobListQuery) (operationsdomain.JobPage, error) {
	fake.query = query
	return fake.page, nil
}
func (*jobsServiceFake) CancelJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return operationsdomain.JobSummary{}, nil
}
func (*jobsServiceFake) RetryJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return operationsdomain.JobSummary{}, nil
}

func TestJobRoutesRequireAuthenticationAndAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service, err := operationsapplication.NewJobService(&jobsServiceFake{}, nil)
	if err != nil {
		t.Fatalf("NewJobService() error = %v", err)
	}
	RegisterJobRoutes(router, service, httptransport.NewUnavailableAuthenticator())
	request := httptest.NewRequest("GET", "/api/v1/operations/jobs", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatalf("unauthenticated job list status = %d, want 401", response.Code)
	}
}

func TestJobListForwardsOpaqueSubjectBoundCursorAndReturnsNextCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &jobsServiceFake{page: operationsdomain.JobPage{NextCursor: "signed.next.cursor"}}
	service, err := operationsapplication.NewJobService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterJobRoutes(router, service, governanceAuthenticator{role: httptransport.RoleAdmin})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/jobs?cursor=signed.input.cursor&kind=normalize_content&state=available&limit=2", nil)
	request.Header.Set("Authorization", "Bearer admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"next_cursor":"signed.next.cursor"`) {
		t.Fatalf("job list response = %d %s", response.Code, response.Body.String())
	}
	if store.query.SubjectUserID != 7 || store.query.Cursor != "signed.input.cursor" || store.query.Kind != "normalize_content" || store.query.State != operationsdomain.JobAvailable || store.query.Limit != 2 {
		t.Fatalf("job list query = %#v", store.query)
	}
}
