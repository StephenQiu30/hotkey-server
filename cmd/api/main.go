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
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
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

	// Wire notification
	notifyRepo := &stubNotifyRepo{}
	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)

	// Auth middleware: validates token and injects user ID into context.
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement real JWT/token validation.
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}

	router := server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		NotificationHandler: notifyHandler,
		AuthMiddleware:      authMiddleware,
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
	_, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("worker", "starting"))

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Print(observability.RenderLog("worker", "shutting down"))
		cancel()
	}()

	// Wire dispatch job
	deliveryRepo := &stubDeliveryRepo{}
	mailer := &stubMailer{}
	emailResolver := &stubUserEmailLookup{}
	dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, emailResolver)

	// Register background jobs
	runner := jobs.NewRunner()
	runner.Register("poll_monitor", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "poll_monitor: running"))
		return nil
	}, 1*time.Minute)
	runner.Register("aggregate_topics", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_topics: running"))
		return nil
	}, 5*time.Minute)
	runner.Register("build_snapshots", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "build_snapshots: running"))
		return nil
	}, 10*time.Minute)
	runner.Register("dispatch_notifications", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
		return dispatchJob.Run(ctx, 0)
	}, 1*time.Minute)

	log.Print(observability.RenderLog("worker", "ready, running jobs"))
	runner.Run(ctx)
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

func (r *stubAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) {
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

// stubNotifyRepo is a placeholder repository that returns empty results.
// Replace with a real database-backed implementation.
type stubNotifyRepo struct{}

func (r *stubNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *stubNotifyRepo) MarkRead(_ context.Context, _, _ int64) error {
	return nil
}
func (r *stubNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

// stubDeliveryRepo is a placeholder for the jobs delivery repository.
type stubDeliveryRepo struct{}

func (r *stubDeliveryRepo) CreateDelivery(_ context.Context, _ jobs.EmailDelivery) error {
	return nil
}

func (r *stubDeliveryRepo) UpdateDeliveryStatus(_ context.Context, _ int64, _ string, _ string, _ string) error {
	return nil
}

func (r *stubDeliveryRepo) GetPendingDeliveries(_ context.Context, _ int) ([]jobs.EmailDelivery, error) {
	return nil, nil
}

// stubUserEmailLookup resolves notification IDs to empty email addresses.
type stubUserEmailLookup struct{}

func (r *stubUserEmailLookup) ResolveEmail(_ context.Context, _ int64) (string, error) {
	return "unresolved@example.com", nil
}

// stubMailer is a placeholder mailer that logs instead of sending.
type stubMailer struct{}

func (m *stubMailer) Send(_ context.Context, _, _, _ string) (string, error) {
	return "stub-msg-id", nil
}
