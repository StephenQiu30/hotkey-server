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
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/server"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
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

	smokeTest := os.Getenv("SMOKE_TEST") == "1"

	// Wire auth, monitor, notify repositories.
	var authRepo auth.Repository
	var monitorRepo monitor.Repository
	var notifyRepo notify.Repository

	if smokeTest {
		// In smoke test mode, use in-memory stubs (no database required).
		authRepo = &smokeAuthRepo{}
		monitorRepo = &smokeMonitorRepo{}
		notifyRepo = &smokeNotifyRepo{}
	} else {
		// Connect to database and use real Postgres repositories.
		db, err := database.Open(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("failed to connect to database: %v", err)
		}
		defer db.Close()
		authRepo = database.NewAuthRepo(db)
		monitorRepo = database.NewMonitorRepo(db)
		notifyRepo = database.NewNotifyRepo(db)
	}

	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc, cfg.JWTSecret)

	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)

	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)

	// Wire content (post query) — uses stub until content repo is implemented.
	postQuerySvc := &stubPostQueryService{}
	postHandler := content.NewPostHandler(postQuerySvc)

	// Wire topic (query) — uses stub until topic repo is implemented.
	topicQuerySvc := &stubTopicQueryService{}
	topicHandler := topic.NewTopicHandler(topicQuerySvc)

	// Wire trend (query) — uses stub until trend repo is implemented.
	trendQuerySvc := &stubTrendQueryService{}
	trendHandler := trend.NewTrendHandler(trendQuerySvc)

	// Auth middleware: validates JWT token and injects user ID into context.
	// When SMOKE_TEST=1, bypasses auth and injects a default user ID for smoke testing.
	authMiddleware := func(next http.Handler) http.Handler {
		if smokeTest {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := monitor.ContextWithUserID(r.Context(), 1)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
		return server.AuthMiddleware(cfg.JWTSecret)(next)
	}

	router := server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		TopicHandler:        topicHandler,
		TrendHandler:        trendHandler,
		PostHandler:         postHandler,
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

// stubPostQueryService is a placeholder query service for content posts.
type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

// stubTopicQueryService is a placeholder query service for topics.
type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

// stubTrendQueryService is a placeholder query service for trends.
type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}


func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

// --- Smoke test stubs (in-memory, no database required) ---

type smokeAuthRepo struct{ users []auth.User }

func (r *smokeAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}
func (r *smokeAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	u := auth.User{ID: int64(len(r.users) + 1), Email: email, PasswordHash: passwordHash, DisplayName: displayName, Status: "active", PlanType: "free"}
	r.users = append(r.users, u)
	return u, nil
}
func (r *smokeAuthRepo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (r *smokeAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

type smokeMonitorRepo struct{}

func (r *smokeMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *smokeMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) { return nil, nil }
func (r *smokeMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *smokeMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, monitor.ErrNotFound
}

type smokeNotifyRepo struct{}

func (r *smokeNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *smokeNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *smokeNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}
