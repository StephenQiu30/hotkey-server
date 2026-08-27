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
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type microEventQueryRepositoryFake struct {
	evidence application.MicroEventEvidencePageDTO
	lastList application.MicroEventListQuery
}

func (fake *microEventQueryRepositoryFake) ListMicroEvents(_ context.Context, query application.MicroEventListQuery) (application.MicroEventPageDTO, error) {
	fake.lastList = query
	return application.MicroEventPageDTO{Items: []application.MicroEventProjectionDTO{{
		ID: 7, Version: 3, EventKey: "semantic-event", Status: "active",
		PrimarySubjectKey: "acme", PrimaryActionKey: "announced", EventStartedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
		ClusteringProfileVersion: "micro-event-clustering-v1", ContentFamilyCount: 2, DocumentCount: 3,
	}}}, nil
}

func TestMicroEventListForwardsMultidimensionalFiltersAndOpaqueCursor(t *testing.T) {
	repository := &microEventQueryRepositoryFake{}
	queries, governance, evidence := newMicroEventHTTPServices(t, repository)
	router := gin.New()
	RegisterMicroEventRoutes(router, queries, governance, evidence, microEventViewerAuthenticator{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/micro-events?sort=relevance&cursor=fixture-cursor&limit=30&status=active,closed&monitor_id=17&source_type=x,hacker_news&evidence_state=multiple_origins,conflicting_reports&started_from=2026-08-01T00%3A00%3A00Z&started_to=2026-08-12T12%3A00%3A00Z", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if repository.lastList.Sort != "relevance" || repository.lastList.Cursor != "fixture-cursor" || repository.lastList.Limit != 30 ||
		repository.lastList.MonitorID != 17 || len(repository.lastList.Statuses) != 2 || len(repository.lastList.SourceTypes) != 2 ||
		len(repository.lastList.EvidenceStates) != 2 || repository.lastList.StartedFrom == nil || repository.lastList.StartedTo == nil ||
		!repository.lastList.StartedFrom.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) ||
		!repository.lastList.StartedTo.Equal(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("list query = %#v", repository.lastList)
	}
}

func TestMicroEventListRejectsInvalidMultidimensionalFilters(t *testing.T) {
	queries, governance, evidence := newMicroEventHTTPServices(t, &microEventQueryRepositoryFake{})
	router := gin.New()
	RegisterMicroEventRoutes(router, queries, governance, evidence, microEventViewerAuthenticator{})
	for _, target := range []string{
		"/api/v1/micro-events?monitor_id=0",
		"/api/v1/micro-events?source_type=unknown",
		"/api/v1/micro-events?evidence_state=verified",
		"/api/v1/micro-events?started_from=yesterday",
		"/api/v1/micro-events?started_from=2026-08-12T12%3A00%3A00Z&started_to=2026-08-01T00%3A00%3A00Z",
		"/api/v1/micro-events?sort=importance",
	} {
		t.Run(target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("Authorization", "Bearer viewer")
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("GET %s status = %d, want 400: %s", target, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMicroEventResponseExposesHeatProfileAvailabilityAndWarmingUp(t *testing.T) {
	projection := application.MicroEventProjectionDTO{ID: 7, Version: 3, EventKey: "semantic-event", Status: "active",
		PrimarySubjectKey: "acme", PrimaryActionKey: "announced", EventStartedAt: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
		ClusteringProfileVersion: "micro-event-clustering-v1", LatestHeat: &application.EventHeatSnapshotDTO{
			ID: 11, MicroEventVersion: 3, HeatProfileVersion: "event-heat-v2-golden",
			WindowStartedAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), WindowEndedAt: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
			IndependentLineageRoots: 4, Velocity: 0, Acceleration: 0, Coverage: .55, Recency: .92,
			AvailableWeight: .5, HeatScore: 72, WarmingUp: true, ReasonCodes: []string{"warming_up", "metrics_unavailable"},
		}}
	response := microEventResponseDTO(projection)
	if response.LatestHeat == nil || response.LatestHeat.HeatProfileVersion != "event-heat-v2-golden" ||
		response.LatestHeat.AvailableWeight != .5 || !response.LatestHeat.WarmingUp ||
		response.LatestHeat.Velocity != 0 || response.LatestHeat.Acceleration != 0 {
		t.Fatalf("heat response = %#v", response.LatestHeat)
	}
}

func TestMicroEventResponseExposesOnlyActiveGovernanceMemberHandles(t *testing.T) {
	response := microEventResponseDTO(application.MicroEventProjectionDTO{Members: []application.MicroEventMemberProjectionDTO{{
		ID: 19, Version: 2, ContentFamilyID: 23, MembershipDecisionID: 29,
		ClusteringProfileVersion: "micro-event-clustering-v1",
	}}})
	if len(response.Members) != 1 || response.Members[0].ID != 19 || response.Members[0].Version != 2 ||
		response.Members[0].ContentFamilyID != 23 || response.Members[0].MembershipDecisionID != 29 ||
		response.Members[0].ClusteringProfileVersion != "micro-event-clustering-v1" {
		t.Fatalf("governance member handles = %#v", response.Members)
	}
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

type microEventGovernanceRepositoryFake struct {
	calls int
}

func (fake *microEventGovernanceRepositoryFake) ApplyMicroEventGovernance(_ context.Context, command application.ApplyMicroEventGovernanceCommand) (application.ApplyMicroEventGovernanceResult, error) {
	fake.calls++
	return application.ApplyMicroEventGovernanceResult{
		Feedback: application.MicroEventGovernanceFeedbackDTO{
			ID: 101, Action: command.Action, ActorUserID: command.ActorUserID,
			MicroEventID: command.MicroEventID, OriginalEventVersion: command.ExpectedEventVersion,
			MembershipDecisionID: command.MembershipDecisionID, ContentFamilyID: command.ContentFamilyID,
			TargetMicroEventID: command.TargetMicroEventID, TargetEventVersion: command.ExpectedTargetEventVersion,
			ResultMicroEventID: command.MicroEventID, ResultEventVersion: command.ExpectedEventVersion + 1,
			GovernanceProfileVersion: command.GovernanceProfileVersion, ReasonCode: command.ReasonCode,
			Note: command.Note, IdempotencyKey: command.IdempotencyKey,
		},
		SourceEvent: application.MicroEventDTO{
			ID: command.MicroEventID, Version: command.ExpectedEventVersion + 1, Status: "closed",
		},
	}, nil
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

type microEventRoleAuthenticator struct {
	role httptransport.Role
}

func (authenticator microEventRoleAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 8, SessionID: 9, Role: authenticator.role}, nil
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
	for _, path := range []string{
		"/api/v1/micro-events/7/feedback",
		"/api/v1/micro-events/7/evidence",
		"/api/v1/micro-events/7/evidence/11/feedback",
	} {
		recorder = httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer viewer")
		request.Header.Set("Content-Type", "application/json")
		viewer.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("viewer mutation %s status = %d, want 403: %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestMicroEventRoutesEnforceFourRoleReadAndGovernanceBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		role             httptransport.Role
		wantGovernStatus int
		wantWrites       int
	}{
		{role: httptransport.RoleViewer, wantGovernStatus: http.StatusForbidden},
		{role: httptransport.RoleAnalyst, wantGovernStatus: http.StatusForbidden},
		{role: httptransport.RoleEditor, wantGovernStatus: http.StatusOK, wantWrites: 1},
		{role: httptransport.RoleAdmin, wantGovernStatus: http.StatusOK, wantWrites: 1},
	} {
		t.Run(string(testCase.role), func(t *testing.T) {
			governanceRepository := &microEventGovernanceRepositoryFake{}
			queries, governance, evidence := newMicroEventHTTPServicesWithGovernanceRepository(
				t, &microEventQueryRepositoryFake{}, governanceRepository,
			)
			router := gin.New()
			RegisterMicroEventRoutes(router, queries, governance, evidence, microEventRoleAuthenticator{role: testCase.role})

			readRecorder := httptest.NewRecorder()
			readRequest := httptest.NewRequest(http.MethodGet, "/api/v1/micro-events", nil)
			readRequest.Header.Set("Authorization", "Bearer "+string(testCase.role))
			router.ServeHTTP(readRecorder, readRequest)
			if readRecorder.Code != http.StatusOK {
				t.Fatalf("%s read status = %d, want 200: %s", testCase.role, readRecorder.Code, readRecorder.Body.String())
			}

			governRecorder := httptest.NewRecorder()
			governRequest := httptest.NewRequest(http.MethodPost, "/api/v1/micro-events/7/feedback", strings.NewReader(
				`{"expected_event_version":3,"action":"close_event","reason_code":"reviewed"}`,
			))
			governRequest.Header.Set("Authorization", "Bearer "+string(testCase.role))
			governRequest.Header.Set("Content-Type", "application/json")
			governRequest.Header.Set("If-Match", `"v3"`)
			governRequest.Header.Set("Idempotency-Key", "govern-event-7-v3-"+string(testCase.role))
			router.ServeHTTP(governRecorder, governRequest)
			if governRecorder.Code != testCase.wantGovernStatus {
				t.Fatalf("%s govern status = %d, want %d: %s", testCase.role, governRecorder.Code, testCase.wantGovernStatus, governRecorder.Body.String())
			}
			if governanceRepository.calls != testCase.wantWrites {
				t.Fatalf("%s governance writes = %d, want %d", testCase.role, governanceRepository.calls, testCase.wantWrites)
			}
		})
	}
}

func TestEventErrorMapsMicroEventGovernanceFailures(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   int
	}{
		{name: "invalid governance", err: application.ErrInvalidMicroEventGovernanceContract, wantStatus: http.StatusBadRequest, wantCode: 10000},
		{name: "forbidden governance", err: application.ErrMicroEventGovernanceForbidden, wantStatus: http.StatusForbidden, wantCode: 20001},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mapped := eventError(testCase.err)
			appError, ok := mapped.(*sharederrors.AppError)
			if !ok || appError.HTTPStatus != testCase.wantStatus || appError.Code != testCase.wantCode {
				t.Fatalf("eventError(%v) = %#v", testCase.err, mapped)
			}
		})
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
	return newMicroEventHTTPServicesWithGovernanceRepository(t, queryRepository, &microEventGovernanceRepositoryFake{})
}

func newMicroEventHTTPServicesWithGovernanceRepository(t *testing.T, queryRepository *microEventQueryRepositoryFake, governanceRepository application.MicroEventGovernanceRepository) (*application.MicroEventQueryService, *application.MicroEventGovernanceService, *application.ClaimEvidenceService) {
	t.Helper()
	queries, err := application.NewMicroEventQueryService(queryRepository)
	if err != nil {
		t.Fatalf("new query service: %v", err)
	}
	governance, err := application.NewMicroEventGovernanceService(governanceRepository)
	if err != nil {
		t.Fatalf("new governance service: %v", err)
	}
	evidence, err := application.NewClaimEvidenceService(&claimEvidenceRepositoryFake{})
	if err != nil {
		t.Fatalf("new evidence service: %v", err)
	}
	return queries, governance, evidence
}
