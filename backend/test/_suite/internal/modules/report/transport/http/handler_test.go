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
	report        domain.Report
	lastListQuery domain.ListQuery
	lifecycleErr  error
	submitCalls   int
	approveCalls  int
	rejectCalls   int
	createCalls   int
}

func (fake *reportServiceFake) CreateDraft(_ context.Context, _ reportapplication.CreateInput) (domain.Report, error) {
	fake.createCalls++
	return fake.report, nil
}

func (fake *reportServiceFake) List(_ context.Context, query domain.ListQuery) (domain.Page, error) {
	fake.lastListQuery = query
	return domain.Page{Items: []domain.Report{fake.report}}, nil
}

func (fake *reportServiceFake) Get(_ context.Context, _ int64) (domain.Report, error) {
	return fake.report, nil
}
func (fake *reportServiceFake) Preview(_ context.Context, _ int64) (domain.Report, error) {
	return fake.report, nil
}
func (fake *reportServiceFake) SubmitForApproval(_ context.Context, _ reportapplication.RevisionLifecycleInput) (domain.Report, error) {
	fake.submitCalls++
	if fake.lifecycleErr != nil {
		return domain.Report{}, fake.lifecycleErr
	}
	fake.report.Status = domain.ReportPendingApproval
	return fake.report, nil
}
func (fake *reportServiceFake) ApproveRevision(_ context.Context, _ reportapplication.RevisionLifecycleInput) (domain.Report, error) {
	fake.approveCalls++
	if fake.lifecycleErr != nil {
		return domain.Report{}, fake.lifecycleErr
	}
	fake.report.Status, fake.report.Frozen = domain.ReportPublished, true
	return fake.report, nil
}
func (fake *reportServiceFake) RejectRevision(_ context.Context, _ reportapplication.RevisionLifecycleInput) (domain.Report, error) {
	fake.rejectCalls++
	if fake.lifecycleErr != nil {
		return domain.Report{}, fake.lifecycleErr
	}
	fake.report.Status = domain.ReportRejected
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
	if response := reportJSONRequest(viewer, "/api/v1/reports/7/approve", "viewer", `{"expected_resource_version":1}`); response.Code != http.StatusForbidden {
		t.Fatalf("viewer approve = %d, want 403", response.Code)
	}
	if response := reportJSONRequest(viewer, "/api/v1/reports", "viewer", `{"type":"daily","timezone":"UTC"}`); response.Code != http.StatusForbidden {
		t.Fatalf("viewer create = %d, want 403", response.Code)
	}
	analyst := gin.New()
	RegisterRoutes(analyst, service, reportAuthenticator{role: httptransport.RoleAnalyst})
	if response := reportJSONRequest(analyst, "/api/v1/reports", "analyst", `{"type":"daily","timezone":"UTC"}`); response.Code != http.StatusOK {
		t.Fatalf("analyst create = %d: %s", response.Code, response.Body.String())
	}
	if response := reportJSONRequest(analyst, "/api/v1/reports/7/submit", "analyst", `{"expected_resource_version":1}`); response.Code != http.StatusOK || service.submitCalls != 1 {
		t.Fatalf("analyst submit = %d/calls=%d: %s", response.Code, service.submitCalls, response.Body.String())
	}
	if response := reportJSONRequest(analyst, "/api/v1/reports/7/approve", "analyst", `{"expected_resource_version":1}`); response.Code != http.StatusForbidden {
		t.Fatalf("analyst approve = %d, want 403", response.Code)
	}

	editor := gin.New()
	RegisterRoutes(editor, service, reportAuthenticator{role: httptransport.RoleEditor})
	if response := reportJSONRequest(editor, "/api/v1/reports", "editor", `{"type":"daily","timezone":"UTC"}`); response.Code != http.StatusOK || service.createCalls != 2 {
		t.Fatalf("editor create = %d/calls=%d: %s", response.Code, service.createCalls, response.Body.String())
	}
	if response := reportJSONRequest(editor, "/api/v1/reports/7/approve", "editor", `{"expected_resource_version":1}`); response.Code != http.StatusOK || service.approveCalls != 1 {
		t.Fatalf("editor approve = %d/calls=%d: %s", response.Code, service.approveCalls, response.Body.String())
	}

	admin := gin.New()
	RegisterRoutes(admin, service, reportAuthenticator{role: httptransport.RoleAdmin})
	if response := reportJSONRequest(admin, "/api/v1/reports/7/reject", "admin", `{"expected_resource_version":1,"reason_code":"insufficient_context"}`); response.Code != http.StatusOK || service.rejectCalls != 1 {
		t.Fatalf("admin reject = %d/calls=%d: %s", response.Code, service.rejectCalls, response.Body.String())
	}
}

func TestReportApprovalMapsInvalidEvidenceToStableConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &reportServiceFake{lifecycleErr: domain.ErrEvidenceInvalid}
	router := gin.New()
	RegisterRoutes(router, service, reportAuthenticator{role: httptransport.RoleEditor})
	response := reportJSONRequest(router, "/api/v1/reports/7/approve", "editor", `{"expected_resource_version":1}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":80000`) {
		t.Fatalf("invalid evidence response = %d: %s", response.Code, response.Body.String())
	}
}

func TestReportListTreatsCursorAsOpaqueTransportValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &reportServiceFake{}
	router := gin.New()
	RegisterRoutes(router, service, reportAuthenticator{role: httptransport.RoleViewer})
	response := reportRequest(router, http.MethodGet, "/api/v1/reports?cursor=opaque.signed&limit=7&type=daily", "viewer")
	if response.Code != http.StatusOK {
		t.Fatalf("list response = %d: %s", response.Code, response.Body.String())
	}
	if service.lastListQuery.Cursor != "opaque.signed" || service.lastListQuery.Limit != 7 ||
		service.lastListQuery.Type == nil || *service.lastListQuery.Type != domain.ReportDaily {
		t.Fatalf("list query = %#v", service.lastListQuery)
	}
}

func TestReportApprovalMapsUnsafeContentWithoutReflectingPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &reportServiceFake{lifecycleErr: domain.ErrUnsafeContent}
	router := gin.New()
	RegisterRoutes(router, service, reportAuthenticator{role: httptransport.RoleEditor})
	response := reportJSONRequest(router, "/api/v1/reports/7/approve", "editor", `{"expected_resource_version":1}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":80001`) {
		t.Fatalf("unsafe content response = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "script") {
		t.Fatalf("unsafe response reflects payload class: %s", response.Body.String())
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
