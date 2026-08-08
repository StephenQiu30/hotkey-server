package postgres_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestRuntimeMetricsCollectorProjectsLowCardinalityOperationalFacts(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	var sourceID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO source_connections (source_type,name,endpoint,auth_type,config)
VALUES ('rss','private-source-name','https://secret.example.test/feed','none','{}')
RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO collection_runs
    (source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at,started_at,finished_at,status)
VALUES ($1,repeat('a',64),now()-interval '2 hours',now()-interval '1 hour','retry',now()-interval '2 hours',now()-interval '90 minutes',now()-interval '89 minutes','failed')`, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO river_job (kind,args,state,attempt,max_attempts,priority,scheduled_at,attempted_at,finalized_at,unique_key)
VALUES ('collect_source','{"entity_id":42,"entity_version":1}','discarded',2,2,1,now()-interval '5 minutes',now()-interval '4 minutes',now()-interval '3 minutes','metrics-job')`); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO users (email,password_hash,display_name,role)
VALUES ('runtime-metrics@example.test','hash','Runtime metrics','viewer') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO quota_usage_ledgers (dimension,subject_type,subject_id,window_start,window_end,used)
VALUES
	  ('manual_searches','user',$1,date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'+interval '1 day',2),
	  ('manual_searches','user',$1,date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'-interval '1 day',date_trunc('day',now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',99)`, userID); err != nil {
		t.Fatal(err)
	}

	registry := prometheus.NewRegistry()
	if err := registry.Register(operationspostgres.NewRuntimeMetricsCollector(runtime)); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/metrics", nil)
	response := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("metrics status = %d", response.Code)
	}
	body := response.Body.String()
	for _, metric := range []string{
		"hotkey_source_collection_runs_total",
		"hotkey_source_collection_duration_seconds_sum",
		"hotkey_source_collection_retries_total",
		"hotkey_job_runs_total",
		"hotkey_job_duration_seconds_sum",
		"hotkey_job_retries_total",
		"hotkey_job_queue_lag_seconds",
		"hotkey_usage_current",
		"hotkey_quota_limit",
		"hotkey_ai_cost_usd",
		"hotkey_runtime_metrics_collection_success",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("metrics output is missing %s", metric)
		}
	}
	for _, secret := range []string{"private-source-name", "secret.example.test", "entity_id", "42"} {
		if strings.Contains(body, secret) {
			t.Errorf("metrics output leaked high-cardinality or private value %q", secret)
		}
	}
	if !strings.Contains(body, `source_type="rss"`) || !strings.Contains(body, `kind="collect_source"`) {
		t.Fatalf("metrics output does not retain stable source/job labels: %s", body)
	}
	if !strings.Contains(body, `hotkey_runtime_metrics_collection_success 1`) {
		t.Fatalf("runtime collection health was not successful: %s", body)
	}
	if !strings.Contains(body, `hotkey_usage_current{dimension="manual_searches",scope="all_users"} 2`) {
		t.Fatalf("manual search usage was not isolated from other ledgers: %s", body)
	}
}

func TestRuntimeMetricsCollectorDegradesWithoutExposingDatabaseErrors(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	runtime.Close()

	registry := prometheus.NewRegistry()
	if err := registry.Register(operationspostgres.NewRuntimeMetricsCollector(runtime)); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, `hotkey_runtime_metrics_collection_success 0`) {
		t.Fatalf("degraded metrics = status %d body %s", response.Code, body)
	}
	for _, forbidden := range []string{"database is closed", "sql:", "postgres://"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("degraded metrics leaked database detail %q: %s", forbidden, body)
		}
	}
}
