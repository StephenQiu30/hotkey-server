package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type agentAuthenticator struct{ subject httptransport.Subject }

func (auth agentAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return auth.subject, nil
}

func TestRequireAgentScopeRejectsBrowserMissingScopeAndRoleDowngrade(t *testing.T) {
	tests := []struct {
		name    string
		subject httptransport.Subject
		want    int
	}{
		{name: "allowed", subject: httptransport.Subject{UserID: 1, AgentTokenID: 2, Role: httptransport.RoleEditor, AgentScopes: "search.run"}, want: http.StatusNoContent},
		{name: "browser", subject: httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleEditor, AgentScopes: "search.run"}, want: http.StatusForbidden},
		{name: "missing scope", subject: httptransport.Subject{UserID: 1, AgentTokenID: 2, Role: httptransport.RoleEditor}, want: http.StatusForbidden},
		{name: "viewer", subject: httptransport.Subject{UserID: 1, AgentTokenID: 2, Role: httptransport.RoleViewer, AgentScopes: "search.run"}, want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/agent", httptransport.RequireAuthentication(agentAuthenticator{test.subject}), httptransport.RequireAgentScope("search.run", httptransport.RoleEditor, httptransport.RoleAdmin), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			request := httptest.NewRequest(http.MethodGet, "/agent", nil)
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
