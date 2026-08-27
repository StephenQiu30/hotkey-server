package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func TestMonitorScanReaderKeepsIndependentThreeSourceFacts(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	scheduledAt := time.Date(2026, time.August, 27, 6, 0, 0, 0, time.UTC)
	fixture := seedCollectionTargetForSource(t, runtime, "scan-rss", sourcedomain.SourceTypeRSS, "draft", false, true, true, false, scheduledAt)

	hnSourceID, hnMonitorSourceID := seedAdditionalScanSource(t, runtime, fixture.configID, sourcedomain.SourceTypeHackerNews, "Hacker News", "https://hacker-news.firebaseio.com/v0", "none")
	xSourceID, xMonitorSourceID := seedAdditionalScanSource(t, runtime, fixture.configID, sourcedomain.SourceTypeX, "X", sourcedomain.XRecentSearchEndpoint, "bearer")

	insertScanFact(t, runtime, fixture.sourceID, fixture.monitorSourceID, fixture.configID, scheduledAt, "succeeded", 3, 2, 1, "")
	insertScanFact(t, runtime, hnSourceID, hnMonitorSourceID, fixture.configID, scheduledAt, "succeeded", 0, 0, 0, "")
	insertScanFact(t, runtime, xSourceID, xMonitorSourceID, fixture.configID, scheduledAt, "failed", 0, 0, 0, "rate_limited")

	facts, err := monitorpostgres.NewMonitorScanReader(runtime).ListMonitorScans(context.Background(), fixture.monitorID, 10)
	if err != nil {
		t.Fatalf("ListMonitorScans(): %v", err)
	}
	if len(facts) != 3 {
		t.Fatalf("scan facts = %#v, want three independent sources", facts)
	}
	if facts[0].SourceType != "rss" || facts[0].Status != sourcedomain.CollectionRunSucceeded || facts[0].AcceptedCount != 2 {
		t.Fatalf("RSS fact = %#v", facts[0])
	}
	if facts[1].SourceType != "hacker_news" || facts[1].Status != sourcedomain.CollectionRunSucceeded || facts[1].CandidateCount != 0 {
		t.Fatalf("Hacker News zero-result fact = %#v", facts[1])
	}
	if facts[2].SourceType != "x" || facts[2].Status != sourcedomain.CollectionRunFailed || facts[2].ErrorCode != "rate_limited" {
		t.Fatalf("X rate-limit fact = %#v", facts[2])
	}
}

func seedAdditionalScanSource(t *testing.T, runtime *database.Runtime, configID int64, sourceType sourcedomain.SourceType, name, endpoint, authType string) (int64, int64) {
	t.Helper()
	var credential any
	if authType == "bearer" {
		credential = "env:X_BEARER_TOKEN"
	}
	var sourceID, monitorSourceID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint,auth_type,credential_ref,config,enabled,health_status)
VALUES ($1,$2,$3,$4,$5,'{}'::jsonb,true,'unknown') RETURNING id`, sourceType, name, endpoint, authType, credential).Scan(&sourceID); err != nil {
		t.Fatalf("insert %s source: %v", sourceType, err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_sources (config_version_id,source_connection_id,query_signature,enabled)
VALUES ($1,$2,$3,true) RETURNING id`, configID, sourceID, strings.Repeat(string(sourceType)[0:1], 64)).Scan(&monitorSourceID); err != nil {
		t.Fatalf("insert %s monitor source: %v", sourceType, err)
	}
	return sourceID, monitorSourceID
}

func insertScanFact(t *testing.T, runtime *database.Runtime, sourceID, monitorSourceID, configID int64, scheduledAt time.Time, status string, candidateCount, acceptedCount, rejectedCount int64, errorCode string) {
	t.Helper()
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO collection_runs (
  source_connection_id,query_signature,window_start,window_end,trigger_type,scheduled_at,
  started_at,finished_at,status,candidate_count,accepted_count,rejected_count,error_code
) VALUES ($1,$2,$3,$4,'manual',$4,$4,$5,$6,$7,$8,$9,NULLIF($10,'')) RETURNING id`,
		sourceID, strings.Repeat("f", 64), scheduledAt.Add(-time.Hour), scheduledAt, scheduledAt.Add(time.Second),
		status, candidateCount, acceptedCount, rejectedCount, errorCode).Scan(&runID); err != nil {
		t.Fatalf("insert %s run fact: %v", status, err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO collection_run_targets (
  collection_run_id,monitor_source_id,monitor_config_version_id,target_status,
  candidate_count,accepted_count,rejected_count,error_code
) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''))`,
		runID, monitorSourceID, configID, status, candidateCount, acceptedCount, rejectedCount, errorCode); err != nil {
		t.Fatalf("insert %s target fact: %v", status, err)
	}
}
