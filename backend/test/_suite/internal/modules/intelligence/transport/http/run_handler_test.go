package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	intelligenceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/gin-gonic/gin"
)

func TestAIRunRecomputeRouteIsAdminOnlyAndReturnsAcceptedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, test := range []struct {
		name       string
		role       httptransport.Role
		token      string
		wantStatus int
		wantCode   int
	}{
		{name: "unauthenticated", role: httptransport.RoleAdmin, wantStatus: stdhttp.StatusUnauthorized, wantCode: sharederrors.CodeUnauthenticated},
		{name: "viewer", role: httptransport.RoleViewer, token: "member", wantStatus: stdhttp.StatusForbidden, wantCode: sharederrors.CodeForbidden},
		{name: "editor", role: httptransport.RoleEditor, token: "member", wantStatus: stdhttp.StatusForbidden, wantCode: sharederrors.CodeForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &aiRunRecomputeServiceStub{}
			response := aiRunRecomputeRequest(newAIRunRouter(service, test.role), "/api/v1/ai/runs/41/recompute", test.token)
			assertModelProfileError(t, response, test.wantStatus, test.wantCode)
			if service.calls != 0 {
				t.Fatalf("service calls = %d, want 0", service.calls)
			}
		})
	}

	service := &aiRunRecomputeServiceStub{result: intelligenceapplication.AIRunRecomputeResult{RunID: 41, JobID: 92, Created: true}}
	response := aiRunRecomputeRequest(newAIRunRouter(service, httptransport.RoleAdmin), "/api/v1/ai/runs/41/recompute", "admin")
	if response.Code != stdhttp.StatusAccepted || service.calls != 1 || service.runID != 41 {
		t.Fatalf("accepted response = %d calls=%d run=%d: %s", response.Code, service.calls, service.runID, response.Body.String())
	}
	var result struct {
		Code int                    `json:"code"`
		Data AIRunRecomputeResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data != (AIRunRecomputeResponse{RunID: 41, JobID: 92, Created: true}) {
		t.Fatalf("recompute response = %#v / %v", result, err)
	}
}

func TestAIRunRecomputeRouteMapsInvalidMissingAndConflictingRuns(t *testing.T) {
	for _, test := range []struct {
		name       string
		path       string
		err        error
		wantStatus int
		wantCode   int
	}{
		{name: "invalid id", path: "/api/v1/ai/runs/nope/recompute", wantStatus: stdhttp.StatusBadRequest, wantCode: sharederrors.CodeInvalidRequest},
		{name: "missing", path: "/api/v1/ai/runs/41/recompute", err: sharedrepository.ErrNotFound, wantStatus: stdhttp.StatusNotFound, wantCode: sharederrors.CodeNotFound},
		{name: "not terminal", path: "/api/v1/ai/runs/41/recompute", err: sharedrepository.ErrConflict, wantStatus: stdhttp.StatusConflict, wantCode: sharederrors.CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &aiRunRecomputeServiceStub{err: test.err}
			response := aiRunRecomputeRequest(newAIRunRouter(service, httptransport.RoleAdmin), test.path, "admin")
			assertModelProfileError(t, response, test.wantStatus, test.wantCode)
		})
	}
}

type aiRunRecomputeServiceStub struct {
	result intelligenceapplication.AIRunRecomputeResult
	err    error
	calls  int
	runID  int64
}

func (stub *aiRunRecomputeServiceStub) Schedule(_ context.Context, runID int64) (intelligenceapplication.AIRunRecomputeResult, error) {
	stub.calls++
	stub.runID = runID
	return stub.result, stub.err
}

func newAIRunRouter(service aiRunRecomputeService, role httptransport.Role) *gin.Engine {
	router := gin.New()
	RegisterAIRunRoutes(router, service, modelProfileAuthenticator{role: role})
	return router
}

func aiRunRecomputeRequest(router *gin.Engine, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(stdhttp.MethodPost, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
