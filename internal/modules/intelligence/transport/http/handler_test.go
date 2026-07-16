package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestModelProfileRoutesEnforceAdminControlPlaneAndRedactCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	profile := transportModelProfile()

	t.Run("unauthenticated requests are rejected before the service", func(t *testing.T) {
		service := &modelProfileServiceStub{profile: profile}
		response := modelProfileRequest(newModelProfileRouter(service, httptransport.RoleAdmin), stdhttp.MethodGet, "/api/v1/ai/model-profiles", "", "")
		assertModelProfileError(t, response, stdhttp.StatusUnauthorized, sharederrors.CodeUnauthenticated)
		if service.listCalls != 0 {
			t.Fatalf("List calls = %d, want 0", service.listCalls)
		}
	})

	for _, role := range []httptransport.Role{httptransport.RoleViewer, httptransport.RoleEditor} {
		t.Run(string(role)+" cannot access any control-plane route", func(t *testing.T) {
			service := &modelProfileServiceStub{profile: profile}
			router := newModelProfileRouter(service, role)
			for _, route := range []struct{ method, path string }{
				{stdhttp.MethodGet, "/api/v1/ai/model-profiles"},
				{stdhttp.MethodPost, "/api/v1/ai/model-profiles"},
				{stdhttp.MethodGet, "/api/v1/ai/model-profiles/7"},
				{stdhttp.MethodPatch, "/api/v1/ai/model-profiles/7"},
				{stdhttp.MethodDelete, "/api/v1/ai/model-profiles/7"},
				{stdhttp.MethodPost, "/api/v1/ai/model-profiles/7/restore"},
			} {
				response := modelProfileRequest(router, route.method, route.path, `{}`, "member")
				assertModelProfileError(t, response, stdhttp.StatusForbidden, sharederrors.CodeForbidden)
			}
		})
	}

	t.Run("admin CRUD and restore use safe response DTOs", func(t *testing.T) {
		service := &modelProfileServiceStub{profile: profile}
		router := newModelProfileRouter(service, httptransport.RoleAdmin)
		create := `{"name":"embedding-primary","task_type":"embedding","provider":"openai","model_name":"text-embedding-3-large","model_version":"2026-07","credential_ref":"env:OPENAI_API_KEY","embedding_dimensions":1024,"timeout_seconds":30,"max_attempts":2,"max_cost":"0.1000","daily_budget":"10.0000","fallback_priority":100,"enabled":true}`
		response := modelProfileRequest(router, stdhttp.MethodPost, "/api/v1/ai/model-profiles", create, "admin")
		if response.Code != stdhttp.StatusCreated {
			t.Fatalf("create status = %d, want 201: %s", response.Code, response.Body.String())
		}
		if service.createCalls != 1 || service.created.CredentialRef == nil || *service.created.CredentialRef != intelligencedomain.OpenAICredentialReference {
			t.Fatalf("create service input = %#v, want write-only credential reference", service.created)
		}
		assertNoModelProfileSecret(t, response.Body.String())

		response = modelProfileRequest(router, stdhttp.MethodGet, "/api/v1/ai/model-profiles", "", "admin")
		if response.Code != stdhttp.StatusOK || service.listCalls != 1 {
			t.Fatalf("list status/calls = %d/%d: %s", response.Code, service.listCalls, response.Body.String())
		}
		assertNoModelProfileSecret(t, response.Body.String())

		response = modelProfileRequest(router, stdhttp.MethodGet, "/api/v1/ai/model-profiles/7", "", "admin")
		if response.Code != stdhttp.StatusOK || service.getCalls == 0 {
			t.Fatalf("get status/calls = %d/%d: %s", response.Code, service.getCalls, response.Body.String())
		}
		assertNoModelProfileSecret(t, response.Body.String())

		response = modelProfileRequest(router, stdhttp.MethodPatch, "/api/v1/ai/model-profiles/7", `{"version":1,"timeout_seconds":45,"daily_budget":null}`, "admin")
		if response.Code != stdhttp.StatusOK || service.updateCalls != 1 || service.updated.TimeoutSeconds != 45 || service.updated.DailyBudget != nil {
			t.Fatalf("update status/input = %d/%#v: %s", response.Code, service.updated, response.Body.String())
		}
		assertNoModelProfileSecret(t, response.Body.String())

		service.profile.Version = 2
		response = modelProfileRequest(router, stdhttp.MethodDelete, "/api/v1/ai/model-profiles/7", `{"version":2}`, "admin")
		if response.Code != stdhttp.StatusOK || service.deleteCalls != 1 {
			t.Fatalf("delete status/calls = %d/%d: %s", response.Code, service.deleteCalls, response.Body.String())
		}
		response = modelProfileRequest(router, stdhttp.MethodPost, "/api/v1/ai/model-profiles/7/restore", `{"version":3}`, "admin")
		if response.Code != stdhttp.StatusOK || service.restoreCalls != 1 {
			t.Fatalf("restore status/calls = %d/%d: %s", response.Code, service.restoreCalls, response.Body.String())
		}
	})

	t.Run("immutable PATCH fields return 70000 without an update", func(t *testing.T) {
		for _, field := range []string{"task_type", "provider", "model_name", "model_version", "credential_ref", "embedding_dimensions"} {
			t.Run(field, func(t *testing.T) {
				service := &modelProfileServiceStub{profile: profile}
				body := `{"version":1,"` + field + `":null}`
				response := modelProfileRequest(newModelProfileRouter(service, httptransport.RoleAdmin), stdhttp.MethodPatch, "/api/v1/ai/model-profiles/7", body, "admin")
				assertModelProfileError(t, response, stdhttp.StatusBadRequest, intelligencedomain.CodeAIModelProfileInvalid)
				if service.updateCalls != 0 {
					t.Fatalf("Update calls = %d, want 0", service.updateCalls)
				}
			})
		}
	})

	t.Run("stale version is an explicit conflict", func(t *testing.T) {
		service := &modelProfileServiceStub{profile: profile}
		service.profile.Version = 2
		response := modelProfileRequest(newModelProfileRouter(service, httptransport.RoleAdmin), stdhttp.MethodPatch, "/api/v1/ai/model-profiles/7", `{"version":1,"enabled":false}`, "admin")
		assertModelProfileError(t, response, stdhttp.StatusConflict, sharederrors.CodeConflict)
		if service.updateCalls != 0 {
			t.Fatalf("Update calls = %d, want 0", service.updateCalls)
		}
	})
}

