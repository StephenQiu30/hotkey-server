// cmd/openapi generates the OpenAPI specification for the HotKey Server API.
// It registers all routes with stub services and exports the OpenAPI JSON.
package main

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

func main() {
	// Create the API with stub services to generate the OpenAPI spec.
	api, _ := platformhttp.NewAPI(platformhttp.Config{
		JWTSecret:     "openapi-gen-placeholder",
		SmokeTest:     false,
		AuthService:   auth.NewService(&stubAuthRepo{}),
		MonitorSvc:    monitor.NewService(&stubMonitorRepo{}),
		NotifySvc:     notify.NewService(&stubNotifyRepo{}),
		PostQuerySvc:  &stubPostQueryService{},
		TopicQuerySvc: &stubTopicQueryService{},
		TrendQuerySvc: &stubTrendQueryService{},
	})

	// Marshal the OpenAPI spec to JSON.
	spec := api.OpenAPI()
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		log.Fatalf("failed to marshal OpenAPI spec: %v", err)
	}

	// Write to docs/openapi.json.
	if err := os.MkdirAll("docs", 0o755); err != nil {
		log.Fatalf("failed to create docs directory: %v", err)
	}
	if err := os.WriteFile("docs/openapi.json", data, 0o644); err != nil {
		log.Fatalf("failed to write openapi.json: %v", err)
	}

	log.Printf("wrote docs/openapi.json (%d bytes)", len(data))
}

// --- Stub implementations for OpenAPI generation ---

type stubAuthRepo struct{}

func (r *stubAuthRepo) ExistsByEmail(_ context.Context, _ string) bool { return false }
func (r *stubAuthRepo) Create(_ context.Context, _, _, _ string) (auth.User, error) {
	return auth.User{}, nil
}
func (r *stubAuthRepo) GetByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *stubAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

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
	return monitor.Monitor{}, nil
}

type stubNotifyRepo struct{}

func (r *stubNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *stubNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *stubNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

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
