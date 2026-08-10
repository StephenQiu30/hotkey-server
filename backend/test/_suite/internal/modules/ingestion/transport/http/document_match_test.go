package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

func TestDocumentMatchRoutesExposeSafeExactFactsAndVersionedOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	query := &documentMatchQueryHTTPStub{result: ingestionapplication.DocumentMatchPageResult{
		Items: []ingestionapplication.DocumentMatchListItemDTO{{
			Automatic: ingestionapplication.DocumentMatchDecisionDTO{
				ID: 31, MonitorID: 7, MonitorVersionID: 11, CompiledProfileID: 13,
				DocumentVersionID: 17, RelevanceProfileID: 19, MatchingAlgorithmVersion: "rrf-k60-v1",
				RerankerVersion: "cross-encoder-v1", CalibrationVersion: "uncalibrated-v1",
				InputHash: strings.Repeat("a", 64), RRFScore: .02, Decision: "review", Degraded: true,
				ReasonCodes: []string{"semantic_model_unavailable"},
				Signals:     []ingestionapplication.DocumentMatchSignalDTO{{Channel: "lexical", Rank: 1, RawScore: .75, AlgorithmVersion: "fts-trgm-dice-v1"}},
				CreatedAt:   now,
			},
			EffectiveDecision: "review", OverrideSequence: 0,
		}},
	}}
	review := &documentMatchReviewHTTPStub{result: ingestionapplication.OverrideDocumentMatchResult{
		Override: ingestionapplication.DocumentMatchOverrideDTO{
			ID: 41, MatchDecisionID: 31, Sequence: 1, MonitorID: 7, MonitorVersionID: 11,
			DocumentVersionID: 17, PreviousEffectiveDecision: "review", Decision: "accepted",
			ReasonCode: "manual_relevant", Note: "matches the published intent", ActorUserID: 1, CreatedAt: now,
		},
	}}

	viewer := newDocumentMatchRouter(query, review, httptransport.RoleViewer)
	response := performDocumentMatchRequest(viewer, stdhttp.MethodGet, "/api/v1/monitors/7/document-matches?decision=review", "viewer", "", "", "")
	if response.Code != stdhttp.StatusOK || query.query.ActorUserID != 1 || query.query.MonitorID != 7 ||
		response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("list response/query = %d %s %#v", response.Code, response.Body.String(), query.query)
	}
	for _, forbidden := range []string{strings.Repeat("a", 64), "credibility", "truth", "unverified", "corroborated", "object_key"} {
		if strings.Contains(strings.ToLower(response.Body.String()), forbidden) {
			t.Fatalf("document match response leaked %q: %s", forbidden, response.Body.String())
		}
	}

	blocked := performDocumentMatchRequest(viewer, stdhttp.MethodPost, "/api/v1/monitors/7/document-matches/31/overrides", "viewer",
		`{"decision":"accepted","reason_code":"manual_relevant"}`, `"v0"`, "review-31")
	if blocked.Code != stdhttp.StatusForbidden || review.calls != 0 {
		t.Fatalf("viewer override status/calls = %d/%d: %s", blocked.Code, review.calls, blocked.Body.String())
	}

	editor := newDocumentMatchRouter(query, review, httptransport.RoleEditor)
	created := performDocumentMatchRequest(editor, stdhttp.MethodPost, "/api/v1/monitors/7/document-matches/31/overrides", "editor",
		`{"decision":"accepted","reason_code":"manual_relevant","note":"matches the published intent"}`, `"v0"`, "review-31")
	if created.Code != stdhttp.StatusCreated || created.Header().Get("ETag") != `"v1"` ||
		review.command.ActorUserID != 1 || review.command.ExpectedSequence != 0 || review.command.IdempotencyKey != "review-31" {
		t.Fatalf("created override = %d %s %#v", created.Code, created.Body.String(), review.command)
	}
	var envelope struct {
		Code int                              `json:"code"`
		Data OverrideDocumentMatchResponseDTO `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || envelope.Code != 0 || envelope.Data.ResourceVersion != 1 {
		t.Fatalf("created envelope = %#v err=%v", envelope, err)
	}
}

func TestDocumentMatchOverrideRouteRejectsStaleMalformedAndUnknownInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &documentMatchQueryHTTPStub{}
	review := &documentMatchReviewHTTPStub{err: sharedrepository.ErrConflict}
	router := newDocumentMatchRouter(query, review, httptransport.RoleEditor)

	for _, fixture := range []struct {
		name, body, etag, key string
	}{
		{name: "missing etag", body: `{"decision":"accepted","reason_code":"manual_relevant"}`, key: "review-31"},
		{name: "weak etag", body: `{"decision":"accepted","reason_code":"manual_relevant"}`, etag: `W/"v0"`, key: "review-31"},
		{name: "unknown field", body: `{"decision":"accepted","reason_code":"manual_relevant","trusted":true}`, etag: `"v0"`, key: "review-31"},
		{name: "missing idempotency", body: `{"decision":"accepted","reason_code":"manual_relevant"}`, etag: `"v0"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			before := review.calls
			response := performDocumentMatchRequest(router, stdhttp.MethodPost, "/api/v1/monitors/7/document-matches/31/overrides", "editor",
				fixture.body, fixture.etag, fixture.key)
			if response.Code != stdhttp.StatusBadRequest || review.calls != before {
				t.Fatalf("response/calls = %d/%d: %s", response.Code, review.calls-before, response.Body.String())
			}
			assertDocumentMatchError(t, response, sharederrors.CodeInvalidRequest)
		})
	}
	stale := performDocumentMatchRequest(router, stdhttp.MethodPost, "/api/v1/monitors/7/document-matches/31/overrides", "editor",
		`{"decision":"accepted","reason_code":"manual_relevant"}`, `"v0"`, "review-31")
	if stale.Code != stdhttp.StatusConflict {
		t.Fatalf("stale response = %d %s", stale.Code, stale.Body.String())
	}
	assertDocumentMatchError(t, stale, sharederrors.CodeConflict)
}

type documentMatchQueryHTTPStub struct {
	query  ingestionapplication.ListDocumentMatchesQuery
	result ingestionapplication.DocumentMatchPageResult
	err    error
}

func (stub *documentMatchQueryHTTPStub) List(_ context.Context, query ingestionapplication.ListDocumentMatchesQuery) (ingestionapplication.DocumentMatchPageResult, error) {
	stub.query = query
	return stub.result, stub.err
}

type documentMatchReviewHTTPStub struct {
	command ingestionapplication.OverrideDocumentMatchCommand
	result  ingestionapplication.OverrideDocumentMatchResult
	err     error
	calls   int
}

func (stub *documentMatchReviewHTTPStub) Override(_ context.Context, command ingestionapplication.OverrideDocumentMatchCommand) (ingestionapplication.OverrideDocumentMatchResult, error) {
	stub.calls++
	stub.command = command
	return stub.result, stub.err
}

func newDocumentMatchRouter(query documentMatchQueryHTTPService, review documentMatchReviewHTTPService, role httptransport.Role) *gin.Engine {
	router := gin.New()
	RegisterDocumentMatchRoutes(router, query, review, contentAuthenticator{role: role})
	return router
}

func performDocumentMatchRequest(router *gin.Engine, method, path, token, body, etag, idempotencyKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		request.Header.Set("If-Match", etag)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertDocumentMatchError(t *testing.T, response *httptest.ResponseRecorder, code int) {
	t.Helper()
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Code != code {
		t.Fatalf("error envelope = %s / %#v / %v, want %d", response.Body.String(), envelope, err, code)
	}
}
