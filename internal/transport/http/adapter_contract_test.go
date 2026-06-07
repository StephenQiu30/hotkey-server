package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
	"golang.org/x/crypto/bcrypt"
)

func setupAdapterTest(t *testing.T, reg *adapter.Registry) (http.Handler, *serviceauth.Service) {
	t.Helper()
	authRepo := serviceauth.NewMemoryRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = authRepo.CreateUser(context.Background(), user.User{
		ID:           "usr_admin",
		Email:        "adapter-admin@example.com",
		PasswordHash: string(hash),
		Role:         user.RoleAdmin,
		Status:       user.StatusActive,
		Timezone:     "Asia/Shanghai",
		DailySendAt:  "08:30",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := serviceauth.NewService(authRepo, serviceauth.Config{AccessTokenSecret: "test-adapter-secret"})
	if err != nil {
		t.Fatal(err)
	}

	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	azService := serviceauth.NewAuthorizationService(authRepo, serviceauth.NewMemoryAuthorizationRepository(), enc, nil)

	router := transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
		AuthService:          authService,
		AuthorizationService: azService,
		AdapterRegistry:      reg,
	})
	return router, authService
}

func loginAsAdmin(t *testing.T, handler http.Handler, authService *serviceauth.Service) string {
	t.Helper()
	login := postJSON(t, handler, "/api/v1/auth/login", map[string]string{
		"email":    "adapter-admin@example.com",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d with body %s", login.Code, login.Body.String())
	}
	return jsonStringAt(t, login.Body.Bytes(), "accessToken")
}

func TestListAdaptersRequiresAdminAuth(t *testing.T) {
	reg := adapter.NewRegistry()
	router, _ := setupAdapterTest(t, reg)
	userToken := registerAndLogin(t, router, "adapter-user@example.com")

	denied := getWithBearer(router, "/api/v1/admin/adapters", userToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d with body %s", denied.Code, denied.Body.String())
	}
}

func TestListAdaptersReturnsAllRegisteredAdapters(t *testing.T) {
	reg := adapter.NewRegistry()
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
		Items: []adapter.NormalizedItem{
			{Title: "Test", URL: "https://example.com/1", PublishedAt: &now},
		},
	}))
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderPublicPage,
		Name:     "page-adapter",
	}))

	router, authService := setupAdapterTest(t, reg)
	adminToken := loginAsAdmin(t, router, authService)

	adapters := getWithBearer(router, "/api/v1/admin/adapters", adminToken)
	if adapters.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", adapters.Code, adapters.Body.String())
	}
	if adapters.Body.Len() == 0 {
		t.Fatal("expected non-empty adapters list")
	}
}

func TestGetAdapterHealthReturnsStatus(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
	}))

	router, authService := setupAdapterTest(t, reg)
	adminToken := loginAsAdmin(t, router, authService)

	health := getWithBearer(router, "/api/v1/admin/adapters/rss/health", adminToken)
	if health.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", health.Code, health.Body.String())
	}
	assertJSONField(t, health.Body.Bytes(), "status", "healthy")
}

func TestGetAdapterHealthReturns404ForMissing(t *testing.T) {
	reg := adapter.NewRegistry()
	router, authService := setupAdapterTest(t, reg)
	adminToken := loginAsAdmin(t, router, authService)

	health := getWithBearer(router, "/api/v1/admin/adapters/official_api/health", adminToken)
	if health.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d with body %s", health.Code, health.Body.String())
	}
}

func TestGetAdapterCapabilitiesReturnsConfig(t *testing.T) {
	reg := adapter.NewRegistry()
	reg.Register(adapter.NewSimulator(adapter.SimulatorConfig{
		Provider: adapter.ProviderRSS,
		Name:     "rss-adapter",
		Capabilities: adapter.Capabilities{
			SupportsIncremental: true,
			MaxItemsPerFetch:    50,
			RateLimitPerHour:    100,
		},
	}))

	router, authService := setupAdapterTest(t, reg)
	adminToken := loginAsAdmin(t, router, authService)

	caps := getWithBearer(router, "/api/v1/admin/adapters/rss/capabilities", adminToken)
	if caps.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d with body %s", caps.Code, caps.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(caps.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["supports_incremental"] != true {
		t.Fatalf("expected supports_incremental=true, got %v", body["supports_incremental"])
	}
	if body["max_items_per_fetch"] != float64(50) {
		t.Fatalf("expected max_items_per_fetch=50, got %v", body["max_items_per_fetch"])
	}
	if body["rate_limit_per_hour"] != float64(100) {
		t.Fatalf("expected rate_limit_per_hour=100, got %v", body["rate_limit_per_hour"])
	}
}

func TestGetAdapterCapabilitiesReturns404ForMissing(t *testing.T) {
	reg := adapter.NewRegistry()
	router, authService := setupAdapterTest(t, reg)
	adminToken := loginAsAdmin(t, router, authService)

	caps := getWithBearer(router, "/api/v1/admin/adapters/official_api/capabilities", adminToken)
	if caps.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d with body %s", caps.Code, caps.Body.String())
	}
}
