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
	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
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

	// Wire content, topic, trend query services.
	var postQuerySvc content.PostQueryService
	var topicQuerySvc topic.TopicQueryService
	var trendQuerySvc trend.TrendQueryService

	if smokeTest {
		// In smoke test mode, use in-memory stubs (no database required).
		authRepo = &smokeAuthRepo{}
		monitorRepo = &smokeMonitorRepo{}
		notifyRepo = &smokeNotifyRepo{}
		postQuerySvc = &smokePostQueryService{}
		topicQuerySvc = &smokeTopicQueryService{}
		trendQuerySvc = &smokeTrendQueryService{}
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
		postQuerySvc = database.NewContentQueryService(db)
		topicQuerySvc = database.NewTopicQueryService(db)
		trendQuerySvc = database.NewTrendQueryService(db)
	}

	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc, cfg.JWTSecret)

	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)

	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)

	postHandler := content.NewPostHandler(postQuerySvc)
	topicHandler := topic.NewTopicHandler(topicQuerySvc)
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
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("worker", "starting"))

	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Print(observability.RenderLog("worker", "shutting down"))
		cancel()
	}()

	// Wire platform connector (X API)
	xClient := x.NewClient(cfg.XToken, cfg.XBaseURL)
	connector := jobs.NewXConnectorAdapter(xClient, cfg.XToken)

	// Wire scoring
	hitScorerRepo := jobs.NewDBHitScorerRepo(db)
	scoringSvc := scoring.NewService(hitScorerRepo)
	scorer := jobs.NewScorerAdapter(scoringSvc)

	// Wire PollMonitorJob
	runRepo := jobs.NewDBRunRepository(db)
	postRepo := jobs.NewDBPostRepository(db)
	hitRepo := jobs.NewDBHitRepository(db)
	pollJob := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, scorer)

	// Wire topic aggregation
	topicRepo := database.NewTopicRepo(db)
	postCandidateProvider := jobs.NewDBPostCandidateProvider(db)
	topicPersister := jobs.NewTopicPersisterAdapter(topicRepo)
	aggregateJob := jobs.NewAggregateTopicsJob(postCandidateProvider, topicPersister)

	// Wire snapshot building
	trendRepo := database.NewTrendRepo(db)
	trendSvc := trend.NewService(trendRepo)
	topicProvider := jobs.NewDBTopicProvider(db)
	snapshotJob := jobs.NewBuildSnapshotsJob(trendSvc, topicProvider)

	// Wire dispatch job
	deliveryRepo := jobs.NewDBDeliveryRepository(db)
	emailResolver := jobs.NewDBUserEmailLookup(db)
	mailer := &noopMailer{}
	dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, emailResolver)

	// Wire monitor lister for worker jobs
	monitorLister := jobs.NewDBMonitorLister(db)

	// Register background jobs
	runner := jobs.NewRunner()
	runner.Register("poll_monitor", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "poll_monitor: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if err := pollJob.Run(ctx, jobs.MonitorInfo{
				ID:       monitorID,
				Platform: "x",
			}); err != nil {
				log.Printf("poll_monitor: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 1*time.Minute)
	runner.Register("aggregate_topics", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_topics: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if _, err := aggregateJob.Run(jobs.AggregateTopicsInput{MonitorID: monitorID, RunTime: time.Now()}); err != nil {
				log.Printf("aggregate_topics: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 5*time.Minute)
	runner.Register("build_snapshots", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "build_snapshots: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if _, err := snapshotJob.Run(jobs.BuildSnapshotsInput{MonitorID: monitorID, SnapshotTime: time.Now()}); err != nil {
				log.Printf("build_snapshots: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 10*time.Minute)
	runner.Register("dispatch_notifications", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
		return dispatchJob.Run(ctx, 0)
	}, 1*time.Minute)

	log.Print(observability.RenderLog("worker", "ready, running jobs"))
	runner.Run(ctx)
}

// noopMailer is a mailer that logs instead of sending (no SMTP configured).
type noopMailer struct{}

func (m *noopMailer) Send(_ context.Context, to, subject, _ string) (string, error) {
	log.Printf("mailer: would send to %s subject=%q (noop)", to, subject)
	return "noop-msg-id", nil
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

type smokePostQueryService struct{}

func (s *smokePostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type smokeTopicQueryService struct{}

func (s *smokeTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type smokeTrendQueryService struct{}

func (s *smokeTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

func (s *smokeTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
