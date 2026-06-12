package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <api|worker>\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "api":
		runAPI()
	case "worker":
		runWorker()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runAPI() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("api", "starting"))

	// Wire auth
	authRepo := &stubAuthRepo{}
	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc)

	// Wire monitor
	monitorRepo := &stubMonitorRepo{}
	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)

	// Auth middleware: validates token and injects user ID into context.
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement real JWT/token validation.
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}

	router := server.NewRouter(server.Dependencies{
		AuthHandler:    authHandler,
		MonitorHandler: monitorHandler,
		AuthMiddleware: authMiddleware,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Print(observability.RenderLog("api", "shutting down"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Print(observability.RenderLog("api", "listening on "+cfg.HTTPAddr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func runWorker() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("worker", "starting"))

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Print(observability.RenderLog("worker", fmt.Sprintf("ready, DATABASE_URL=%s REDIS_ADDR=%s", cfg.DatabaseURL, cfg.RedisAddr)))

	<-sigCh
	log.Print(observability.RenderLog("worker", "shutting down"))
}

// stubAuthRepo is a placeholder repository that returns errors.
type stubAuthRepo struct{}

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, _ string) bool { return false }
func (r *stubAuthRepo) Create(_ context.Context, _, _, _ string) (auth.User, error) {
	return auth.User{}, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}

// stubMonitorRepo is a placeholder repository that returns errors.
type stubMonitorRepo struct{}

func (r *stubMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *stubMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) {
	return nil, nil
}
func (r *stubMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *stubMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, monitor.ErrNotFound
}
