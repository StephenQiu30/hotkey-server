package bootstrap

import (
	"context"
	"errors"
	"net"
	stdhttp "net/http"
	"os"
	"testing"
	"time"

	ingestionjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/jobs"
	intelligencedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/domain"
	intelligencejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/intelligence/infrastructure/jobs"
	knowledgejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/jobs"
	notificationjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/infrastructure/jobs"
	sourcejobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/jobs"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDefaultP0HandlersExposeTheMultiSourceHotspotCoreChain(t *testing.T) {
	handlers := newP0Handlers(p0HandlerParams{
		Collect:                 &sourcejobs.CollectHandler{},
		Normalize:               &ingestionjobs.NormalizeHandler{},
		AnalyzeMonitorIntent:    &monitorIntentAnalysisHandler{},
		ProjectUserNotification: &notificationjobs.OutboxProjectionHandler{},
		RecomputeAIRun:          &intelligencejobs.AIRunRecomputeHandler{},
	})
	want := map[string]bool{
		"collect_source":            true,
		"normalize_content":         true,
		"analyze_monitor_intent":    true,
		"project_user_notification": true,
		"recompute_ai_run":          true,
	}
	if len(handlers) != len(want) {
		t.Fatalf("default P0 handler count = %d, want %d", len(handlers), len(want))
	}
	for kind := range want {
		if handlers[kind] == nil {
			t.Errorf("default P0 handler %q is not registered", kind)
		}
	}
}

func TestP0HandlersRegisterVaultRecoveryWhenConfigured(t *testing.T) {
	handlers := newP0Handlers(p0HandlerParams{
		Collect:                 &sourcejobs.CollectHandler{},
		Normalize:               &ingestionjobs.NormalizeHandler{},
		AnalyzeMonitorIntent:    &monitorIntentAnalysisHandler{},
		ProjectUserNotification: &notificationjobs.OutboxProjectionHandler{},
		RecomputeAIRun:          &intelligencejobs.AIRunRecomputeHandler{},
		ProjectKnowledge:        &knowledgejobs.Handler{},
	})
	if handlers["project_knowledge"] == nil {
		t.Fatal("Vault recovery handler is not registered")
	}
}

func TestPlan018AIProviderRegistryUsesExplicitConfiguration(t *testing.T) {
	cfg := config.Default()
	cfg.AI.DeepSeekAPIKey = "test-only-deepseek-key"
	cfg.AI.OllamaEnabled = true
	cfg.AI.OllamaBaseURL = "http://127.0.0.1:11434"
	registry := newAIProviderRegistry(cfg, zap.NewNop())
	if _, ok := registry.Resolve(intelligencedomain.ProviderDeepSeek); !ok {
		t.Fatal("DeepSeek provider is not registered")
	}
	if _, ok := registry.Resolve(intelligencedomain.ProviderOllama); !ok {
		t.Fatal("Ollama provider is not registered")
	}

	cfg.AI.DeepSeekAPIKey = ""
	cfg.AI.OllamaBaseURL = "file:///tmp/ollama.sock"
	core, logs := observer.New(zap.WarnLevel)
	registry = newAIProviderRegistry(cfg, zap.New(core))
	if _, ok := registry.Resolve(intelligencedomain.ProviderDeepSeek); ok {
		t.Fatal("DeepSeek provider registered without a key")
	}
	if _, ok := registry.Resolve(intelligencedomain.ProviderOllama); ok {
		t.Fatal("Ollama provider registered with an unsafe URL")
	}
	if logs.Len() != 1 || logs.All()[0].ContextMap()["provider"] != string(intelligencedomain.ProviderOllama) {
		t.Fatalf("safe configuration diagnostics = %#v", logs.All())
	}
}

func TestAgentShadowRunnerIsOptionalAndConstructsOnlyFromValidatedInternalConfig(t *testing.T) {
	cfg := config.Default()
	disabled, err := newAgentShadowRunner(cfg, zap.NewNop())
	if err != nil || disabled == nil {
		t.Fatalf("newAgentShadowRunner(disabled) = %#v / %v", disabled, err)
	}
	cfg.Agent = config.AgentConfig{
		URL: "http://hotkey-agent:8090", AuthToken: "test-agent-secret-0123456789abcdef0123456789abcdef", MaxResponseBytes: 1 << 20, ShadowEnabled: true,
	}
	enabled, err := newAgentShadowRunner(cfg, zap.NewNop())
	if err != nil || enabled == nil {
		t.Fatalf("newAgentShadowRunner(enabled) = %#v / %v", enabled, err)
	}
}

