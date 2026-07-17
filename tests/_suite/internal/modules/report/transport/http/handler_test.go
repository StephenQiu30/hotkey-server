package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/report/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type reportServiceFake struct {
	report       domain.Report
	publishCalls int
}

func (fake *reportServiceFake) List(_ context.Context, _ domain.ListQuery) (domain.Page, error) {
	return domain.Page{Items: []domain.Report{fake.report}}, nil
}

func (fake *reportServiceFake) Get(_ context.Context, _ int64) (domain.Report, error) {
	return fake.report, nil
}
func (fake *reportServiceFake) Preview(_ context.Context, _ int64) (domain.Report, error) {
	return fake.report, nil
}
func (fake *reportServiceFake) Publish(_ context.Context, _ int64) (domain.Report, error) {
	fake.publishCalls++
	fake.report.Status = domain.ReportPublished
	fake.report.Frozen = true
	return fake.report, nil
}

type reportAuthenticator struct{ role httptransport.Role }

func (auth reportAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 2, Role: auth.role}, nil
}

func TestReportRoutesProtectPublicationAndExposePreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	period, err := domain.PeriodFor(time.Now().UTC(), domain.ReportDaily, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	service := &reportServiceFake{report: domain.Report{ID: 7, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period, Title: "daily", Status: domain.ReportDraft, Items: []domain.Item{{EventID: 9, Rank: 1, Title: "event", HeatScore: 80}}}}

	unauthenticated := gin.New()
	RegisterRoutes(unauthenticated, service, reportAuthenticator{role: httptransport.RoleViewer})
	if response := reportRequest(unauthenticated, http.MethodGet, "/api/v1/reports", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d, want 401", response.Code)
	}

	viewer := gin.New()
	RegisterRoutes(viewer, service, reportAuthenticator{role: httptransport.RoleViewer})
	if response := reportRequest(viewer, http.MethodPost, "/api/v1/reports/7/preview", "viewer"); response.Code != http.StatusOK {
		t.Fatalf("viewer preview = %d: %s", response.Code, response.Body.String())
	}
	if response := reportRequest(viewer, http.MethodPost, "/api/v1/reports/7/publish", "viewer"); response.Code != http.StatusForbidden {
		t.Fatalf("viewer publish = %d, want 403", response.Code)
	}

	admin := gin.New()
	RegisterRoutes(admin, service, reportAuthenticator{role: httptransport.RoleAdmin})
	if response := reportRequest(admin, http.MethodPost, "/api/v1/reports/7/publish", "admin"); response.Code != http.StatusOK || service.publishCalls != 1 {
		t.Fatalf("admin publish = %d/calls=%d: %s", response.Code, service.publishCalls, response.Body.String())
	}
}

func reportRequest(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
