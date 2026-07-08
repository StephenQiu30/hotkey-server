
package platformhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/controller"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

type stubReportService struct {
	items []dto.Report
}

func (s *stubReportService) Create(ctx context.Context, userID int64, input dto.CreateInput) (dto.Report, error) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	item := dto.Report{
		ID:           int64(len(s.items) + 1),
		UserID:       userID,
		ReportType:   input.ReportType,
		PeriodStart:  time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
		PeriodEnd:    time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
		Subject:      "AI Regulation 周报",
		Summary:      "本期共跟踪 2 个热点。",
		Content:      "# AI Regulation 周报\n\n## 本周概览\n\n## 热点主题\n",
		HotspotCount: 2,
		Status:       service.StatusDraft,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.items = append(s.items, item)
	return item, nil
}

func (s *stubReportService) List(ctx context.Context, userID int64, filter dto.ListFilter) ([]dto.Report, int64, error) {
	return s.items, int64(len(s.items)), nil
}

func (s *stubReportService) GetByID(ctx context.Context, id, userID int64) (dto.Report, error) {
	for _, item := range s.items {
		if item.ID == id && item.UserID == userID {
			return item, nil
		}
	}
	return dto.Report{}, service.ReportErrNotFound
}

func (s *stubReportService) HTML(ctx context.Context, id, userID int64) (string, error) {
	item, err := s.GetByID(ctx, id, userID)
	if err != nil {
		return "", err
	}
	return "<h1>" + item.Subject + "</h1>", nil
}

func (s *stubReportService) MarkSent(ctx context.Context, id, userID int64) (dto.Report, error) {
	item, err := s.GetByID(ctx, id, userID)
	if err != nil {
		return dto.Report{}, err
	}
	item.Status = service.StatusSent
	s.items[id-1] = item
	return item, nil
}

func TestReportRoutesCreateReadHTMLAndSend(t *testing.T) {
	reports := &stubReportService{}
	handler := newTestHandlerWithReports(reports)
	token := testToken(t, 7)

	createBody := bytes.NewBufferString(`{"report_type":"weekly","period_start":"2026-06-24"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports", createBody)
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created struct {
		Data dto.Report `json:"data"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.ReportType != service.TypeWeekly {
		t.Fatalf("created report type = %q", created.Data.ReportType)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/reports?report_type=weekly", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}

	htmlReq := httptest.NewRequest(http.MethodGet, "/api/v1/reports/1/html", nil)
	htmlReq.Header.Set("Authorization", "Bearer "+token)
	htmlRR := httptest.NewRecorder()
	handler.ServeHTTP(htmlRR, htmlReq)
	if htmlRR.Code != http.StatusOK {
		t.Fatalf("html expected 200, got %d: %s", htmlRR.Code, htmlRR.Body.String())
	}
	if !strings.Contains(htmlRR.Body.String(), "<h1>AI Regulation 周报</h1>") {
		t.Fatalf("html body mismatch: %s", htmlRR.Body.String())
	}

	sendReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports/1/send", nil)
	sendReq.Header.Set("Authorization", "Bearer "+token)
	sendRR := httptest.NewRecorder()
	handler.ServeHTTP(sendRR, sendReq)
	if sendRR.Code != http.StatusOK {
		t.Fatalf("send expected 200, got %d: %s", sendRR.Code, sendRR.Body.String())
	}
}

func newTestHandlerWithReports(reports controller.ReportService) http.Handler {
	return controller.NewRouter(controller.Config{
		JWTSecret:     "test-secret",
		SmokeTest:     false,
		AuthService:   service.NewAuthService(&stubAuthRepo{}),
		MonitorSvc:    service.NewMonitorService(&stubMonitorRepo{}, nil),
		NotifySvc:     service.NewNotifyService(&stubNotifyRepo{}),
		ReportSvc:     reports,
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})
}

func testToken(t *testing.T, userID int64) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": float64(userID),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return tokenStr
}
