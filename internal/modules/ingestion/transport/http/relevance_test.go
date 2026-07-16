package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/application"
	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestPlan009RelevanceRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &relevanceHTTPStub{snapshot: plan009HTTPSnapshot(), content: plan009HTTPContent()}

	t.Run("unauthenticated request stops before application", func(t *testing.T) {
		router := newRelevanceRouter(stub, httptransport.RoleViewer)
		response := performRelevanceRequest(router, stdhttp.MethodGet, "/api/v1/monitors/1/matches", "", "")
		if response.Code != stdhttp.StatusUnauthorized || stub.listCalls != 0 {
			t.Fatalf("unauthenticated status/calls = %d/%d, want 401/0", response.Code, stub.listCalls)
		}
		assertRelevanceError(t, response, sharederrors.CodeUnauthenticated)
	})

	for _, test := range []struct {
		name, method, path, body string
		role                     httptransport.Role
		status                   int
	}{
		{name: "viewer lists matches", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/matches?limit=1", role: httptransport.RoleViewer, status: stdhttp.StatusOK},
		{name: "viewer gets explanation", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/matches/7", role: httptransport.RoleViewer, status: stdhttp.StatusOK},
		{name: "admin reads suggestions", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/feedback/suggestions?limit=1", role: httptransport.RoleAdmin, status: stdhttp.StatusOK},
		{name: "editor previews with no side effects", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/relevance-preview", role: httptransport.RoleEditor, status: stdhttp.StatusOK},
		{name: "editor writes match feedback", method: stdhttp.MethodPut, path: "/api/v1/monitors/1/matches/7/feedback", body: `{"feedback_type":"relevant","expected_feedback_version":null}`, role: httptransport.RoleEditor, status: stdhttp.StatusOK},
		{name: "editor writes false-negative content feedback", method: stdhttp.MethodPut, path: "/api/v1/monitors/1/contents/3/feedback", body: `{"expected_feedback_version":null}`, role: httptransport.RoleEditor, status: stdhttp.StatusOK},
		{name: "admin reads evaluation", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/feedback/evaluation", role: httptransport.RoleAdmin, status: stdhttp.StatusOK},
		{name: "admin refreshes suggestions", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/feedback/suggestions/refresh", role: httptransport.RoleAdmin, status: stdhttp.StatusOK},
		{name: "admin reviews suggestion", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/feedback/suggestions/9/review", body: `{"expected_version":1,"status":"approved"}`, role: httptransport.RoleAdmin, status: stdhttp.StatusOK},
		{name: "viewer cannot preview", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/relevance-preview", role: httptransport.RoleViewer, status: stdhttp.StatusForbidden},
		{name: "editor cannot evaluate", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/feedback/evaluation", role: httptransport.RoleEditor, status: stdhttp.StatusForbidden},
		{name: "editor cannot refresh", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/feedback/suggestions/refresh", role: httptransport.RoleEditor, status: stdhttp.StatusForbidden},
		{name: "viewer cannot read suggestions", method: stdhttp.MethodGet, path: "/api/v1/monitors/1/feedback/suggestions", role: httptransport.RoleViewer, status: stdhttp.StatusForbidden},
		{name: "editor cannot review suggestion", method: stdhttp.MethodPost, path: "/api/v1/monitors/1/feedback/suggestions/9/review", body: `{"expected_version":1,"status":"approved"}`, role: httptransport.RoleEditor, status: stdhttp.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := newRelevanceRouter(stub, test.role)
			response := performRelevanceRequest(router, test.method, test.path, test.body, "member")
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			if test.status == stdhttp.StatusForbidden {
				assertRelevanceError(t, response, sharederrors.CodeForbidden)
				return
			}
			assertRelevanceSuccess(t, response)
		})
	}

	t.Run("content feedback rejects every general feedback type", func(t *testing.T) {
		router := newRelevanceRouter(stub, httptransport.RoleEditor)
		for _, feedbackType := range []string{"relevant", "irrelevant", "false_positive", "false_negative"} {
			t.Run(feedbackType, func(t *testing.T) {
				response := performRelevanceRequest(router, stdhttp.MethodPut, "/api/v1/monitors/1/contents/3/feedback", `{"feedback_type":"`+feedbackType+`","expected_feedback_version":null}`, "editor")
				if response.Code != stdhttp.StatusBadRequest {
					t.Fatalf("content feedback type %q status = %d, want 400: %s", feedbackType, response.Code, response.Body.String())
				}
				assertRelevanceError(t, response, sharederrors.CodeInvalidRequest)
			})
		}
	})

	t.Run("invalid list and suggestion cursors are bad requests", func(t *testing.T) {
		for _, test := range []struct {
			path string
			role httptransport.Role
		}{
			{path: "/api/v1/monitors/1/matches?limit=0", role: httptransport.RoleViewer},
			{path: "/api/v1/monitors/1/matches?cursor=not-a-cursor", role: httptransport.RoleViewer},
			{path: "/api/v1/monitors/1/feedback/suggestions?cursor=not-a-cursor", role: httptransport.RoleAdmin},
		} {
			router := newRelevanceRouter(stub, test.role)
			response := performRelevanceRequest(router, stdhttp.MethodGet, test.path, "", "member")
			if response.Code != stdhttp.StatusBadRequest {
				t.Fatalf("%s status = %d, want 400: %s", test.path, response.Code, response.Body.String())
			}
			assertRelevanceError(t, response, sharederrors.CodeInvalidRequest)
		}
	})

	t.Run("foreign match and stale feedback remain safe", func(t *testing.T) {
		stub.getErr = sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "")
		router := newRelevanceRouter(stub, httptransport.RoleEditor)
		response := performRelevanceRequest(router, stdhttp.MethodGet, "/api/v1/monitors/1/matches/999", "", "editor")
		if response.Code != stdhttp.StatusNotFound {
			t.Fatalf("foreign match status = %d, want 404: %s", response.Code, response.Body.String())
		}
		assertRelevanceError(t, response, sharederrors.CodeNotFound)
		stub.getErr = nil
		stub.feedbackErr = sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
		response = performRelevanceRequest(router, stdhttp.MethodPut, "/api/v1/monitors/1/matches/7/feedback", `{"feedback_type":"irrelevant","expected_feedback_version":1}`, "editor")
		if response.Code != stdhttp.StatusConflict {
			t.Fatalf("stale feedback status = %d, want 409: %s", response.Code, response.Body.String())
		}
		assertRelevanceError(t, response, sharederrors.CodeConflict)
		stub.feedbackErr = nil
	})

	router := newRelevanceRouter(stub, httptransport.RoleViewer)
	response := performRelevanceRequest(router, stdhttp.MethodGet, "/api/v1/monitors/1/matches/7", "", "viewer")
	if strings.Contains(response.Body.String(), "private-body") || strings.Contains(response.Body.String(), "credential_ref") || strings.Contains(response.Body.String(), "object_key") || strings.Contains(response.Body.String(), strings.Repeat("a", 64)) {
		t.Fatalf("relevance DTO leaked sensitive fact: %s", response.Body.String())
	}
	router = newRelevanceRouter(stub, httptransport.RoleEditor)
	response = performRelevanceRequest(router, stdhttp.MethodPost, "/api/v1/monitors/1/relevance-preview", "", "editor")
	if strings.Contains(response.Body.String(), "preview-input-hash") || strings.Contains(response.Body.String(), "private-model") {
		t.Fatalf("preview DTO leaked private relevance fact: %s", response.Body.String())
	}
}

type relevanceHTTPStub struct {
	snapshot    ingestiondomain.RelevanceSnapshot
	content     ingestiondomain.Content
	getErr      error
	feedbackErr error
	listCalls   int
}

func (stub *relevanceHTTPStub) ListMatches(_ context.Context, _ int64, _ ingestiondomain.RelevanceSnapshotListQuery) (ingestiondomain.RelevanceSnapshotPage, error) {
	stub.listCalls++
	return ingestiondomain.RelevanceSnapshotPage{Items: []ingestiondomain.RelevanceSnapshot{stub.snapshot}}, nil
}
func (stub *relevanceHTTPStub) GetMatch(_ context.Context, _ int64, _ int64) (ingestionapplication.RelevanceMatchDetail, error) {
	if stub.getErr != nil {
		return ingestionapplication.RelevanceMatchDetail{}, stub.getErr
	}
	return ingestionapplication.RelevanceMatchDetail{Snapshot: stub.snapshot, Content: stub.content}, nil
}
func (stub *relevanceHTTPStub) Preview(_ context.Context, _ int64) ([]ingestionapplication.RelevancePreviewItem, error) {
	modelVersion := "private-model"
	return []ingestionapplication.RelevancePreviewItem{{ContentID: 3, Candidates: []ingestionapplication.ScoredRelevanceCandidate{{
		MonitorID: 1, MonitorConfigVersionID: 2, InputHash: "preview-input-hash", ScoringVersion: "relevance-v1",
		RecallPaths: []string{"lexical"}, ReasonCodes: []string{"lexical_candidate"}, MatchedTerms: []string{"openai"},
		Factors: ingestionapplication.RelevanceFactors{Lexical: 80, Entity: 60, Title: 70, Preference: 50}, RuleScore: 70,
		Decision: ingestiondomain.MatchDecisionReview, EmbeddingProfile: &ingestionapplication.ModelProfileReference{ID: 4, Version: 1, ModelVersion: modelVersion},
	}}}}, nil
}
func (stub *relevanceHTTPStub) UpsertMatchFeedback(_ context.Context, _ int64, _ int64, _ int64, _ ingestiondomain.FeedbackType, _ *int64) (ingestiondomain.RelevanceFeedback, error) {
	if stub.feedbackErr != nil {
		return ingestiondomain.RelevanceFeedback{}, stub.feedbackErr
	}
	return ingestiondomain.RelevanceFeedback{RelevanceFeedbackInput: ingestiondomain.RelevanceFeedbackInput{FeedbackType: ingestiondomain.FeedbackTypeRelevant}, ID: 8, Version: 1}, nil
}
func (stub *relevanceHTTPStub) UpsertFalseNegativeContentFeedback(_ context.Context, _ int64, _ int64, _ int64, _ *int64) (ingestiondomain.RelevanceFeedback, error) {
	return ingestiondomain.RelevanceFeedback{RelevanceFeedbackInput: ingestiondomain.RelevanceFeedbackInput{FeedbackType: ingestiondomain.FeedbackTypeFalseNegative}, ID: 8, Version: 1}, nil
}
func (stub *relevanceHTTPStub) Evaluations(context.Context, int64) ([]ingestiondomain.RelevanceEvaluation, error) {
	return []ingestiondomain.RelevanceEvaluation{{ScoringVersion: "relevance-v1", PrecisionAt20: 80, ExclusionFalsePositiveRate: 1, EvaluatedCount: 20}}, nil
}
func (stub *relevanceHTTPStub) RefreshSuggestions(context.Context, int64) (int, error) { return 1, nil }
func (stub *relevanceHTTPStub) ListSuggestions(context.Context, int64, ingestiondomain.RelevanceSuggestionListQuery) (ingestiondomain.RelevanceSuggestionPage, error) {
	return ingestiondomain.RelevanceSuggestionPage{Items: []ingestiondomain.RelevanceSuggestion{{ID: 9, Version: 1, SuggestionType: ingestiondomain.SuggestionTypeAddTerm, Value: "OpenAI", SupportCount: 2, Status: ingestiondomain.SuggestionStatusPending}}}, nil
}
func (stub *relevanceHTTPStub) ReviewSuggestion(context.Context, int64, int64, int64, int64, ingestiondomain.SuggestionStatus) (ingestiondomain.RelevanceSuggestion, error) {
	return ingestiondomain.RelevanceSuggestion{ID: 9, Version: 2, SuggestionType: ingestiondomain.SuggestionTypeAddTerm, Value: "OpenAI", SupportCount: 2, Status: ingestiondomain.SuggestionStatusApproved}, nil
}

func plan009HTTPSnapshot() ingestiondomain.RelevanceSnapshot {
	return ingestiondomain.RelevanceSnapshot{RelevanceSnapshotInput: ingestiondomain.RelevanceSnapshotInput{
		MonitorID: 1, MonitorConfigVersionID: 2, ContentID: 3, InputHash: strings.Repeat("a", 64), ScoringVersion: "relevance-v1", RecallPaths: []string{"lexical"},
		ReasonCodes: []string{"lexical_match"}, RuleScore: 70, FinalScore: 72, Decision: ingestiondomain.MatchDecisionReview, DecisionOrigin: ingestiondomain.DecisionOriginAI,
		Explanation: json.RawMessage(`{"matched_terms":["openai"],"matched_entities":[],"excluded_terms":[],"recall_paths":["lexical"],"scores":{"semantic":0,"lexical":80,"entity":60,"title":70,"preference":50},"reason_codes":["ambiguous_context"],"provenance":{"review_ai_run_id":4}}`),
	}, ID: 7, Version: 1}
}
func plan009HTTPContent() ingestiondomain.Content {
	return ingestiondomain.Content{ID: 3, Title: "Safe title", Language: "en", CanonicalURL: "https://example.test/item"}
}

func newRelevanceRouter(service relevanceHTTPService, role httptransport.Role) *gin.Engine {
	router := gin.New()
	RegisterRelevanceRoutes(router, service, contentAuthenticator{role: role})
	return router
}
func performRelevanceRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
func assertRelevanceSuccess(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var value struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || value.Code != 0 {
		t.Fatalf("success envelope = %q / %#v / %v", response.Body.String(), value, err)
	}
}
func assertRelevanceError(t *testing.T, response *httptest.ResponseRecorder, code int) {
	t.Helper()
	var value struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || value.Code != code {
		t.Fatalf("error envelope = %q / %#v / %v, want code %d", response.Body.String(), value, err, code)
	}
}
