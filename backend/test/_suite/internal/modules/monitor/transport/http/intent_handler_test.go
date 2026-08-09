package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

func TestIntentDraftPutUsesExplicitInitializationOrStrongResourceCAS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &intentHTTPServiceFake{draft: intentHTTPDraft(1)}
	router := gin.New()
	RegisterIntentRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 17, SessionID: 1, Role: httptransport.RoleEditor}})
	body := `{"expected_resource_version":0,"objective":"Track launch disruption","clauses":[{"operator":"must","field":"action","value":"launch"}]}`

	for _, test := range []struct {
		name, ifMatch, ifNone string
		wantStatus            int
	}{
		{name: "missing condition", wantStatus: stdhttp.StatusBadRequest},
		{name: "invented v0", ifMatch: `"v0"`, wantStatus: stdhttp.StatusBadRequest},
		{name: "initialization", ifNone: "*", wantStatus: stdhttp.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(stdhttp.MethodPut, "/api/v1/monitors/7/draft/intent", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer editor")
			request.Header.Set("Content-Type", "application/json")
			if test.ifMatch != "" {
				request.Header.Set("If-Match", test.ifMatch)
			}
			if test.ifNone != "" {
				request.Header.Set("If-None-Match", test.ifNone)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == stdhttp.StatusCreated && (response.Header().Get("ETag") != `"v1"` || service.lastPut.Actor.UserID != 17 || service.lastPut.ExpectedResourceVersion != 0) {
				t.Fatalf("initialization headers/command = %q %#v", response.Header().Get("ETag"), service.lastPut)
			}
		})
	}

	service.draft = intentHTTPDraft(2)
	updateBody := strings.Replace(body, `"expected_resource_version":0`, `"expected_resource_version":1`, 1)
	request := httptest.NewRequest(stdhttp.MethodPut, "/api/v1/monitors/7/draft/intent", strings.NewReader(updateBody))
	request.Header.Set("Authorization", "Bearer editor")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", `"v1"`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusOK || response.Header().Get("ETag") != `"v2"` || service.lastPut.ExpectedResourceVersion != 1 {
		t.Fatalf("update response/command = %d %q %#v: %s", response.Code, response.Header().Get("ETag"), service.lastPut, response.Body.String())
	}
}

func TestIntentAsyncSubmitFailsClosedAndReturnsBoundedAcceptedProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &intentHTTPServiceFake{expansionSubmit: monitorapplication.SubmitExpansionRunResult{Run: intentHTTPRun("expansion"), Reused: false}}
	router := gin.New()
	RegisterIntentRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 9, SessionID: 2, Role: httptransport.RoleAdmin}})
	body := `{"expected_resource_version":4,"expansion_profile":"monitor-intent-expansion-v1"}`

	for _, test := range []struct {
		name, ifMatch, key string
		wantStatus         int
	}{
		{name: "missing both", wantStatus: stdhttp.StatusBadRequest},
		{name: "missing idempotency", ifMatch: `"v4"`, wantStatus: stdhttp.StatusBadRequest},
		{name: "weak etag", ifMatch: `W/"v4"`, key: "expand-once", wantStatus: stdhttp.StatusBadRequest},
		{name: "accepted", ifMatch: `"v4"`, key: "expand-once", wantStatus: stdhttp.StatusAccepted},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/monitors/7/draft/expansion-runs", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer admin")
			request.Header.Set("Content-Type", "application/json")
			if test.ifMatch != "" {
				request.Header.Set("If-Match", test.ifMatch)
			}
			if test.key != "" {
				request.Header.Set("Idempotency-Key", test.key)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == stdhttp.StatusAccepted {
				if response.Header().Get("Location") != "/api/v1/monitors/7/draft/expansion-runs/501" {
					t.Fatalf("Location = %q", response.Header().Get("Location"))
				}
				lower := strings.ToLower(response.Body.String())
				for _, forbidden := range []string{`"body"`, `"markdown"`, `"prompt"`, `"objective"`, `"raw"`} {
					if strings.Contains(lower, forbidden) {
						t.Fatalf("accepted response leaked %q: %s", forbidden, response.Body.String())
					}
				}
				for _, required := range []string{`"run_id":501`, `"draft_id":101`, `"resource_version":4`, `"input_hash"`, `"status_url"`} {
					if !strings.Contains(response.Body.String(), required) {
						t.Fatalf("accepted response missing %s: %s", required, response.Body.String())
					}
				}
			}
		})
	}
}

