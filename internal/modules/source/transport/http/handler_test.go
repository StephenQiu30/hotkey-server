package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// TestSourceRoutesRequireAdminForManagement keeps the shared Source list
// reader-safe while guarding all mutating routes at the HTTP boundary.
func TestSourceRoutesRequireAdminForManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, (*sourceapplication.Service)(nil), testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleViewer}})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/source-connections", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestSourceResponsesNeverEchoCredentialReference(t *testing.T) {
	t.Parallel()
	public := sourceResponse(domain.PublicSourceConnection{ID: 1, Version: 2, Name: "RSS", SourceType: domain.SourceTypeRSS, CredentialConfigured: true})
	management := managementResponse(domain.ManagementSourceConnection{PublicSourceConnection: domain.PublicSourceConnection{ID: 1, Version: 2, Name: "RSS", SourceType: domain.SourceTypeRSS, CredentialConfigured: true}, Endpoint: "https://feeds.example.test/rss", Config: domain.DefaultSourceConfig()})
	for _, response := range []any{public, management} {
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		if strings.Contains(string(encoded), "credential_ref") || strings.Contains(string(encoded), "env:") {
			t.Fatalf("response leaked credential reference: %s", encoded)
		}
	}
}

func TestSourceTransportRejectsUnknownConfigKeysBeforeApplication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, (*sourceapplication.Service)(nil), testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleAdmin}})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/source-connections", strings.NewReader(`{"source_type":"rss","name":"RSS","endpoint":"https://feeds.example.test/rss","auth_type":"none","config":{"raw_secret":"must-be-rejected"}}`))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "must-be-rejected") {
		t.Fatalf("error echoed rejected secret-shaped input: %s", response.Body.String())
	}
}

type testAuthenticator struct{ subject httptransport.Subject }

func (auth testAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return auth.subject, nil
}
