package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestReportHTTPFlow(t *testing.T) {
	repo := servicereport.NewMemoryReportRepository()
	_, err := repo.SaveReport(context.Background(), servicereport.DailyReport{
		ID: "rpt-1", Date: "2026-05-31", Body: "中文日报\n来源引用：[1]", Status: servicereport.ReportStatusSucceeded,
		PromptVersion: servicereport.PromptVersion,
		SourceRefs:    []servicereport.SourceRef{{SourceID: "src-1", ItemID: "item-1", Title: "来源", URL: "https://example.test/1"}},
	})
	if err != nil {
		t.Fatalf("save report: %v", err)
	}
	router := transportRouterWithDependenciesForTest(transporthttp.Dependencies{
		ReportService: servicereport.NewService(repo, nil, nil, nil, nil, nil),
	})
	token := registerAndLogin(t, router, "report-user@example.com")

	list := getWithBearer(router, "/api/v1/reports?date=2026-05-31", token)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "中文日报") {
		t.Fatalf("expected reports list, got %d %s", list.Code, list.Body.String())
	}
	detail := getWithBearer(router, "/api/v1/reports/rpt-1", token)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "sourceRefs") {
		t.Fatalf("expected report detail, got %d %s", detail.Code, detail.Body.String())
	}
}
