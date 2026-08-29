package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type rightsManagementHTTPServiceFake struct {
	capability sourceapplication.SourceEndpointCapabilityDTO
	policies   sourceapplication.ListRightsPoliciesResult
	batches    sourceapplication.ListRightsDecisionBatchesResult
	decision   sourceapplication.RightsDecisionReadDTO
	matrix     sourceapplication.RightsActionMatrixDTO
	policy     sourceapplication.CreateRightsPolicyResult
	recorded   sourceapplication.RecordRightsDecisionResult
	err        error

	capabilityCalls int
	policyListCalls int
	batchListCalls  int
	decisionCalls   int
	evaluationCalls int
	createCalls     int
	recordCalls     int

	lastCapability sourceapplication.GetSourceEndpointCapabilityQuery
	lastPolicies   sourceapplication.ListRightsPoliciesQuery
	lastBatches    sourceapplication.ListRightsDecisionBatchesQuery
	lastDecision   sourceapplication.GetRightsDecisionQuery
	lastEvaluation sourceapplication.EvaluateRightsActionMatrixQuery
	lastCreate     sourceapplication.CreateRightsPolicyCommand
	lastRecord     sourceapplication.RecordRightsDecisionCommand
}

func (service *rightsManagementHTTPServiceFake) GetSourceEndpointCapability(_ context.Context, query sourceapplication.GetSourceEndpointCapabilityQuery) (sourceapplication.SourceEndpointCapabilityDTO, error) {
	service.capabilityCalls++
	service.lastCapability = query
	return service.capability, service.err
}

func (service *rightsManagementHTTPServiceFake) ListRightsPolicies(_ context.Context, query sourceapplication.ListRightsPoliciesQuery) (sourceapplication.ListRightsPoliciesResult, error) {
	service.policyListCalls++
	service.lastPolicies = query
	return service.policies, service.err
}

func (service *rightsManagementHTTPServiceFake) ListRightsDecisionBatches(_ context.Context, query sourceapplication.ListRightsDecisionBatchesQuery) (sourceapplication.ListRightsDecisionBatchesResult, error) {
	service.batchListCalls++
	service.lastBatches = query
	return service.batches, service.err
}

func (service *rightsManagementHTTPServiceFake) GetRightsDecision(_ context.Context, query sourceapplication.GetRightsDecisionQuery) (sourceapplication.RightsDecisionReadDTO, error) {
	service.decisionCalls++
	service.lastDecision = query
	return service.decision, service.err
}

func (service *rightsManagementHTTPServiceFake) EvaluateRightsActionMatrix(_ context.Context, query sourceapplication.EvaluateRightsActionMatrixQuery) (sourceapplication.RightsActionMatrixDTO, error) {
	service.evaluationCalls++
	service.lastEvaluation = query
	return service.matrix, service.err
}

func (service *rightsManagementHTTPServiceFake) CreatePolicy(_ context.Context, command sourceapplication.CreateRightsPolicyCommand) (sourceapplication.CreateRightsPolicyResult, error) {
	service.createCalls++
	service.lastCreate = command
	return service.policy, service.err
}

func (service *rightsManagementHTTPServiceFake) RecordDecisions(_ context.Context, command sourceapplication.RecordRightsDecisionCommand) (sourceapplication.RecordRightsDecisionResult, error) {
	service.recordCalls++
	service.lastRecord = command
	return service.recorded, service.err
}