func TestApplicationRolesStartAndStopIndependently(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleAll, RoleAPI, RoleWorker} {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			cfg := apiTestConfig()
			cfg.Role = string(role)
			if role.StartsAPI() {
				cfg.HTTPAddr = "127.0.0.1:0"
			} else {
				cfg.HTTPAddr = ""
			}

			app, err := NewApp(cfg, zap.NewNop())
			if err != nil {
				t.Fatalf("NewApp() error = %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.Start(ctx); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if err := app.Stop(ctx); err != nil {
				t.Fatalf("Stop() error = %v", err)
			}
		})
	}
}

func TestNewAppRejectsInvalidRole(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Role = "scheduler"
	if _, err := NewApp(cfg, zap.NewNop()); err == nil {
		t.Fatal("NewApp() error = nil, want an error")
	}
}

func TestAPIFxGraphRegistersExactDocumentMatchRoutes(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	cfg := apiTestConfig()
	cfg.Role, cfg.HTTPAddr, cfg.DatabaseURL = string(RoleAPI), "127.0.0.1:0", dsn
	var router *gin.Engine
	app, err := NewAppWithReadiness(
		cfg, zap.NewNop(), httptransport.ReadinessFunc(func(context.Context) error { return nil }), fx.Populate(&router),
	)
	if err != nil {
		t.Fatalf("NewAppWithReadiness(): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start(): %v", err)
	}
	defer func() { _ = app.Stop(ctx) }()
	wanted := map[string]bool{
		"GET /api/v1/notifications":                                               false,
		"GET /api/v1/notifications/ws":                                            false,
		"GET /api/v1/monitors/:id/document-matches":                               false,
		"POST /api/v1/monitors/:id/document-matches/:match_decision_id/overrides": false,
		"GET /api/v1/micro-events":                                                false,
		"GET /api/v1/micro-events/:id":                                            false,
		"GET /api/v1/micro-events/:id/evidence":                                   false,
		"POST /api/v1/micro-events/:id/evidence":                                  false,
		"POST /api/v1/micro-events/:id/evidence/:evidence_id/feedback":            false,
		"POST /api/v1/micro-events/:id/feedback":                                  false,
		"POST /api/v1/document-versions/:id/text-quote-selectors":                 false,
		"POST /api/v1/content-lineage-decisions/:id/feedback":                     false,
		"POST /api/v1/source-webhooks/bilibili":                                   false,
	}
	forbidden := map[string]bool{
		"GET /api/v1/notifications/stream":                    false,
		"GET /api/v1/notifications/push-capability":           false,
		"GET /api/v1/notifications/push-subscriptions":        false,
		"POST /api/v1/notifications/push-subscriptions":       false,
		"PUT /api/v1/notifications/push-subscriptions/:id":    false,
		"DELETE /api/v1/notifications/push-subscriptions/:id": false,
		"GET /api/v1/events":                                  false,
		"GET /api/v1/radar/events":                            false,
		"GET /api/v1/alerts":                                  false,
		"GET /api/v1/reports":                                 false,
		"GET /api/v1/report-subscriptions":                    false,
		"GET /api/v1/knowledge/documents":                     false,
		"GET /api/v1/agent-tokens":                            false,
		"GET /api/v1/agent/events":                            false,
		"GET /feeds/:token":                                   false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, found := wanted[key]; found {
			wanted[key] = true
		}
		if _, found := forbidden[key]; found {
			forbidden[key] = true
		}
	}
	for route, found := range wanted {
		if !found {
			t.Errorf("route %s is not registered", route)
		}
	}
	for route, found := range forbidden {
		if found {
			t.Errorf("legacy route %s remains registered", route)
		}
	}
}

func TestNewAppRejectsInvalidEnabledSMTPForWorker(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Role = string(RoleWorker)
	cfg.HTTPAddr = ""
	cfg.Authentication.SMTP.Enabled = true
	cfg.Authentication.SMTP.Host = ""
	if _, err := NewApp(cfg, zap.NewNop()); err == nil {
		t.Fatal("NewApp() error = nil, want invalid enabled SMTP rejection before worker assembly")
	}
}

func TestNewAppWithReadinessRejectsMissingAPICheck(t *testing.T) {
	cfg := apiTestConfig()
	cfg.HTTPAddr = "127.0.0.1:0"
	if _, err := NewAppWithReadiness(cfg, zap.NewNop(), nil); err == nil {
		t.Fatal("NewAppWithReadiness() error = nil, want missing readiness error")
	}
}

