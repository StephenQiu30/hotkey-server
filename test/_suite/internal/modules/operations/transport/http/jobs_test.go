package http

import (
	"context"
	"net/http/httptest"
	"testing"

	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type jobsServiceFake struct{}

func (jobsServiceFake) ListJobs(context.Context, operationsdomain.JobListQuery) (operationsdomain.JobPage, error) {
	return operationsdomain.JobPage{}, nil
}
func (jobsServiceFake) CancelJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return operationsdomain.JobSummary{}, nil
}
func (jobsServiceFake) RetryJob(context.Context, int64) (operationsdomain.JobSummary, error) {
	return operationsdomain.JobSummary{}, nil
}

func TestJobRoutesRequireAuthenticationAndAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service, err := operationsapplication.NewJobService(jobsServiceFake{}, nil)
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