func TestRightsManagementRoutesSeparatePublicCapabilityFromAdministratorFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		role       httptransport.Role
		method     string
		path       string
		body       string
		wantStatus int
		wantCall   string
	}{
		{name: "viewer public capability", role: httptransport.RoleViewer, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/capabilities", wantStatus: 200, wantCall: "capability"},
		{name: "editor public capability", role: httptransport.RoleEditor, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/capabilities", wantStatus: 200, wantCall: "capability"},
		{name: "viewer policy history", role: httptransport.RoleViewer, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/rights-policies", wantStatus: 403},
		{name: "editor decision history", role: httptransport.RoleEditor, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/rights-decision-batches", wantStatus: 403},
		{name: "viewer policy mutation", role: httptransport.RoleViewer, method: stdhttp.MethodPost, path: "/api/v1/source-endpoints/42/rights-policies", body: `{}`, wantStatus: 403},
		{name: "viewer exact evaluation", role: httptransport.RoleViewer, method: stdhttp.MethodPost, path: "/api/v1/source-endpoints/42/rights-evaluations", body: `{}`, wantStatus: 403},
		{name: "admin policy history", role: httptransport.RoleAdmin, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/rights-policies", wantStatus: 200, wantCall: "policies"},
		{name: "admin batch history", role: httptransport.RoleAdmin, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/rights-decision-batches", wantStatus: 200, wantCall: "batches"},
		{name: "admin decision detail", role: httptransport.RoleAdmin, method: stdhttp.MethodGet, path: "/api/v1/source-endpoints/42/rights-decisions/91", wantStatus: 200, wantCall: "decision"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &rightsManagementHTTPServiceFake{capability: publicRightsCapability(42)}
			router := gin.New()
			RegisterRightsManagementRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 1, Role: test.role}})
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer member")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("Cache-Control = %q, want private, no-store", response.Header().Get("Cache-Control"))
			}
			if test.wantCall == "capability" {
				body := strings.ToLower(response.Body.String())
				for _, forbidden := range []string{`"allow"`, "subject_key", "input_digest", "allow_body_storage"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("public capability leaked authorizing field %q: %s", forbidden, response.Body.String())
					}
				}
			}
			calls := map[string]int{
				"capability": service.capabilityCalls, "policies": service.policyListCalls,
				"batches": service.batchListCalls, "decision": service.decisionCalls,
				"create": service.createCalls, "record": service.recordCalls,
			}
			for name, count := range calls {
				want := 0
				if name == test.wantCall {
					want = 1
				}
				if count != want {
					t.Fatalf("%s calls = %d, want %d", name, count, want)
				}
			}
		})
	}
}

func TestRightsManagementMutationRejectsUnauthenticatedBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &rightsManagementHTTPServiceFake{}
	router := gin.New()
	RegisterRightsManagementRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 1, Role: httptransport.RoleAdmin}})

	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-policies", strings.NewReader(`{"scope_type":"source_endpoint"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertRightsError(t, response, stdhttp.StatusUnauthorized, sharederrors.CodeUnauthenticated)
	if service.createCalls != 0 || service.recordCalls != 0 {
		t.Fatalf("unauthenticated mutation reached service: create=%d record=%d", service.createCalls, service.recordCalls)
	}
}