func TestIntentStatusAndCandidateDecisionUseSafeDTOsAndExactHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reviewedAt := time.Date(2026, 8, 9, 19, 0, 0, 0, time.UTC)
	reviewer := int64(9)
	candidate := monitorapplication.ExpansionCandidateDTO{
		ID: "launch-outage", Value: "launch outage", Source: "llm", Reason: "semantic neighbor",
		ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("a", 64),
		Similarity: 0.8, Risk: "low", ApprovalStatus: "approved", ReviewerUserID: &reviewer, ReviewedAt: &reviewedAt,
	}
	service := &intentHTTPServiceFake{
		draft: intentHTTPDraft(5),
		expansionRead: monitorapplication.ReadExpansionRunResult{Expansion: monitorapplication.ExpansionRunDTO{
			Run: intentHTTPRun("expansion"), Candidates: []monitorapplication.ExpansionCandidateDTO{candidate},
		}},
	}
	service.draft.Candidates = []monitorapplication.ExpansionCandidateDTO{candidate}
	router := gin.New()
	RegisterIntentRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 9, SessionID: 2, Role: httptransport.RoleAdmin}})

	statusRequest := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/monitors/7/draft/expansion-runs/501", nil)
	statusRequest.Header.Set("Authorization", "Bearer admin")
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, statusRequest)
	if statusResponse.Code != stdhttp.StatusOK || service.lastExpansionRead.RunID != 501 {
		t.Fatalf("status response/query = %d %#v: %s", statusResponse.Code, service.lastExpansionRead, statusResponse.Body.String())
	}
	if strings.Contains(strings.ToLower(statusResponse.Body.String()), `"prompt"`) || !strings.Contains(statusResponse.Body.String(), `"prompt_version":"prompt-v1"`) {
		t.Fatalf("status prompt projection is unsafe or missing version: %s", statusResponse.Body.String())
	}

	decisionRequest := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/monitors/7/draft/expansion-candidates/launch-outage/decision", strings.NewReader(`{"expected_resource_version":4,"decision":"approved","note":"reviewed"}`))
	decisionRequest.Header.Set("Authorization", "Bearer admin")
	decisionRequest.Header.Set("Content-Type", "application/json")
	decisionRequest.Header.Set("If-Match", `"v4"`)
	decisionRequest.Header.Set("Idempotency-Key", "candidate-review-once")
	decisionResponse := httptest.NewRecorder()
	router.ServeHTTP(decisionResponse, decisionRequest)
	if decisionResponse.Code != stdhttp.StatusOK || decisionResponse.Header().Get("ETag") != `"v5"` || service.lastReview.CandidateID != "launch-outage" || service.lastReview.IdempotencyKey != "candidate-review-once" {
		t.Fatalf("decision response/command = %d %q %#v: %s", decisionResponse.Code, decisionResponse.Header().Get("ETag"), service.lastReview, decisionResponse.Body.String())
	}
}

