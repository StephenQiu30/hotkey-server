package bootstrap

import (
	"context"
	"errors"
	"net"
	stdhttp "net/http"
	"os"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

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
	dsn := os.Getenv("HOTKEY_TEST_DSN")
	if dsn == "" {
		t.Fatal("HOTKEY_TEST_DSN is required for database lifecycle integration")
	}
	cfg := config.Default()
	cfg.Role = string(RoleWorker)
	cfg.HTTPAddr = ""
	cfg.DatabaseURL = dsn
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

// TestConfiguredAPIWiresMonitorAndSourceControlPlane verifies the exact Fx
// graph used by the real API role. A 401 from each route proves the routers
// are mounted while avoiding any mutation or identity fixture setup.
func TestConfiguredAPIWiresMonitorAndSourceControlPlane(t *testing.T) {
	dsn := os.Getenv("HOTKEY_TEST_DSN")
	if dsn == "" {
		t.Fatal("HOTKEY_TEST_DSN is required for database lifecycle integration")
	}
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
	for _, path := range []string{"/api/v1/monitors", "/api/v1/source-connections"} {
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
