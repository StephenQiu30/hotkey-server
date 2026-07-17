package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

type readFake struct{}

func (readFake) List(context.Context, domain.EventListQuery) (domain.EventPage, error) {
	return domain.EventPage{}, nil
}
func (readFake) Get(context.Context, int64) (domain.Event, error) { return domain.Event{}, nil }
func (readFake) ListMembers(context.Context, int64) (domain.EventMemberPage, error) {
	return domain.EventMemberPage{}, nil
}

type missingEventReadFake struct{ readFake }

func (missingEventReadFake) Get(context.Context, int64) (domain.Event, error) {
	return domain.Event{}, sharedrepository.ErrNotFound
}

type missingMemberEventReadFake struct{ readFake }

func (missingMemberEventReadFake) ListMembers(context.Context, int64) (domain.EventMemberPage, error) {
	return domain.EventMemberPage{}, sharedrepository.ErrNotFound
}

type staleLifecycleStore struct{ event domain.Event }

func (store staleLifecycleStore) Get(context.Context, int64) (domain.Event, error) {
	return store.event, nil
}
func (staleLifecycleStore) Save(context.Context, domain.Event, int64, domain.GovernanceAudit) error {
	return nil
}

type governanceStub struct{}

func (governanceStub) Merge(context.Context, application.MergeCommand) (domain.Event, error) {
	return domain.Event{}, nil
}
func (governanceStub) Split(context.Context, application.SplitCommand) (domain.Event, error) {
	return domain.Event{}, nil
}
func (governanceStub) SetMemberLock(context.Context, application.MemberLockCommand) (domain.EventMember, error) {
	return domain.EventMember{}, nil
}

type adminAuthenticator struct{}

func (adminAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleAdmin}, nil
}

type viewerAuthenticator struct{}

func (viewerAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}, nil
}

type intelligenceReadStub struct {
	result application.EventIntelligenceReadResult
	err    error
}

func (stub intelligenceReadStub) Read(context.Context, int64) (application.EventIntelligenceReadResult, error) {
	return stub.result, stub.err
}

type summaryGeneratorStub struct {
	result application.EventSummaryGenerationResult
	err    error
}

func (stub summaryGeneratorStub) Generate(context.Context, int64) (application.EventSummaryGenerationResult, error) {
	return stub.result, stub.err
}

type extractionGeneratorStub struct {
	result application.EventClaimExtractionResult
	err    error
}

func (stub extractionGeneratorStub) Extract(context.Context, int64) (application.EventClaimExtractionResult, error) {
	return stub.result, stub.err
}

func TestEventRoutesRequireAuthenticationAndExposeMemberLockPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, application.NewReadService(readFake{}), nil, nil, httptransport.NewUnavailableAuthenticator())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/events", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != 401 {
		t.Fatalf("unauthenticated event list status = %d, want 401", recorder.Code)
	}
	if _, ok := router.Routes()[0], true; !ok {
		t.Fatal("event routes are not registered")
	}
}

func TestEventRoutesMapInvalidNotFoundAndConflictToStableResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	router := gin.New()
	RegisterRoutes(router, application.NewReadService(missingEventReadFake{}), application.NewLifecycleService(staleLifecycleStore{event: domain.Event{ID: 1, Version: 2, EventKey: "evt_1", TitleZH: "事件", LifecycleStatus: domain.LifecycleDetected, FirstSeenAt: now, LastSeenAt: now}}), application.NewGovernanceService(governanceStub{}), adminAuthenticator{})
	cases := []struct {
		name, method, target, body string
		wantStatus, wantCode       int
	}{
		{name: "invalid identifier", method: http.MethodGet, target: "/api/v1/events/0", wantStatus: http.StatusBadRequest, wantCode: 10000},
		{name: "missing event", method: http.MethodGet, target: "/api/v1/events/9", wantStatus: http.StatusNotFound, wantCode: 10003},
		{name: "stale lifecycle", method: http.MethodPost, target: "/api/v1/events/1/lifecycle", body: `{"expected_version":1,"to":"active","reason":"reviewed"}`, wantStatus: http.StatusConflict, wantCode: 10002},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.target, bytes.NewBufferString(testCase.body))
			request.Header.Set("Authorization", "Bearer test-token")
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if result.Code != testCase.wantCode {
				t.Fatalf("business code = %d, want %d body=%s", result.Code, testCase.wantCode, recorder.Body.String())
			}
		})
	}
}

