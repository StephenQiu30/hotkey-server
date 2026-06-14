package app

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// GenerateOpenAPI writes docs/openapi.json from the Huma route definitions.
func GenerateOpenAPI() {
	api, _ := platformhttp.NewAPI(platformhttp.Config{
		JWTSecret:     "openapi-gen-placeholder",
		SmokeTest:     false,
		AuthService:   auth.NewService(&openapiAuthRepo{}),
		MonitorSvc:    monitor.NewService(&openapiMonitorRepo{}),
		NotifySvc:     notify.NewService(&openapiNotifyRepo{}),
		PostQuerySvc:  &openapiPostQueryService{},
		TopicQuerySvc: &openapiTopicQueryService{},
		TrendQuerySvc: &openapiTrendQueryService{},
	})

	data, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal OpenAPI spec: %v", err)
	}

	if err := os.MkdirAll("docs", 0o755); err != nil {
		log.Fatalf("failed to create docs directory: %v", err)
	}
	if err := os.WriteFile("docs/openapi.json", data, 0o644); err != nil {
		log.Fatalf("failed to write openapi.json: %v", err)
	}

	log.Printf("wrote docs/openapi.json (%d bytes)", len(data))
}

type openapiAuthRepo struct{}

func (r *openapiAuthRepo) ExistsByEmail(_ context.Context, _ string) bool { return false }
func (r *openapiAuthRepo) Create(_ context.Context, _, _, _ string) (auth.User, error) {
	return auth.User{}, nil
}
func (r *openapiAuthRepo) GetByEmail(_ context.Context, _ string) (*auth.User, error) { return nil, nil }
func (r *openapiAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error)     { return nil, nil }

type openapiMonitorRepo struct{}

func (r *openapiMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *openapiMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) { return nil, nil }
func (r *openapiMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *openapiMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}

type openapiNotifyRepo struct{}

func (r *openapiNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *openapiNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *openapiNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

type openapiPostQueryService struct{}

func (s *openapiPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type openapiTopicQueryService struct{}

func (s *openapiTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type openapiTrendQueryService struct{}

func (s *openapiTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
func (s *openapiTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