func TestModelProfileRoutesRegisterExactlySixControlPlanePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newModelProfileRouter(&modelProfileServiceStub{profile: transportModelProfile()}, httptransport.RoleAdmin)
	want := map[string]struct{}{
		"GET /api/v1/ai/model-profiles":              {},
		"POST /api/v1/ai/model-profiles":             {},
		"GET /api/v1/ai/model-profiles/:id":          {},
		"PATCH /api/v1/ai/model-profiles/:id":        {},
		"DELETE /api/v1/ai/model-profiles/:id":       {},
		"POST /api/v1/ai/model-profiles/:id/restore": {},
	}
	for _, route := range router.Routes() {
		delete(want, route.Method+" "+route.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing model-profile routes: %#v", want)
	}
}

type modelProfileServiceStub struct {
	profile                                       intelligencedomain.ModelProfile
	created, updated                              intelligencedomain.ModelProfile
	listCalls, getCalls, createCalls, updateCalls int
	deleteCalls, restoreCalls                     int
	err                                           error
}

func (service *modelProfileServiceStub) List(context.Context) ([]intelligencedomain.ModelProfile, error) {
	service.listCalls++
	if service.err != nil {
		return nil, service.err
	}
	return []intelligencedomain.ModelProfile{service.profile}, nil
}
func (service *modelProfileServiceStub) Get(context.Context, int64) (intelligencedomain.ModelProfile, error) {
	service.getCalls++
	if service.err != nil {
		return intelligencedomain.ModelProfile{}, service.err
	}
	return service.profile, nil
}
func (service *modelProfileServiceStub) Create(_ context.Context, profile intelligencedomain.ModelProfile) (intelligencedomain.ModelProfile, error) {
	service.createCalls++
	if service.err != nil {
		return intelligencedomain.ModelProfile{}, service.err
	}
	service.created = profile
	profile.ID, profile.Version = 7, 1
	profile.CreatedAt, profile.UpdatedAt = time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC), time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)
	service.profile = profile
	return profile, nil
}
func (service *modelProfileServiceStub) Update(_ context.Context, profile intelligencedomain.ModelProfile, version int64) (intelligencedomain.ModelProfile, error) {
	service.updateCalls++
	if service.err != nil {
		return intelligencedomain.ModelProfile{}, service.err
	}
	if version != service.profile.Version {
		return intelligencedomain.ModelProfile{}, sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
	}
	profile.Version++
	service.updated, service.profile = profile, profile
	return profile, nil
}
func (service *modelProfileServiceStub) SoftDelete(_ context.Context, _ int64, version int64) (intelligencedomain.ModelProfile, error) {
	service.deleteCalls++
	if version != service.profile.Version {
		return intelligencedomain.ModelProfile{}, sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
	}
	service.profile.Version++
	service.profile.Deleted = true
	return service.profile, nil
}
func (service *modelProfileServiceStub) Restore(_ context.Context, _ int64, version int64) (intelligencedomain.ModelProfile, error) {
	service.restoreCalls++
	if version != service.profile.Version {
		return intelligencedomain.ModelProfile{}, sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "")
	}
	service.profile.Version++
	service.profile.Deleted = false
	return service.profile, nil
}

