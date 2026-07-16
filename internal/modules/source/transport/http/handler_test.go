package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

func TestSourceReadRoutesUseRoleDependentSafeUnion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	public := domain.PublicSourceConnection{ID: 1, Version: 2, Name: "RSS", SourceType: domain.SourceTypeRSS, Enabled: true, HealthStatus: domain.HealthStatusHealthy, CredentialConfigured: true}
	management := domain.ManagementSourceConnection{PublicSourceConnection: public, Endpoint: "https://feeds.example.test/rss", Config: domain.DefaultSourceConfig()}
	service := &readSourceService{public: public, management: management}

	for _, test := range []struct {
		name         string
		role         httptransport.Role
		path         string
		wantEndpoint bool
	}{
		{name: "viewer get", role: httptransport.RoleViewer, path: "/api/v1/source-connections/1", wantEndpoint: false},
		{name: "editor list", role: httptransport.RoleEditor, path: "/api/v1/source-connections", wantEndpoint: false},
		{name: "admin get", role: httptransport.RoleAdmin, path: "/api/v1/source-connections/1", wantEndpoint: true},
		{name: "admin list", role: httptransport.RoleAdmin, path: "/api/v1/source-connections", wantEndpoint: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			RegisterRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: test.role}})
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Authorization", "Bearer member")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
			}
			body := response.Body.String()
			if strings.Contains(body, `"credential_ref"`) || strings.Contains(body, `"env:`) {
				t.Fatalf("source read leaked credential reference: %s", body)
			}
			if strings.Contains(body, `"endpoint"`) != test.wantEndpoint || strings.Contains(body, `"config"`) != test.wantEndpoint {
				t.Fatalf("source read endpoint/config branch = %s, want endpoint=%v", body, test.wantEndpoint)
			}
		})
	}
}

func TestActualSourceHandlerRecoveryDoesNotLogSensitiveRequestFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	metrics, err := observability.NewMetrics()
	if err != nil {
		t.Fatalf("NewMetrics(): %v", err)
	}
	cfg := config.Default()
	telemetry, err := observability.NewTelemetry(cfg)
	if err != nil {
		t.Fatalf("NewTelemetry(): %v", err)
	}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	router := httptransport.NewRouter(httptransport.ReadinessFunc(func(context.Context) error { return nil }), metrics, telemetry, zap.New(core), cfg)
	service := &panicSourceService{}
	RegisterRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleAdmin}})

	const endpointFragment = "endpoint-fragment-7d9"
	const credentialReference = "env:PRIVATE_SOURCE_TOKEN"
	const secretShapedConfig = "secret-shaped-config"
	body := `{"source_type":"rss","name":"RSS","endpoint":"https://feeds.example.test/rss?hint=` + endpointFragment + `","auth_type":"api_key","credential_ref":"` + credentialReference + `","config":{"allowed_languages":["en-x-` + secretShapedConfig + `"]}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/source-connections", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if service.createCalls != 1 {
		t.Fatalf("source handler did not reach service, create calls = %d", service.createCalls)
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	for _, value := range []string{endpointFragment, credentialReference, secretShapedConfig} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("recovery response leaked %q: %s", value, response.Body.String())
		}
	}
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatalf("marshal log fields: %v", err)
		}
		for _, value := range []string{endpointFragment, credentialReference, secretShapedConfig} {
			if strings.Contains(string(encoded), value) || strings.Contains(entry.Message, value) {
				t.Fatalf("actual source route log leaked %q: message=%q fields=%s", value, entry.Message, encoded)
			}
		}
	}
}

type panicSourceService struct{ createCalls int }

func (service *panicSourceService) Create(context.Context, sourceapplication.CreateInput) (*domain.ManagementSourceConnection, error) {
	service.createCalls++
	panic("forced source application recovery")
}
func (*panicSourceService) Update(context.Context, sourceapplication.UpdateInput) (*domain.ManagementSourceConnection, error) {
	return nil, nil
}
func (*panicSourceService) Enable(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return nil, nil
}
func (*panicSourceService) Disable(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return nil, nil
}
func (*panicSourceService) Archive(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return nil, nil
}
func (*panicSourceService) Restore(context.Context, sourceapplication.LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return nil, nil
}
func (*panicSourceService) GetPublic(context.Context, identitydomain.Subject, int64) (domain.PublicSourceConnection, error) {
	return domain.PublicSourceConnection{}, nil
}
func (*panicSourceService) GetManagement(context.Context, identitydomain.Subject, int64) (domain.ManagementSourceConnection, error) {
	return domain.ManagementSourceConnection{}, nil
}
func (*panicSourceService) ListPublic(context.Context, sourceapplication.ListInput) (domain.PublicSourceConnectionPage, error) {
	return domain.PublicSourceConnectionPage{}, nil
}
func (*panicSourceService) ListManagement(context.Context, sourceapplication.ListInput) (domain.ManagementSourceConnectionPage, error) {
	return domain.ManagementSourceConnectionPage{}, nil
}

type readSourceService struct {
	panicSourceService
	public     domain.PublicSourceConnection
	management domain.ManagementSourceConnection
}

func (service *readSourceService) GetPublic(context.Context, identitydomain.Subject, int64) (domain.PublicSourceConnection, error) {
	return service.public, nil
}
func (service *readSourceService) GetManagement(context.Context, identitydomain.Subject, int64) (domain.ManagementSourceConnection, error) {
	return service.management, nil
}
func (service *readSourceService) ListPublic(context.Context, sourceapplication.ListInput) (domain.PublicSourceConnectionPage, error) {
	return domain.PublicSourceConnectionPage{Items: []domain.PublicSourceConnection{service.public}}, nil
}
func (service *readSourceService) ListManagement(context.Context, sourceapplication.ListInput) (domain.ManagementSourceConnectionPage, error) {
	return domain.ManagementSourceConnectionPage{Items: []domain.ManagementSourceConnection{service.management}}, nil
}

type testAuthenticator struct{ subject httptransport.Subject }

func (auth testAuthenticator) Authenticate(_ context.Context, _ string) (httptransport.Subject, error) {
	return auth.subject, nil
}
