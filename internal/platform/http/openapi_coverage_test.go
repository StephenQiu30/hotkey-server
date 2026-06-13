package http_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// expectedOperations lists every operationId that must appear in docs/openapi.json.
// Derived from huma.Register calls in auth.go, monitor.go, content.go,
// topic.go, trend.go, notify.go, and health.go.
var expectedOperations = []string{
	"register",
	"login",
	"list-monitors",
	"create-monitor",
	"get-monitor",
	"update-monitor",
	"list-posts",
	"list-topics",
	"get-monitor-trends",
	"get-topic-trends",
	"list-notifications",
	"mark-notification-read",
	"health-check",
}

// expectedAPIv1Paths lists every /api/v1/* path that must be covered.
var expectedAPIv1Paths = []string{
	"/api/v1/auth/login",
	"/api/v1/auth/register",
	"/api/v1/monitors",
	"/api/v1/monitors/{id}",
	"/api/v1/monitors/{id}/posts",
	"/api/v1/monitors/{id}/topics",
	"/api/v1/monitors/{id}/trends",
	"/api/v1/notifications",
	"/api/v1/notifications/{id}/read",
	"/api/v1/topics/{id}/trends",
}

// openapiSpec is a minimal struct for parsing the OpenAPI JSON.
type openapiSpec struct {
	OpenAPI string                    `json:"openapi"`
	Info    map[string]interface{}    `json:"info"`
	Paths   map[string]map[string]any `json:"paths"`
}

// TestOpenAPICoverage verifies that the generated docs/openapi.json covers
// all Huma-registered operations and /api/v1/* paths.
func TestOpenAPICoverage(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v (run `make openapi` first)", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	// Collect all operationIds from the spec.
	gotOps := collectOperationIDs(spec.Paths)

	// Check every expected operation is present.
	for _, want := range expectedOperations {
		if !gotOps[want] {
			t.Errorf("missing operationId %q in docs/openapi.json", want)
		}
	}

	// Check every expected /api/v1/* path is present.
	for _, want := range expectedAPIv1Paths {
		if _, ok := spec.Paths[want]; !ok {
			t.Errorf("missing path %q in docs/openapi.json", want)
		}
	}
}

// TestOpenAPIVersion verifies the spec declares OpenAPI 3.1.0.
func TestOpenAPIVersion(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	if spec.OpenAPI != "3.1.0" {
		t.Errorf("expected openapi version 3.1.0, got %q", spec.OpenAPI)
	}
}

// TestOpenAPISecurityScheme verifies the spec declares BearerAuth.
func TestOpenAPISecurityScheme(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	components, _ := raw["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	if _, ok := schemes["bearer"]; !ok {
		t.Error("missing securityScheme 'bearer' in docs/openapi.json")
	}
}

// TestOpenAPIPathCount verifies total API path count matches expectations.
func TestOpenAPIPathCount(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "docs", "openapi.json")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("cannot read docs/openapi.json: %v", err)
	}

	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("docs/openapi.json is not valid JSON: %v", err)
	}

	// We expect at least len(expectedAPIv1Paths) + 1 (healthz).
	minPaths := len(expectedAPIv1Paths) + 1
	if len(spec.Paths) < minPaths {
		t.Errorf("expected at least %d paths, got %d", minPaths, len(spec.Paths))
	}
}

// TestOpenAPIFromHumaAPI generates the spec from the Huma API in-process and
// verifies it matches the expected operations — independent of the file on disk.
func TestOpenAPIFromHumaAPI(t *testing.T) {
	api, _ := platformhttp.NewAPI(platformhttp.Config{
		JWTSecret:     "test-placeholder",
		SmokeTest:     false,
		AuthService:   auth.NewService(&noopAuthRepo{}),
		MonitorSvc:    monitor.NewService(&noopMonitorRepo{}),
		NotifySvc:     notify.NewService(&noopNotifyRepo{}),
		PostQuerySvc:  &noopPostQueryService{},
		TopicQuerySvc: &noopTopicQueryService{},
		TrendQuerySvc: &noopTrendQueryService{},
	})

	spec := api.OpenAPI()

	// Collect operationIds from all HTTP method fields on each PathItem.
	gotOps := make(map[string]bool)
	for _, pathItem := range spec.Paths {
		for _, op := range []*huma.Operation{
			pathItem.Get, pathItem.Put, pathItem.Post, pathItem.Delete,
			pathItem.Patch, pathItem.Options, pathItem.Head, pathItem.Trace,
		} {
			if op != nil && op.OperationID != "" {
				gotOps[op.OperationID] = true
			}
		}
	}

	for _, want := range expectedOperations {
		if !gotOps[want] {
			t.Errorf("Huma API missing operationId %q", want)
		}
	}
}

// collectOperationIDs extracts all operationIds from an OpenAPI paths map.
func collectOperationIDs(paths map[string]map[string]any) map[string]bool {
	ops := make(map[string]bool)
	httpMethods := map[string]bool{
		"get": true, "post": true, "put": true,
		"patch": true, "delete": true, "head": true, "options": true,
	}
	for _, methods := range paths {
		for method, val := range methods {
			if !httpMethods[method] {
				continue
			}
			opMap, ok := val.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := opMap["operationId"].(string); ok {
				ops[id] = true
			}
		}
	}
	return ops
}

// --- Noop stubs for in-process Huma API generation ---

type noopAuthRepo struct{}

func (r *noopAuthRepo) ExistsByEmail(_ context.Context, _ string) bool { return false }
func (r *noopAuthRepo) Create(_ context.Context, _, _, _ string) (auth.User, error) {
	return auth.User{}, nil
}
func (r *noopAuthRepo) GetByEmail(_ context.Context, _ string) (*auth.User, error) {
	return nil, nil
}
func (r *noopAuthRepo) GetByID(_ context.Context, _ int64) (*auth.User, error) { return nil, nil }

type noopMonitorRepo struct{}

func (r *noopMonitorRepo) Create(_ context.Context, _ int64, _ monitor.CreateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}
func (r *noopMonitorRepo) GetByID(_ context.Context, _ int64) (*monitor.Monitor, error) {
	return nil, nil
}
func (r *noopMonitorRepo) ListByUser(_ context.Context, _ int64) ([]monitor.Monitor, error) {
	return nil, nil
}
func (r *noopMonitorRepo) Update(_ context.Context, _ int64, _ monitor.UpdateMonitorInput) (monitor.Monitor, error) {
	return monitor.Monitor{}, nil
}

type noopNotifyRepo struct{}

func (r *noopNotifyRepo) ListUnread(_ context.Context, _ int64) ([]notify.Notification, error) {
	return nil, nil
}
func (r *noopNotifyRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (r *noopNotifyRepo) Create(_ context.Context, n notify.Notification) (notify.Notification, error) {
	return n, nil
}

type noopPostQueryService struct{}

func (s *noopPostQueryService) ListPostsByMonitor(_ int64, _, _ int) ([]content.PostSummary, error) {
	return nil, nil
}

type noopTopicQueryService struct{}

func (s *noopTopicQueryService) ListByMonitor(_ int64) ([]topic.TopicSummary, error) {
	return nil, nil
}

type noopTrendQueryService struct{}

func (s *noopTrendQueryService) GetTopicTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
func (s *noopTrendQueryService) GetMonitorTrends(_ int64, _ time.Time) ([]trend.TrendPoint, error) {
	return nil, nil
}
