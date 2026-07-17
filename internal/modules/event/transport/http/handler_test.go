package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
