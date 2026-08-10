package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type microEventQueryRepositoryFake struct {
	evidence application.MicroEventEvidencePageDTO
}

func (fake *microEventQueryRepositoryFake) ListMicroEvents(context.Context, application.MicroEventListQuery) (application.MicroEventPageDTO, error) {
	return application.MicroEventPageDTO{Items: []application.MicroEventProjectionDTO{{
		ID: 7, Version: 3, EventKey: "semantic-event", Status: "active",
		PrimarySubjectKey: "acme", PrimaryActionKey: "announced", EventStartedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
		ClusteringProfileVersion: "micro-event-clustering-v1", ContentFamilyCount: 2, DocumentCount: 3,
	}}}, nil
}

func (fake *microEventQueryRepositoryFake) GetMicroEvent(context.Context, int64) (application.MicroEventProjectionDTO, error) {
	return application.MicroEventProjectionDTO{ID: 7, Version: 3, EventKey: "semantic-event", Status: "active",
		PrimarySubjectKey: "acme", PrimaryActionKey: "announced", EventStartedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
		ClusteringProfileVersion: "micro-event-clustering-v1"}, nil
}

func (fake *microEventQueryRepositoryFake) ListMicroEventEvidence(context.Context, application.MicroEventEvidenceQuery) (application.MicroEventEvidencePageDTO, error) {
	return fake.evidence, nil
}

func (fake *microEventQueryRepositoryFake) GetMicroEventSummary(context.Context, int64, int64) (*application.EvidenceSummaryDTO, error) {
	return nil, application.ErrEvidenceSummaryUnavailable
}

type microEventGovernanceRepositoryFake struct{}

func (*microEventGovernanceRepositoryFake) ApplyMicroEventGovernance(context.Context, application.ApplyMicroEventGovernanceCommand) (application.ApplyMicroEventGovernanceResult, error) {
	return application.ApplyMicroEventGovernanceResult{}, nil
}

type claimEvidenceRepositoryFake struct{}

func (*claimEvidenceRepositoryFake) ReadClaimEvidenceTarget(context.Context, application.ClaimEvidenceTargetQuery) (application.ClaimEvidenceTargetDTO, error) {
	return application.ClaimEvidenceTargetDTO{}, nil
}
func (*claimEvidenceRepositoryFake) CommitClaimEvidence(context.Context, application.CommitClaimEvidenceCommand) (application.RecordClaimEvidenceResult, error) {
	return application.RecordClaimEvidenceResult{}, nil
}
func (*claimEvidenceRepositoryFake) ReadEvidenceStateTarget(context.Context, application.EvidenceStateTargetQuery) (application.EvidenceStateTargetDTO, error) {
	return application.EvidenceStateTargetDTO{}, nil
}
func (*claimEvidenceRepositoryFake) CommitEvidenceStateSnapshot(context.Context, application.CommitEvidenceStateSnapshotCommand) (application.EvidenceStateSnapshotDTO, error) {
	return application.EvidenceStateSnapshotDTO{}, nil
}
func (*claimEvidenceRepositoryFake) ReadClaimEvidenceCorrectionTarget(context.Context, application.ClaimEvidenceCorrectionTargetQuery) (application.ClaimEvidenceCorrectionTargetDTO, error) {
	return application.ClaimEvidenceCorrectionTargetDTO{}, nil
}
func (*claimEvidenceRepositoryFake) CommitClaimEvidenceCorrection(context.Context, application.CommitClaimEvidenceCorrectionCommand) (application.CorrectClaimEvidenceResult, error) {
	return application.CorrectClaimEvidenceResult{}, nil
}

type microEventViewerAuthenticator struct{}

func (microEventViewerAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 8, SessionID: 9, Role: httptransport.RoleViewer}, nil
}

func TestMicroEventRoutesRequireAuthenticationAndProtectMutations(t *testing.T) {
	queries, governance, evidence := newMicroEventHTTPServices(t, &microEventQueryRepositoryFake{})
	gin.SetMode(gin.TestMode)
	unauthenticated := gin.New()
	RegisterMicroEventRoutes(unauthenticated, queries, governance, evidence, httptransport.NewUnavailableAuthenticator())
	recorder := httptest.NewRecorder()
	unauthenticated.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/micro-events", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", recorder.Code)
	}

	viewer := gin.New()
	RegisterMicroEventRoutes(viewer, queries, governance, evidence, microEventViewerAuthenticator{})
	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/micro-events/7/feedback", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set("Content-Type", "application/json")
	viewer.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("viewer mutation status = %d, want 403: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMicroEventReadProjectionOmitsTruthScoresAndRevokedQuotes(t *testing.T) {
	repository := &microEventQueryRepositoryFake{evidence: application.MicroEventEvidencePageDTO{Items: []application.ClaimEvidenceProjectionDTO{{
		ID: 13, Version: 2, ClaimID: 11, DocumentVersionID: 17, TextQuoteSelectorID: 19,
		ContentFamilyID: 23, LineageRootID: 17, ClaimSubject: "Acme", ClaimPredicate: "announced",
		ClaimObject: "Project", Relation: "asserts", Availability: "rights_unavailable",
		CapturedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC), ExtractionSchemaVersion: "atomic-claim-evidence-v2",
		DecisionOrigin: "manual", CreatedAt: time.Date(2026, 8, 10, 8, 1, 0, 0, time.UTC),
	}}}}
	queries, governance, evidence := newMicroEventHTTPServices(t, repository)
	router := gin.New()
	RegisterMicroEventRoutes(router, queries, governance, evidence, microEventViewerAuthenticator{})
	for _, target := range []string{"/api/v1/micro-events", "/api/v1/micro-events/7", "/api/v1/micro-events/7/evidence"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer viewer")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d: %s", target, recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		for _, forbidden := range []string{"credibility", "confirmation", "data_confidence", "verification", "truth", "model_relation_score", "exact_quote", "quote_sha256", "plaintext_sha256"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("GET %s leaked %q: %s", target, forbidden, body)
			}
		}
	}
}

func newMicroEventHTTPServices(t *testing.T, queryRepository *microEventQueryRepositoryFake) (*application.MicroEventQueryService, *application.MicroEventGovernanceService, *application.ClaimEvidenceService) {
	t.Helper()
	queries, err := application.NewMicroEventQueryService(queryRepository)
	if err != nil {
		t.Fatalf("new query service: %v", err)
	}
	governance, err := application.NewMicroEventGovernanceService(&microEventGovernanceRepositoryFake{})
	if err != nil {
		t.Fatalf("new governance service: %v", err)
	}
	evidence, err := application.NewClaimEvidenceService(&claimEvidenceRepositoryFake{})
	if err != nil {
		t.Fatalf("new evidence service: %v", err)
	}
	return queries, governance, evidence
}