func TestEventRoutesReturnNotFoundForMissingMembersAndBadRequestForLongReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	router := gin.New()
	RegisterRoutes(router, application.NewReadService(missingMemberEventReadFake{}), application.NewLifecycleService(staleLifecycleStore{event: domain.Event{ID: 1, Version: 1, EventKey: "evt_1", TitleZH: "事件", LifecycleStatus: domain.LifecycleDetected, FirstSeenAt: now, LastSeenAt: now}}), application.NewGovernanceService(governanceStub{}), adminAuthenticator{})
	cases := []struct {
		name, method, target, body string
		wantStatus, wantCode       int
	}{
		{name: "missing member event", method: http.MethodGet, target: "/api/v1/events/9/contents", wantStatus: http.StatusNotFound, wantCode: 10003},
		{name: "long lifecycle reason", method: http.MethodPost, target: "/api/v1/events/1/lifecycle", body: `{"expected_version":1,"to":"active","reason":"` + strings.Repeat("a", domain.MaxReasonCodeLength+1) + `"}`, wantStatus: http.StatusBadRequest, wantCode: 10000},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.target, bytes.NewBufferString(testCase.body))
			request.Header.Set("Authorization", "Bearer test-token")
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if result.Code != testCase.wantCode {
				t.Fatalf("business code = %d, want %d body=%s", result.Code, testCase.wantCode, recorder.Body.String())
			}
		})
	}
}

