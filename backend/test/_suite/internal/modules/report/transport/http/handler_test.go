package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	reportapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/report/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type reportServiceFake struct {
	report       domain.Report
	publishErr   error
	publishCalls int
	createCalls  int
}

func (fake *reportServiceFake) CreateDraft(_ context.Context, _ reportapplication.CreateInput) (domain.Report, error) {
	fake.createCalls++
	return fake.report, nil
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
	if fake.publishErr != nil {
		return domain.Report{}, fake.publishErr
	}
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
	service := &reportServiceFake{report: domain.Report{ID: 7, Version: 1, VersionNo: 1, Type: domain.ReportDaily, Period: period, Title: "daily", Status: domain.ReportDraft, Items: []domain.Item{{EventID: 9, EventUpdateID: 19, Rank: 1, Title: "event", HeatScore: 80, EvidenceSetHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReasonCodes: []string{"rising"}}}}}

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
	if response := reportJSONRequest(viewer, "/api/v1/reports", "viewer", `{"type":"daily","timezone":"UTC"}`); response.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want 403", response.Code)
	}
	analyst := gin.New()
	RegisterRoutes(analyst, service, reportAuthenticator{role: httptransport.RoleAnalyst})
	if response := reportRequest(analyst, http.MethodPost, "/api/v1/reports/7/publish", "analyst"); response.Code != http.StatusForbidden {
		t.Fatalf("analyst publish = %d, want 403", response.Code)
	}

	editor := gin.New()
	RegisterRoutes(editor, service, reportAuthenticator{role: httptransport.RoleEditor})
	if response := reportJSONRequest(editor, "/api/v1/reports", "editor", `{"type":"daily","timezone":"UTC"}`); response.Code != http.StatusOK || service.createCalls != 1 {
		t.Fatalf("editor create = %d/calls=%d: %s", response.Code, service.createCalls, response.Body.String())
	}
	if response := reportRequest(editor, http.MethodPost, "/api/v1/reports/7/publish", "editor"); response.Code != http.StatusOK || service.publishCalls != 1 {
		t.Fatalf("editor publish = %d/calls=%d: %s", response.Code, service.publishCalls, response.Body.String())
	}

	admin := gin.New()
	RegisterRoutes(admin, service, reportAuthenticator{role: httptransport.RoleAdmin})
	if response := reportRequest(admin, http.MethodPost, "/api/v1/reports/7/publish", "admin"); response.Code != http.StatusOK || service.publishCalls != 2 {
		t.Fatalf("admin publish = %d/calls=%d: %s", response.Code, service.publishCalls, response.Body.String())
	}
}

func TestReportPublishMapsInvalidEvidenceToStableConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &reportServiceFake{publishErr: domain.ErrEvidenceInvalid}
	router := gin.New()
	RegisterRoutes(router, service, reportAuthenticator{role: httptransport.RoleEditor})
	response := reportRequest(router, http.MethodPost, "/api/v1/reports/7/publish", "editor")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":80000`) {
		t.Fatalf("invalid evidence response = %d: %s", response.Code, response.Body.String())
	}
}

func TestReportResponseExposesExactSentenceCitations(t *testing.T) {
	actorID := int64(3)
	response := reportResponse(domain.Report{Items: []domain.Item{{MicroEventID: 9, MicroEventVersion: 2,
		MicroEventUpdateID: 19, MicroEventSummaryID: 29, Sentences: []domain.Sentence{{SourceSummarySentenceID: 39,
			Text: "可引用事实", DecisionOrigin: "manual", ActorUserID: &actorID, ClaimEvidenceVersionIDs: []int64{49, 50}}}}}})
	if len(response.Items) != 1 || len(response.Items[0].Sentences) != 1 ||
		response.Items[0].Sentences[0].SourceSummarySentenceID != 39 ||
		len(response.Items[0].Sentences[0].ClaimEvidenceVersionIDs) != 2 {
		t.Fatalf("report response = %#v", response)
	}
}

func reportJSONRequest(router *gin.Engine, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
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
