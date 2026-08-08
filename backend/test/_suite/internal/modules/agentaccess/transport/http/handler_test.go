package http

import (
	"bytes"
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/application"
	agentdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type lifecycleServiceFake struct{ token agentdomain.Token }

func (service lifecycleServiceFake) Create(context.Context, agentapplication.CreateInput) (agentapplication.CreatedToken, error) {
	return agentapplication.CreatedToken{Token: service.token, Raw: "hk_agent_one_time_secret"}, nil
}
func (service lifecycleServiceFake) List(context.Context, identitydomain.Subject) ([]agentdomain.Token, error) {
	return []agentdomain.Token{service.token}, nil
}
func (service lifecycleServiceFake) Revoke(context.Context, agentapplication.RevokeInput) (*agentdomain.Token, error) {
	token := service.token
	now := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	token.RevokedAt, token.Version = &now, token.Version+1
	return &token, nil
}

type lifecycleAuthenticator struct{ subject httptransport.Subject }

func (auth lifecycleAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return auth.subject, nil
}

func TestAgentTokenLifecycleReturnsRawOnlyOnCreate(t *testing.T) {
	token := agentdomain.Token{ID: 4, Version: 1, UserID: 2, Name: "Research", TokenPrefix: "hk_agent_abcdefghij", TokenHash: strings.Repeat("a", 64), Scopes: []agentdomain.Scope{agentdomain.ScopeEventsRead}, ExpiresAt: time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)}
	router := lifecycleRouter(lifecycleServiceFake{token: token}, httptransport.Subject{UserID: 2, SessionID: 3, Role: httptransport.RoleViewer})

	created := serveLifecycle(t, router, stdhttp.MethodPost, "/api/v1/agent-tokens", `{"name":"Research","scopes":["events.read"],"lifetime_days":30}`)
	if created.Code != stdhttp.StatusCreated || !strings.Contains(created.Body.String(), "hk_agent_one_time_secret") || strings.Contains(created.Body.String(), token.TokenHash) {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}
	listed := serveLifecycle(t, router, stdhttp.MethodGet, "/api/v1/agent-tokens", "")
	if listed.Code != stdhttp.StatusOK || strings.Contains(listed.Body.String(), "hk_agent_one_time_secret") || strings.Contains(listed.Body.String(), token.TokenHash) {
		t.Fatalf("list response = %d %s", listed.Code, listed.Body.String())
	}
	revoked := serveLifecycle(t, router, stdhttp.MethodPost, "/api/v1/agent-tokens/4/revoke", `{"expected_version":1}`)
	if revoked.Code != stdhttp.StatusOK || strings.Contains(revoked.Body.String(), "hk_agent_one_time_secret") || strings.Contains(revoked.Body.String(), token.TokenHash) {
		t.Fatalf("revoke response = %d %s", revoked.Code, revoked.Body.String())
	}
}

func TestAgentTokenCannotManageItsOwnLifecycle(t *testing.T) {
	router := lifecycleRouter(lifecycleServiceFake{}, httptransport.Subject{UserID: 2, AgentTokenID: 9, Role: httptransport.RoleViewer, AgentScopes: "events.read"})
	response := serveLifecycle(t, router, stdhttp.MethodGet, "/api/v1/agent-tokens", "")
	if response.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["code"] != float64(20000) {
		t.Fatalf("response = %#v, %v", result, err)
	}
}

func lifecycleRouter(service tokenService, subject httptransport.Subject) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, service, lifecycleAuthenticator{subject: subject})
	return router
}

func serveLifecycle(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer credential")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
