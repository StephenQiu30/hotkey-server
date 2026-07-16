package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestCollectionAdminRoutesEnforceRolesAndExposeOnlySafeRunFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &collectionControlServiceFake{page: domain.CollectionRunPage{Items: []domain.CollectionRunSummary{{
		ID: 41, Status: domain.CollectionRunFailed, CandidateCount: 7, AcceptedCount: 2, RejectedCount: 5,
		ErrorCode: "temporary", StartedAt: timePtr(time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC)),
		FinishedAt: timePtr(time.Date(2026, 7, 16, 1, 3, 3, 0, time.UTC)),
		Targets:    []domain.CollectionRunTargetSummary{{ID: 73, Status: domain.CollectionRunFailed, CandidateCount: 7, AcceptedCount: 2, RejectedCount: 5, ErrorCode: "temporary"}},
	}}}}

	for _, test := range []struct {
		name       string
		role       httptransport.Role
		path       string
		method     string
		wantStatus int
		wantCode   int
	}{
		{name: "viewer list denied", role: httptransport.RoleViewer, path: "/api/v1/collection-runs", method: http.MethodGet, wantStatus: http.StatusForbidden, wantCode: sharederrors.CodeForbidden},
		{name: "editor retry denied", role: httptransport.RoleEditor, path: "/api/v1/collection-runs/41/retry", method: http.MethodPost, wantStatus: http.StatusForbidden, wantCode: sharederrors.CodeForbidden},
		{name: "admin list", role: httptransport.RoleAdmin, path: "/api/v1/collection-runs", method: http.MethodGet, wantStatus: http.StatusOK, wantCode: 0},
		{name: "admin retry", role: httptransport.RoleAdmin, path: "/api/v1/collection-runs/41/retry", method: http.MethodPost, wantStatus: http.StatusOK, wantCode: 0},
		{name: "admin health", role: httptransport.RoleAdmin, path: "/api/v1/source-connections/12/health", method: http.MethodPost, wantStatus: http.StatusOK, wantCode: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterCollectionRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 2, Role: test.role}})
			request := httptest.NewRequest(test.method, test.path, nil)
			request.Header.Set("Authorization", "Bearer member")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			var result struct {
				Code int             `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if result.Code != test.wantCode {
				t.Fatalf("result code = %d, want %d: %s", result.Code, test.wantCode, response.Body.String())
			}
			if test.wantCode == 0 {
				for _, sensitive := range []string{"source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "endpoint", "credential"} {
					if strings.Contains(response.Body.String(), sensitive) {
						t.Fatalf("safe collection response leaked %q: %s", sensitive, response.Body.String())
					}
				}
			}
		})
	}
	if service.listCalls != 1 || service.retryCalls != 1 || service.healthCalls != 1 {
		t.Fatalf("service calls = list:%d retry:%d health:%d, want 1 each", service.listCalls, service.retryCalls, service.healthCalls)
	}
}

func TestCollectionAdminRoutesReturnStableRunErrorsAndRejectInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		path       string
		service    *collectionControlServiceFake
		wantStatus int
		wantCode   int
	}{
		{name: "missing run", path: "/api/v1/collection-runs/99/retry", service: &collectionControlServiceFake{retryErr: domain.CollectionRunNotFound()}, wantStatus: http.StatusNotFound, wantCode: sharederrors.CodeCollectionRunNotFound},
		{name: "fresh run conflict", path: "/api/v1/collection-runs/99/retry", service: &collectionControlServiceFake{retryErr: domain.CollectionRunConflict()}, wantStatus: http.StatusConflict, wantCode: sharederrors.CodeCollectionRunConflict},
		{name: "invalid list query", path: "/api/v1/collection-runs?limit=zero", service: &collectionControlServiceFake{}, wantStatus: http.StatusBadRequest, wantCode: sharederrors.CodeInvalidCollectionRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterCollectionRoutes(router, test.service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}})
			method := http.MethodPost
			if strings.Contains(test.path, "?") {
				method = http.MethodGet
			}
			request := httptest.NewRequest(method, test.path, nil)
			request.Header.Set("Authorization", "Bearer admin")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			var result struct {
				Code int             `json:"code"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if result.Code != test.wantCode || string(result.Data) != "null" {
				t.Fatalf("result = %#v, want code %d and data null", result, test.wantCode)
			}
		})
	}

	router := gin.New()
	RegisterCollectionRoutes(router, &collectionControlServiceFake{}, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/collection-runs", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d: %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

type collectionControlServiceFake struct {
	page        domain.CollectionRunPage
	retry       domain.CollectionRunSummary
	health      domain.SourceHealth
	retryErr    error
	listCalls   int
	retryCalls  int
	healthCalls int
}

func (service *collectionControlServiceFake) List(_ context.Context, input sourceapplication.CollectionRunListInput) (domain.CollectionRunPage, error) {
	service.listCalls++
	if input.Subject.Role != identitydomain.RoleAdmin {
		return domain.CollectionRunPage{}, sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	return service.page, nil
}

func (service *collectionControlServiceFake) Retry(_ context.Context, input sourceapplication.CollectionRunRetryInput) (domain.CollectionRunSummary, error) {
	service.retryCalls++
	if input.Subject.Role != identitydomain.RoleAdmin {
		return domain.CollectionRunSummary{}, sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	if service.retryErr != nil {
		return domain.CollectionRunSummary{}, service.retryErr
	}
	return service.retry, nil
}

func (service *collectionControlServiceFake) Health(_ context.Context, input sourceapplication.SourceHealthInput) (domain.SourceHealth, error) {
	service.healthCalls++
	if input.Subject.Role != identitydomain.RoleAdmin {
		return domain.SourceHealth{}, sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	return service.health, nil
}

func timePtr(value time.Time) *time.Time { return &value }
