package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
	"github.com/gin-gonic/gin"
)

func TestListReports(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	repo := servicereport.NewMemoryReportRepository()
	_, err := repo.SaveReport(context.Background(), servicereport.DailyReport{ID: "rpt-1", Date: "2026-05-31", Body: "中文日报", Status: servicereport.ReportStatusSucceeded})
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	service := servicereport.NewService(repo, nil, nil, nil, nil, nil)
	router := gin.New()
	h := New(service)
	router.GET("/reports", h.ListReports)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/reports?date=2026-05-31", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "中文日报") {
		t.Fatalf("expected 200 with report, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestGetReportNotFound(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	service := servicereport.NewService(servicereport.NewMemoryReportRepository(), nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/reports/:reportID", New(service).GetReport)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/reports/missing", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", rec.Code, rec.Body.String())
	}
}