func TestIdentityAPIRoleRejectsMissingVerificationHMACSecretBeforeServing(t *testing.T) {
	cfg := apiTestConfig()
	cfg.Role = string(RoleAPI)
	cfg.HTTPAddr = "127.0.0.1:0"
	cfg.Authentication.VerificationHMACSecret = ""

	if _, err := NewApp(cfg, zap.NewNop()); err == nil {
		t.Fatal("NewApp() error = nil, want missing verification HMAC rejection")
	}
}

func TestIdentityWorkerDoesNotConstructAuthenticationDependencies(t *testing.T) {
	cfg := config.Default()
	cfg.Role = string(RoleWorker)
	cfg.HTTPAddr = ""
	cfg.Authentication.RedisURL = "://not-a-redis-url"

	if _, err := NewApp(cfg, zap.NewNop()); err != nil {
		t.Fatalf("NewApp(worker) error = %v, want worker independent of API authentication dependencies", err)
	}
}

func TestRunningAppUsesInjectedFailingReadiness(t *testing.T) {
	cfg := apiTestConfig()
	cfg.Role = string(RoleAPI)
	cfg.HTTPAddr = "127.0.0.1:0"
	var server *httptransport.Server
	app, err := NewAppWithReadiness(cfg, zap.NewNop(), httptransport.ReadinessFunc(func(context.Context) error {
		return errors.New("required dependency unavailable")
	}), fx.Populate(&server))
	if err != nil {
		t.Fatalf("NewAppWithReadiness() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = app.Stop(ctx) }()

	response, err := stdhttp.Get("http://" + server.Address() + "/readyz")
	if err != nil {
		t.Fatalf("GET readyz: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want %d", response.StatusCode, stdhttp.StatusServiceUnavailable)
	}
}

func TestAPIPortConflictRollsBackAndCanRestart(t *testing.T) {
	address := availableAddress(t)
	cfg := apiTestConfig()
	cfg.Role = string(RoleAPI)
	cfg.HTTPAddr = address
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first, err := NewApp(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("first NewApp() error = %v", err)
	}
	if err := first.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	second, err := NewApp(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("second NewApp() error = %v", err)
	}
	if err := second.Start(ctx); err == nil {
		t.Fatal("second Start() error = nil, want port conflict")
	}
	if err := first.Stop(ctx); err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}

	restarted, err := NewApp(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("restart NewApp() error = %v", err)
	}
	if err := restarted.Start(ctx); err != nil {
		t.Fatalf("restart Start() error = %v", err)
	}
	if err := restarted.Stop(ctx); err != nil {
		t.Fatalf("restart Stop() error = %v", err)
	}
}

func TestLifecycleStartFailureRollsBackStartedServer(t *testing.T) {
	address := availableAddress(t)
	cfg := apiTestConfig()
	cfg.Role = string(RoleAPI)
	cfg.HTTPAddr = address
	app, err := NewAppWithReadiness(cfg, zap.NewNop(), httptransport.ReadinessFunc(func(context.Context) error { return nil }), fx.Invoke(func(lifecycle fx.Lifecycle) {
		lifecycle.Append(fx.Hook{OnStart: func(context.Context) error { return errors.New("intentional lifecycle failure") }})
	}))
	if err != nil {
		t.Fatalf("NewAppWithReadiness() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err == nil {
		t.Fatal("Start() error = nil, want lifecycle failure")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("listener remained after failed start: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close replacement listener: %v", err)
	}
}

func apiTestConfig() config.Config {
	cfg := config.Default()
	cfg.Authentication.JWTSecret = "0123456789abcdef0123456789abcdef"
	cfg.Authentication.VerificationHMACSecret = "verification-hmac-secret-for-tests-32-bytes"
	cfg.Authentication.AllowedOrigins = []string{"http://localhost:3000"}
	return cfg
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve test address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release test address: %v", err)
	}
	return address
}

func TestRunRejectsMissingDatabaseURL(t *testing.T) {
	t.Setenv("HOTKEY_ROLE", "worker")
	t.Setenv("HOTKEY_HTTP_ADDR", "")
	t.Setenv("HOTKEY_SHUTDOWN_TIMEOUT", "1s")
	t.Setenv("HOTKEY_DATABASE_URL", "")

	if err := Run(context.Background(), []string{"serve"}); err == nil {
		t.Fatal("Run() error = nil, want missing database URL")
	}
}

func TestConfiguredWorkerVerifiesDatabaseOnStart(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	cfg := config.Default()
	cfg.Role = string(RoleWorker)
	cfg.HTTPAddr = ""
	cfg.DatabaseURL = dsn
	cfg.Authentication.JWTSecret = "unused-short-worker-secret"
	app, err := NewApp(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("configured app Start() error = %v", err)
	}
	if err := app.Stop(ctx); err != nil {
		t.Fatalf("configured app Stop() error = %v", err)
	}
}

func TestConfiguredAllRoleBuildsOneSharedDependencyGraph(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	cfg := apiTestConfig()
	cfg.Role, cfg.HTTPAddr, cfg.DatabaseURL = string(RoleAll), "127.0.0.1:0", dsn
	app, err := NewApp(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewApp() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("configured all-role app Start() error = %v", err)
	}
	if err := app.Stop(ctx); err != nil {
		t.Fatalf("configured all-role app Stop() error = %v", err)
	}
}

// TestConfiguredAPIWiresControlPlanes verifies the exact Fx
// graph used by the real API role. A 401 from each route proves the routers
// are mounted while avoiding any mutation or identity fixture setup.
func TestConfiguredAPIWiresControlPlanes(t *testing.T) {
	dsn := initializedBootstrapDatabase(t)
	cfg := apiTestConfig()
	cfg.Role, cfg.HTTPAddr, cfg.DatabaseURL = string(RoleAPI), "127.0.0.1:0", dsn
	var server *httptransport.Server
	app, err := NewAppWithReadiness(cfg, zap.NewNop(), httptransport.ReadinessFunc(func(context.Context) error { return nil }), fx.Populate(&server))
	if err != nil {
		t.Fatalf("NewAppWithReadiness() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = app.Stop(ctx) }()
	for _, path := range []string{"/api/v1/monitors", "/api/v1/monitors/1/draft", "/api/v1/source-connections", "/api/v1/contents", "/api/v1/micro-events", "/api/v1/ai/model-profiles", "/api/v1/operations/jobs", "/api/v1/notifications"} {
		response, err := stdhttp.Get("http://" + server.Address() + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if response.StatusCode != stdhttp.StatusUnauthorized {
			response.Body.Close()
			t.Fatalf("%s status = %d, want %d", path, response.StatusCode, stdhttp.StatusUnauthorized)
		}
		response.Body.Close()
	}
	for _, path := range []string{"/api/v1/notifications/stream", "/api/v1/notifications/push-capability", "/api/v1/notifications/push-subscriptions", "/api/v1/events", "/api/v1/radar/events", "/api/v1/alerts", "/api/v1/reports", "/api/v1/report-subscriptions", "/api/v1/knowledge/documents", "/api/v1/agent-tokens", "/api/v1/agent/events", "/feeds/legacy-token"} {
		response, err := stdhttp.Get("http://" + server.Address() + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if response.StatusCode != stdhttp.StatusNotFound {
			response.Body.Close()
			t.Fatalf("legacy route %s status = %d, want %d", path, response.StatusCode, stdhttp.StatusNotFound)
		}
		response.Body.Close()
	}
}

// initializedBootstrapDatabase keeps lifecycle tests independent from the
// disposable administrator database. Other packages intentionally rebuild
// that base fixture in parallel, so sharing it makes the catalog verifier see
// a partially recreated schema instead of the application under test.
func initializedBootstrapDatabase(t *testing.T) string {
	t.Helper()
	if os.Getenv("HOTKEY_TEST_DSN") == "" {
		t.Fatal("HOTKEY_TEST_DSN is required for database lifecycle integration")
	}
	dsn := postgresfixture.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open bootstrap fixture database: %v", err)
	}
	defer func() { _ = runtime.Close() }()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatalf("initialize bootstrap fixture database: %v", err)
	}
	return dsn
}

func TestApplyCommandLineRejectsUnknownCommandAndArguments(t *testing.T) {
	cfg := config.Default()
	if err := applyCommandLine(&cfg, []string{"db", "verify"}); err == nil {
		t.Fatal("applyCommandLine() error = nil, want unknown command error")
	}
	if err := applyCommandLine(&cfg, []string{"serve", "unexpected"}); err == nil {
		t.Fatal("applyCommandLine() error = nil, want unexpected argument error")
	}
}