type modelProfileAuthenticator struct{ role httptransport.Role }

func (authenticator modelProfileAuthenticator) Authenticate(_ context.Context, token string) (httptransport.Subject, error) {
	if token != "admin" && token != "member" {
		return httptransport.Subject{}, errors.New("invalid token")
	}
	return httptransport.Subject{UserID: 1, SessionID: 2, Role: authenticator.role}, nil
}

func newModelProfileRouter(service modelProfileService, role httptransport.Role) *gin.Engine {
	router := gin.New()
	RegisterRoutes(router, service, modelProfileAuthenticator{role: role})
	return router
}

func modelProfileRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertModelProfileError(t *testing.T, response *httptest.ResponseRecorder, wantStatus, wantCode int) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", response.Code, wantStatus, response.Body.String())
	}
	var result struct {
		Code int `json:"code"`
		Data any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if result.Code != wantCode || result.Data != nil {
		t.Fatalf("error result = %#v, want code=%d data=nil", result, wantCode)
	}
}

func assertNoModelProfileSecret(t *testing.T, body string) {
	t.Helper()
	for _, value := range []string{"credential_ref", "env:OPENAI_API_KEY", "api_key", "endpoint", "parameters", "prompt", "raw_response"} {
		if strings.Contains(body, value) {
			t.Fatalf("safe response leaked %q: %s", value, body)
		}
	}
}

func transportModelProfile() intelligencedomain.ModelProfile {
	credential := intelligencedomain.OpenAICredentialReference
	dimensions := intelligencedomain.EmbeddingDimensions
	dailyBudget := "10.0000"
	now := time.Date(2026, time.July, 17, 9, 0, 0, 0, time.UTC)
	return intelligencedomain.ModelProfile{ID: 7, Version: 1, Name: "embedding-primary", TaskType: intelligencedomain.TaskTypeEmbedding,
		Provider: intelligencedomain.ProviderOpenAI, ModelName: "text-embedding-3-large", ModelVersion: "2026-07", CredentialRef: &credential,
		EmbeddingDimensions: &dimensions, TimeoutSeconds: 30, MaxAttempts: 2, MaxCost: "0.1000", DailyBudget: &dailyBudget,
		FallbackPriority: 100, Enabled: true, CreatedAt: now, UpdatedAt: now}
}
