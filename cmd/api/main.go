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

	// Wire content (posts)
	postHandler := content.NewPostHandler(&stubPostQueryService{})

	// Wire topics
	topicHandler := topic.NewTopicHandler(&stubTopicQueryService{})

	// Wire trends
	trendHandler := trend.NewTrendHandler(&stubTrendQueryService{})

	// Auth middleware: validates token and injects user ID into context.
	// When SMOKE_TEST=1, bypasses auth and injects a default user ID for smoke testing.
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if os.Getenv("SMOKE_TEST") == "1" {
				ctx := monitor.ContextWithUserID(r.Context(), 1)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// TODO: Implement real JWT/token validation.
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}

	router := server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		PostHandler:         postHandler,
		TopicHandler:        topicHandler,
		TrendHandler:        trendHandler,
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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Register background jobs (poll_monitor, aggregate_topics, build_snapshots, dispatch_notifications)
	// when the jobs module is wired with real repositories. See internal/jobs/.
	log.Print(observability.RenderLog("worker", "ready, waiting for jobs"))

	<-sigCh
	log.Print(observability.RenderLog("worker", "shutting down"))
}

// stubAuthRepo is a placeholder repository for smoke testing.
// Tracks registered users in-memory so login can validate credentials.
type stubAuthRepo struct {
	users []auth.User
	nextID int64
}

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}
func (r *stubAuthRepo) Create(_ context.Context, email, passwordHash, displayName string) (auth.User, error) {
	r.nextID++
	u := auth.User{
		ID:           r.nextID,
		Email:        email,
		PasswordHash: passwordHash,
		DisplayName:  displayName,
	}
	r.users = append(r.users, u)
	return u, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, email string) (*auth.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
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

// stubPostQueryService is a placeholder for content post queries.
type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

// stubTopicQueryService is a placeholder for topic queries.
type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

// stubTrendQueryService is a placeholder for trend queries.
type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