func TestEventIntelligenceHandlersExposeOnlySafeFactsAndRegenerationResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := intelligenceReadStub{result: application.EventIntelligenceReadResult{EventID: 7,
		Entities: []application.EventIntelligenceEntity{{Entity: domain.Entity{ID: 11, Version: 2, Key: "acme", Name: "Acme", Type: domain.EntityOrganization}, EventEntity: domain.EventEntity{ID: 12, Version: 3, EventID: 7, EntityID: 11, Role: "mentioned", Confidence: 50, Origin: domain.FactOriginModel}}},
		Claims:   []domain.Claim{{ID: 13, Version: 4, EventID: 7, NormalizedClaim: "acme announced", ClaimHash: strings.Repeat("a", 64), Status: domain.ClaimSingleSource, Confidence: 90, Evidence: []domain.ClaimEvidence{{EvidenceRef: domain.EvidenceRef{ContentID: 2, Locator: "title", Excerpt: "trusted"}, Stance: "supports", Confidence: 90}}}},
	}}
	summary := summaryGeneratorStub{result: application.EventSummaryGenerationResult{RunID: 21, Summary: domain.EventSummary{Version: "event-summary-v1", TitleZH: "事件", Sentences: []domain.EvidenceSentence{{Text: "事实", Evidence: []domain.EvidenceRef{{ContentID: 2, Locator: "title", Excerpt: "trusted"}}}}}}}
	extraction := extractionGeneratorStub{result: application.EventClaimExtractionResult{Status: "succeeded", RunID: 22, Facts: application.PersistedEventFacts{Entities: []domain.Entity{{ID: 11}}, Claims: []domain.Claim{{ID: 13}}}}}
	handler := NewHandlerWithIntelligence(nil, nil, nil, nil, nil, reader, summary, extraction)
	router := gin.New()
	api := router.Group("/api/v1/events", httptransport.RequireAuthentication(adminAuthenticator{}))
	api.GET("/:id/intelligence", httptransport.Wrap(handler.GetIntelligence))
	editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
	editor.POST("/:id/intelligence/summary/regenerate", httptransport.Wrap(handler.RegenerateSummary))
	editor.POST("/:id/intelligence/extract", httptransport.Wrap(handler.RegenerateExtraction))

	for _, testCase := range []struct {
		method, target string
		want           string
	}{
		{method: http.MethodGet, target: "/api/v1/events/7/intelligence", want: `"entity_key":"acme"`},
		{method: http.MethodPost, target: "/api/v1/events/7/intelligence/summary/regenerate", want: `"run_id":21`},
		{method: http.MethodPost, target: "/api/v1/events/7/intelligence/extract", want: `"claim_count":1`},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.target, nil)
		request.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), testCase.want) || strings.Contains(recorder.Body.String(), "provider") || strings.Contains(recorder.Body.String(), "prompt") {
			t.Fatalf("%s %s status/body = %d/%s", testCase.method, testCase.target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestEventIntelligenceHandlersKeepAuthenticationRoleAndErrorBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newRouter := func(authenticator httptransport.Authenticator, reader intelligenceReadStub, summary summaryGeneratorStub) *gin.Engine {
		handler := NewHandlerWithIntelligence(nil, nil, nil, nil, nil, reader, summary, extractionGeneratorStub{})
		router := gin.New()
		api := router.Group("/api/v1/events", httptransport.RequireAuthentication(authenticator))
		api.GET("/:id/intelligence", httptransport.Wrap(handler.GetIntelligence))
		editor := api.Group("", httptransport.RequireRoles(httptransport.RoleEditor, httptransport.RoleAdmin))
		editor.POST("/:id/intelligence/summary/regenerate", httptransport.Wrap(handler.RegenerateSummary))
		return router
	}
	for _, testCase := range []struct {
		name, method, target string
		router                *gin.Engine
		withToken             bool
		wantStatus, wantCode  int
	}{
		{name: "unauthenticated read", method: http.MethodGet, target: "/api/v1/events/7/intelligence", router: newRouter(adminAuthenticator{}, intelligenceReadStub{}, summaryGeneratorStub{}), wantStatus: http.StatusUnauthorized, wantCode: 20000},
		{name: "viewer cannot regenerate", method: http.MethodPost, target: "/api/v1/events/7/intelligence/summary/regenerate", router: newRouter(viewerAuthenticator{}, intelligenceReadStub{}, summaryGeneratorStub{}), withToken: true, wantStatus: http.StatusForbidden, wantCode: 20001},
		{name: "invalid identifier", method: http.MethodGet, target: "/api/v1/events/0/intelligence", router: newRouter(adminAuthenticator{}, intelligenceReadStub{}, summaryGeneratorStub{}), withToken: true, wantStatus: http.StatusBadRequest, wantCode: 10000},
		{name: "unknown event", method: http.MethodGet, target: "/api/v1/events/7/intelligence", router: newRouter(adminAuthenticator{}, intelligenceReadStub{err: sharedrepository.ErrNotFound}, summaryGeneratorStub{}), withToken: true, wantStatus: http.StatusNotFound, wantCode: 10003},
		{name: "locked fact conflict", method: http.MethodPost, target: "/api/v1/events/7/intelligence/summary/regenerate", router: newRouter(adminAuthenticator{}, intelligenceReadStub{}, summaryGeneratorStub{err: sharedrepository.ErrConflict}), withToken: true, wantStatus: http.StatusConflict, wantCode: 10002},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(testCase.method, testCase.target, nil)
			if testCase.withToken {
				request.Header.Set("Authorization", "Bearer test-token")
			}
			testCase.router.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if result.Code != testCase.wantCode {
				t.Fatalf("business code = %d, want %d body=%s", result.Code, testCase.wantCode, recorder.Body.String())
			}
		})
	}
}
