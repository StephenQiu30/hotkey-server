package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
	"github.com/gin-gonic/gin"
)

func TestCollectionAdminHTTPIntegrationUsesSafeDTOsAndDurableStateCommands(t *testing.T) {
	runtime := collectionHTTPRuntime(t)
	defer func() { _ = runtime.Close() }()
	request, runID := collectionHTTPFailedRun(t, runtime)
	checkedAt := time.Date(2026, time.July, 16, 14, 0, 0, 0, time.UTC)
	service, err := sourceapplication.NewCollectionControlService(sourceapplication.CollectionControlDependencies{
		Runtime: runtime, Sources: sourcepostgres.NewRepository(runtime), Runs: sourcepostgres.NewCollectionRepository(runtime),
		Connectors: collectionHTTPRegistry{connector: collectionHTTPConnector{health: domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorTemporary, DiagnosticCode: "request_failed"}}},
		Now:        func() time.Time { return checkedAt },
	})
	if err != nil {
		t.Fatalf("NewCollectionControlService(): %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCollectionRoutes(router, service, testAuthenticator{subject: httptransport.Subject{UserID: 1, SessionID: 1, Role: httptransport.RoleAdmin}})

	list := collectionHTTPRequest(t, router, http.MethodGet, "/api/v1/collection-runs", http.StatusOK)
	for _, forbidden := range []string{"source_connection_id", "query_signature", "request_cursor", "next_cursor", "etag", "last_modified", "endpoint", "credential", "integration-secret-marker"} {
		if strings.Contains(list, forbidden) {
			t.Fatalf("collection list leaked %q: %s", forbidden, list)
		}
	}
	var listResult struct {
		Code int `json:"code"`
		Data struct {
			Items []CollectionRunResponse `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(list), &listResult); err != nil || listResult.Code != 0 || len(listResult.Data.Items) != 1 || listResult.Data.Items[0].Status != string(domain.CollectionRunFailed) {
		t.Fatalf("list result/error = %#v / %v, want one failed safe run", listResult, err)
	}

	retried := collectionHTTPRequest(t, router, http.MethodPost, fmt.Sprintf("/api/v1/collection-runs/%d/retry", runID), http.StatusOK)
	if strings.Contains(retried, "integration-secret-marker") || !strings.Contains(retried, `"status":"queued"`) {
		t.Fatalf("retry response = %s, want safe queued run", retried)
	}
	health := collectionHTTPRequest(t, router, http.MethodPost, fmt.Sprintf("/api/v1/source-connections/%d/health", request.SourceConnectionID), http.StatusOK)
	if strings.Contains(health, "integration-secret-marker") || !strings.Contains(health, `"error_code":"request_failed"`) || strings.Contains(health, "endpoint") {
		t.Fatalf("health response = %s, want safe probe result", health)
	}
	var healthStatus string
	if err := runtime.SQL.QueryRow(`SELECT health_status FROM source_connections WHERE id = $1`, request.SourceConnectionID).Scan(&healthStatus); err != nil {
		t.Fatalf("read persisted health: %v", err)
	}
	if healthStatus != string(domain.HealthStatusDegraded) {
		t.Fatalf("health status = %q, want %q", healthStatus, domain.HealthStatusDegraded)
	}
}

func collectionHTTPRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	return runtime
}

func collectionHTTPFailedRun(t *testing.T, runtime *database.Runtime) (domain.CollectionRequest, int64) {
	t.Helper()
	connection := domain.SourceConnection{
		SourceType: domain.SourceTypeRSS, Name: "Integration source", Endpoint: "https://feeds.example.test/rss?marker=integration-secret-marker",
		AuthType: domain.AuthTypeNone, Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusUnknown,
	}
	if err := sourcepostgres.NewRepository(runtime).Create(context.Background(), &connection); err != nil {
		t.Fatalf("create source: %v", err)
	}
	signature := strings.Repeat("d", 64)
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	var monitorID, configID, monitorSourceID, checkpointID, checkpointVersion int64
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ('collection-http-monitor') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create config: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_sources (config_version_id, source_connection_id, query_signature) VALUES ($1, $2, $3) RETURNING id`, configID, connection.ID, signature).Scan(&monitorSourceID); err != nil {
		t.Fatalf("create monitor source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO source_checkpoints (monitor_source_id, query_hash, next_poll_at) VALUES ($1, $2, $3) RETURNING id, version`, monitorSourceID, signature, windowStart).Scan(&checkpointID, &checkpointVersion); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	request := domain.CollectionRequest{
		SourceConnectionID: connection.ID, QuerySignature: signature, Query: "safe query should not leave source", Languages: []string{"en"},
		WindowStart: windowStart, WindowEnd: windowStart.Add(time.Hour), Targets: []domain.PublishedCollectionTarget{{
			MonitorSourceID: monitorSourceID, MonitorConfigVersionID: configID, SourceConnectionID: connection.ID, QuerySignature: signature,
			Terms: []domain.CollectionTerm{{Value: "safe"}}, Languages: []string{"en"}, CollectionInterval: 5 * time.Minute,
			Checkpoint: domain.CollectionCheckpoint{ID: checkpointID, Version: checkpointVersion, MonitorSourceID: monitorSourceID, QueryHash: signature, NextPollAt: windowStart},
		}},
	}
	runs := sourcepostgres.NewCollectionRepository(runtime)
	run, _, err := runs.CreateOrReuseRun(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateOrReuseRun(): %v", err)
	}
	if _, started, err := runs.StartRun(context.Background(), run.ID, time.Time{}); err != nil || !started {
		t.Fatalf("StartRun() started/error = %t / %v", started, err)
	}
	if _, err := runs.PersistFailure(context.Background(), domain.CollectionRunFailure{RunID: run.ID, Targets: request.Targets, ErrorKind: domain.CollectionErrorTemporary, CompletedAt: request.WindowEnd}); err != nil {
		t.Fatalf("PersistFailure(): %v", err)
	}
	return request, run.ID
}

func collectionHTTPRequest(t *testing.T, router *gin.Engine, method, path string, wantStatus int) string {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d: %s", method, path, response.Code, wantStatus, response.Body.String())
	}
	return response.Body.String()
}

type collectionHTTPRegistry struct{ connector domain.Connector }

func (registry collectionHTTPRegistry) Resolve(context.Context, domain.SourceConnection) (domain.Connector, error) {
	return registry.connector, nil
}

type collectionHTTPConnector struct{ health domain.HealthResult }

func (collectionHTTPConnector) Validate(context.Context, domain.SourceConnection) error { return nil }
func (collectionHTTPConnector) Fetch(context.Context, domain.FetchRequest) (domain.FetchResult, error) {
	return domain.FetchResult{}, nil
}
func (connector collectionHTTPConnector) Health(context.Context, domain.SourceConnection) domain.HealthResult {
	return connector.health
}
