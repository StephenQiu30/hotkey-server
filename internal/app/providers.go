package app

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/server"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// DB opens a database connection.
func provideDB(cfg config.Config) (*sql.DB, error) {
	return database.Open(cfg.DatabaseURL)
}

// AuthRepo provides the auth repository.
func provideAuthRepo(db *sql.DB) auth.Repository {
	return database.NewAuthRepo(db)
}

// MonitorRepo provides the monitor repository.
func provideMonitorRepo(db *sql.DB) monitor.Repository {
	return database.NewMonitorRepo(db)
}

// NotifyRepo provides the notify repository.
func provideNotifyRepo(db *sql.DB) notify.Repository {
	return database.NewNotifyRepo(db)
}

// AuthMiddleware creates the auth middleware.
func provideAuthMiddleware(cfg config.Config) func(http.Handler) http.Handler {
	return server.AuthMiddleware(cfg.JWTSecret)
}

// PostQueryService provides a stub post query service (until content repo is implemented).
func providePostQueryService() content.PostQueryService {
	return &stubPostQueryService{}
}

// TopicQueryService provides a stub topic query service (until topic repo is implemented).
func provideTopicQueryService() topic.TopicQueryService {
	return &stubTopicQueryService{}
}

// TrendQueryService provides a stub trend query service (until trend repo is implemented).
func provideTrendQueryService() trend.TrendQueryService {
	return &stubTrendQueryService{}
}

// Stub services for content/topic/trend (until repos are implemented)

type stubPostQueryService struct{}

func (s *stubPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type stubTopicQueryService struct{}

func (s *stubTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type stubTrendQueryService struct{}

func (s *stubTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

func (s *stubTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}

// newSmokeRouter builds an HTTP handler using in-memory stubs for SMOKE_TEST=1 mode.
// No database connection is required.
func newSmokeRouter(cfg config.Config) (http.Handler, error) {
	authSvc := auth.NewService(&smokeAuthRepo{})
	authHandler := auth.NewHandler(authSvc, cfg.JWTSecret)

	monitorSvc := monitor.NewService(&smokeMonitorRepo{})
	monitorHandler := monitor.NewHandler(monitorSvc)

	notifySvc := notify.NewService(&smokeNotifyRepo{})
	notifyHandler := notify.NewHandler(notifySvc)

	postHandler := content.NewPostHandler(&stubPostQueryService{})
	topicHandler := topic.NewTopicHandler(&stubTopicQueryService{})
	trendHandler := trend.NewTrendHandler(&stubTrendQueryService{})

	// Smoke auth middleware: bypasses JWT, injects default user ID.
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := monitor.ContextWithUserID(r.Context(), 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	return server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		TopicHandler:        topicHandler,
		TrendHandler:        trendHandler,
		PostHandler:         postHandler,
		NotificationHandler: notifyHandler,
		AuthMiddleware:      authMiddleware,
	}), nil
}

// Smoke test stubs — in-memory implementations for SMOKE_TEST=1 mode.

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