func TestRightsManagementMutationsRequireIdempotencyAndMatchingPolicyETag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.August, 9, 13, 0, 0, 0, time.UTC)
	policyResponse := sourceapplication.RightsPolicyDTO{
		ID: 71, Version: 1, SourceConnectionID: rightsTransportInt64Pointer(42), ScopeType: "source_endpoint",
		ScopeSubject: "feed-main", Revision: 7, Priority: 300, BasisSummary: "approved feed terms",
		TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:feed-main",
		PolicyHash: strings.Repeat("a", 64), EffectiveFrom: now, ApprovedByUserID: rightsTransportInt64Pointer(7),
	}

	createBody := `{"scope_type":"source_endpoint","scope_subject":"feed-main","revision":7,"priority":300,"basis_summary":"approved feed terms","terms_url":"https://publisher.example.test/terms","license_uri":"urn:license:feed-main","effective_from":"2026-08-09T13:00:00Z","approved_by_user_id":7}`
	service := &rightsManagementHTTPServiceFake{policy: sourceapplication.CreateRightsPolicyResult{Policy: policyResponse}}
	router := newRightsManagementRouter(service, httptransport.RoleAdmin)
	missingKey := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-policies", createBody, map[string]string{})
	if missingKey.Code != stdhttp.StatusBadRequest || service.createCalls != 0 {
		t.Fatalf("missing idempotency status/calls = %d/%d: %s", missingKey.Code, service.createCalls, missingKey.Body.String())
	}
	created := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-policies", createBody, map[string]string{"Idempotency-Key": "rights-policy-http-1"})
	if created.Code != stdhttp.StatusCreated || service.createCalls != 1 || service.lastCreate.ActorID != 7 ||
		service.lastCreate.SourceConnectionID == nil || *service.lastCreate.SourceConnectionID != 42 ||
		service.lastCreate.IdempotencyKey != "rights-policy-http-1" || created.Header().Get("ETag") != `"v1"` {
		t.Fatalf("create policy response/command = %d %s %#v", created.Code, created.Body.String(), service.lastCreate)
	}

	decisionBody := `{"policy_id":71,"expected_policy_version":1,"subject_type":"raw_response","subject_key":"` + strings.Repeat("b", 64) + `","input_digest":"` + strings.Repeat("c", 64) + `","decisions":[{"action":"store_raw","decision":"allow","reason_codes":["terms_confirmed"],"evaluator":"rights-admin","evaluated_at":"2026-08-09T13:00:00Z","effective_from":"2026-08-09T13:00:00Z"}]}`
	missingMatch := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-decision-batches", decisionBody, map[string]string{"Idempotency-Key": "rights-decision-http-1"})
	if missingMatch.Code != stdhttp.StatusBadRequest || service.recordCalls != 0 {
		t.Fatalf("missing If-Match status/calls = %d/%d: %s", missingMatch.Code, service.recordCalls, missingMatch.Body.String())
	}
	mismatch := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-decision-batches", decisionBody, map[string]string{
		"Idempotency-Key": "rights-decision-http-1", "If-Match": `"v2"`,
	})
	if mismatch.Code != stdhttp.StatusConflict || service.recordCalls != 0 {
		t.Fatalf("mismatched If-Match status/calls = %d/%d: %s", mismatch.Code, service.recordCalls, mismatch.Body.String())
	}
	service.recorded = sourceapplication.RecordRightsDecisionResult{DecisionBatchID: 90, Decisions: []sourceapplication.RightsDecisionDTO{{
		ID: 91, DecisionBatchID: 90, SourceConnectionID: 42, PolicyID: 71, PolicyRevision: 7,
		PolicyScopeType: "source_endpoint", PolicyScopeSubject: "feed-main", Priority: 300,
		BasisSummary: "approved feed terms", SubjectType: "raw_response", SubjectKey: strings.Repeat("b", 64),
		InputDigest: strings.Repeat("c", 64), Action: "store_raw", Decision: "allow",
		ReasonCodes: []string{"terms_confirmed"}, Evaluator: "rights-admin", EvaluatedAt: now, EffectiveFrom: now,
	}}}
	recorded := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-decision-batches", decisionBody, map[string]string{
		"Idempotency-Key": "rights-decision-http-1", "If-Match": `"v1"`,
	})
	if recorded.Code != stdhttp.StatusCreated || service.recordCalls != 1 || service.lastRecord.ActorID != 7 ||
		service.lastRecord.SourceConnectionID != 42 || service.lastRecord.ExpectedPolicyVersion != 1 ||
		service.lastRecord.IdempotencyKey != "rights-decision-http-1" || len(service.lastRecord.Decisions) != 1 {
		t.Fatalf("record decisions response/command = %d %s %#v", recorded.Code, recorded.Body.String(), service.lastRecord)
	}
	assertRightsResponseHasNoInternalFields(t, created.Body.String())
	assertRightsResponseHasNoInternalFields(t, recorded.Body.String())
}

func TestRightsEvaluationKeepsExactIdentityOutOfURLResponseAndAccessLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	telemetry, err := observability.NewTelemetry(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	service := &rightsManagementHTTPServiceFake{matrix: sourceapplication.RightsActionMatrixDTO{
		SourceEndpointID: 42, EvaluatedAt: time.Date(2026, time.August, 9, 13, 0, 0, 0, time.UTC),
		Actions: []sourceapplication.RightsActionCapabilityDTO{{Action: "fetch", Decision: "allow", DecisionIDs: []int64{91}, PolicyIDs: []int64{71}, Priority: rightsTransportIntPointer(300)}},
	}}
	router := httptransport.NewRouter(httptransport.ReadinessFunc(func(context.Context) error { return nil }), metrics, telemetry, zap.New(core), cfg)
	RegisterRightsManagementRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 1, Role: httptransport.RoleAdmin}})
	subjectMarker := "subject-marker-private-7d9"
	digestMarker := strings.Repeat("d", 64)
	body := `{"subject_type":"raw_response","subject_key":"` + subjectMarker + `","input_digest":"` + digestMarker + `","at":"2026-08-09T13:00:00Z"}`
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-evaluations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK || service.evaluationCalls != 1 || service.lastEvaluation.SubjectKey != subjectMarker || service.lastEvaluation.InputDigest != digestMarker {
		t.Fatalf("evaluation status/query = %d %#v: %s", response.Code, service.lastEvaluation, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("evaluation Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	for _, forbidden := range []string{subjectMarker, digestMarker, "input_digest", "subject_key", "object_key", "allow_body_storage"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("evaluation response leaked %q: %s", forbidden, response.Body.String())
		}
		for _, entry := range logs.All() {
			encoded, marshalErr := json.Marshal(entry.ContextMap())
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(entry.Message, forbidden) || strings.Contains(string(encoded), forbidden) {
				t.Fatalf("access log leaked %q: %s %s", forbidden, entry.Message, encoded)
			}
		}
	}
}