func TestIntentStatusMapperSanitizesFailureAndNamesChannelScoreRaw(t *testing.T) {
	t.Parallel()
	run := intentHTTPRun("preview")
	run.Status = "failed"
	run.FailureReason = "provider returned SECRET prompt and internal endpoint"
	response := intentPreviewRunStatusResponseDTO(monitorapplication.PreviewRunDTO{
		Run: run,
		Preview: &monitorapplication.IntentPreviewDTO{Samples: []monitorapplication.PreviewSampleDTO{{
			DocumentVersionID: 41, Title: "Launch update", Decision: "review",
			RecallSignals: []monitorapplication.PreviewRecallSignalDTO{{Channel: "lexical", Rank: 1, Score: 8.75}},
		}}},
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	projection := string(encoded)
	if strings.Contains(projection, "SECRET") || strings.Contains(projection, "provider returned") || !strings.Contains(projection, `"failure_code":"analysis_failed"`) {
		t.Fatalf("failure projection = %s", projection)
	}
	if !strings.Contains(projection, `"raw_score":8.75`) || strings.Contains(projection, `"score":`) {
		t.Fatalf("recall signal projection = %s", projection)
	}
}

func intentHTTPDraft(version int64) monitorapplication.IntentDraftDTO {
	return monitorapplication.IntentDraftDTO{
		MonitorID: 7, DraftID: 101, ResourceVersion: version, Objective: "Track launch disruption",
		Clauses: []monitorapplication.IntentClauseDTO{{Operator: "must", Field: "action", Value: "launch"}},
	}
}

func intentHTTPRun(kind string) monitorapplication.IntentRunDTO {
	return monitorapplication.IntentRunDTO{
		ID: 501, Kind: kind, MonitorID: 7, DraftID: 101, DraftResourceVersion: 4,
		InputHash: strings.Repeat("a", 64), Status: "queued", QueuedAt: time.Date(2026, 8, 9, 18, 0, 0, 0, time.UTC),
	}
}

type intentHTTPServiceFake struct {
	draft             monitorapplication.IntentDraftDTO
	expansionSubmit   monitorapplication.SubmitExpansionRunResult
	previewSubmit     monitorapplication.SubmitPreviewRunResult
	expansionRead     monitorapplication.ReadExpansionRunResult
	previewRead       monitorapplication.ReadPreviewRunResult
	lastPut           monitorapplication.PutCurrentIntentDraftCommand
	lastReview        monitorapplication.ReviewCurrentExpansionCandidateCommand
	lastExpansionRead monitorapplication.ReadIntentExpansionRunQuery
}

func (service *intentHTTPServiceFake) ReadDraft(_ context.Context, _ monitorapplication.ReadCurrentIntentDraftQuery) (monitorapplication.ReadCurrentIntentDraftResult, error) {
	return monitorapplication.ReadCurrentIntentDraftResult{Draft: service.draft}, nil
}
func (service *intentHTTPServiceFake) PutDraft(_ context.Context, command monitorapplication.PutCurrentIntentDraftCommand) (monitorapplication.PutCurrentIntentDraftResult, error) {
	service.lastPut = command
	return monitorapplication.PutCurrentIntentDraftResult{Draft: service.draft, Created: command.ExpectedResourceVersion == 0}, nil
}
func (service *intentHTTPServiceFake) ReviewExpansionCandidate(_ context.Context, command monitorapplication.ReviewCurrentExpansionCandidateCommand) (monitorapplication.ReviewExpansionCandidateResult, error) {
	service.lastReview = command
	return monitorapplication.ReviewExpansionCandidateResult{Draft: service.draft}, nil
}
func (service *intentHTTPServiceFake) SubmitExpansionRun(_ context.Context, _ monitorapplication.SubmitCurrentExpansionRunCommand) (monitorapplication.SubmitExpansionRunResult, error) {
	return service.expansionSubmit, nil
}
func (service *intentHTTPServiceFake) SubmitPreviewRun(_ context.Context, _ monitorapplication.SubmitCurrentPreviewRunCommand) (monitorapplication.SubmitPreviewRunResult, error) {
	return service.previewSubmit, nil
}
func (service *intentHTTPServiceFake) ReadExpansionRun(_ context.Context, query monitorapplication.ReadIntentExpansionRunQuery) (monitorapplication.ReadExpansionRunResult, error) {
	service.lastExpansionRead = query
	return service.expansionRead, nil
}
func (service *intentHTTPServiceFake) ReadPreviewRun(_ context.Context, _ monitorapplication.ReadIntentPreviewRunQuery) (monitorapplication.ReadPreviewRunResult, error) {
	return service.previewRead, nil
}

func decodeIntentResultData(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatal(err)
	}
}
