package app

import (
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