func TestRightsManagementTransportMapsStableRepositoryFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &rightsManagementHTTPServiceFake{err: sharedrepository.ErrNotFound}
	router := newRightsManagementRouter(service, httptransport.RoleAdmin)
	notFound := performRightsManagementRequest(router, stdhttp.MethodGet, "/api/v1/source-endpoints/42/rights-policies", "", nil)
	assertRightsError(t, notFound, stdhttp.StatusNotFound, sharederrors.CodeNotFound)

	service.err = sharedrepository.ErrConflict
	body := `{"scope_type":"source_endpoint","scope_subject":"feed-main","revision":1,"priority":300,"basis_summary":"approved","effective_from":"2026-08-09T13:00:00Z"}`
	conflict := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-policies", body, map[string]string{"Idempotency-Key": "rights-policy-conflict"})
	assertRightsError(t, conflict, stdhttp.StatusConflict, sharederrors.CodeConflict)

	service.err = sharedrepository.ErrNotFound
	decisionBody := `{"policy_id":71,"expected_policy_version":2,"subject_type":"raw_response","subject_key":"` + strings.Repeat("b", 64) + `","input_digest":"` + strings.Repeat("c", 64) + `","decisions":[{"action":"store_raw","decision":"allow","reason_codes":["terms_confirmed"],"evaluator":"rights-admin","evaluated_at":"2026-08-09T13:00:00Z","effective_from":"2026-08-09T13:00:00Z"}]}`
	stale := performRightsManagementRequest(router, stdhttp.MethodPost, "/api/v1/source-endpoints/42/rights-decision-batches", decisionBody, map[string]string{
		"Idempotency-Key": "rights-decision-stale", "If-Match": `"v2"`,
	})
	assertRightsError(t, stale, stdhttp.StatusConflict, sharederrors.CodeConflict)

	service.err = sharederrors.New(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "")
	forbidden := performRightsManagementRequest(router, stdhttp.MethodGet, "/api/v1/source-endpoints/42/capabilities", "", nil)
	assertRightsError(t, forbidden, stdhttp.StatusForbidden, sharederrors.CodeForbidden)
}

func TestRightsManagementResponseDTOsCannotCarryRawOrSecretFields(t *testing.T) {
	t.Parallel()
	for _, response := range []any{
		SourceEndpointCapabilityResponseDTO{}, RightsPolicyResponseDTO{}, RightsDecisionResponseDTO{},
		RightsDecisionBatchResponseDTO{}, RightsActionMatrixResponseDTO{},
	} {
		typeOf := reflect.TypeOf(response)
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			name := strings.ToLower(field.Name + " " + field.Tag.Get("json"))
			for _, forbidden := range []string{"object_key", "credential", "raw_body", "raw_payload", "allow_body_storage", "command_fingerprint", "idempotency_key"} {
				if strings.Contains(name, forbidden) {
					t.Fatalf("%s exposes forbidden field %s", typeOf.Name(), field.Name)
				}
			}
		}
	}
}

func publicRightsCapability(sourceEndpointID int64) sourceapplication.SourceEndpointCapabilityDTO {
	return sourceapplication.SourceEndpointCapabilityDTO{
		SourceEndpointID: sourceEndpointID, SourceType: "rss", CollectionInterface: "rss_atom_feed",
		ContentScope: "feed_payload", DocumentCaptureMode: "policy_gated_body", DefaultAccessMode: "metadata_only",
		RequiredActions: []string{"fetch", "store_raw", "store_derived", "display_private", "quote", "embed_local", "retain"},
		Availability:    "available", RightsStatus: "policy_required",
	}
}

func newRightsManagementRouter(service rightsManagementHTTPService, role httptransport.Role) *gin.Engine {
	router := gin.New()
	RegisterRightsManagementRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 7, SessionID: 1, Role: role}})
	return router
}

func performRightsManagementRequest(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer member")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertRightsResponseHasNoInternalFields(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{"object_key", "credential", "raw_body", "raw_payload", "allow_body_storage", "command_fingerprint", "idempotency_key"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}

func assertRightsError(t *testing.T, response *httptest.ResponseRecorder, status, code int) {
	t.Helper()
	var result struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode error result: %v (%s)", err, response.Body.String())
	}
	if response.Code != status || result.Code != code || string(result.Data) != "null" {
		t.Fatalf("error result = status:%d body:%s, want status:%d code:%d data:null", response.Code, response.Body.String(), status, code)
	}
}

func rightsTransportIntPointer(value int) *int       { return &value }
func rightsTransportInt64Pointer(value int64) *int64 { return &value }
