package postgres_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
)

func TestPublishedCollectionTargetReaderReturnsOnlyDueActivePublishedEnabledTargets(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)

	due := seedCollectionTarget(t, runtime.SQL, "due", "active", true, true, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "paused", "paused", true, true, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "disabled", "active", true, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "draft", "draft", false, true, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "future", "active", true, true, now.Add(time.Minute))

	var before int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs`).Scan(&before); err != nil {
		t.Fatalf("count collection runs before read: %v", err)
	}
	targets, err := monitorpostgres.NewPublishedCollectionTargetReader(runtime).ListDue(context.Background(), now)
	if err != nil {
		t.Fatalf("ListDue(): %v", err)
	}
	var after int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs`).Scan(&after); err != nil {
		t.Fatalf("count collection runs after read: %v", err)
	}
	if before != after {
		t.Fatalf("ListDue wrote collection facts: before=%d after=%d", before, after)
	}
	if len(targets) != 1 {
		t.Fatalf("ListDue() targets = %#v, want only one active published enabled due target", targets)
	}
	target := targets[0]
	if target.MonitorSourceID != due.monitorSourceID || target.MonitorConfigVersionID != due.configID || target.SourceConnectionID != due.sourceID {
		t.Fatalf("target identity = %#v, want immutable source/config/connection IDs %#v", target, due)
	}
	if target.QuerySignature != strings.Repeat("a", 64) || target.Checkpoint.MonitorSourceID != due.monitorSourceID || !target.Checkpoint.NextPollAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("target signature/checkpoint = %#v", target)
	}
	if target.CollectionInterval != 5*time.Minute || len(target.Languages) != 1 || target.Languages[0] != "en" {
		t.Fatalf("target locale/interval = %#v", target)
	}
	if len(target.Terms) != 2 || target.Terms[0] != (sourcedomain.CollectionTerm{Value: "climate"}) || target.Terms[1] != (sourcedomain.CollectionTerm{Value: "spam", Excluded: true}) {
		t.Fatalf("target terms = %#v, want approved immutable include/exclude terms only", target.Terms)
	}
}

type seededCollectionTarget struct {
	sourceID, monitorSourceID, configID int64
}

func seedCollectionTarget(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}, suffix, monitorStatus string, published, sourceEnabled bool, nextPollAt time.Time) seededCollectionTarget {
	t.Helper()
	var result seededCollectionTarget
	if err := runtime.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, config, enabled, health_status) VALUES ('rss', $1, 'https://feeds.example.test/collection', 'none', '{}'::jsonb, true, 'unknown') RETURNING id`, "collection source "+suffix).Scan(&result.sourceID); err != nil {
		t.Fatalf("seed %s source: %v", suffix, err)
	}
	var monitorID int64
	if err := runtime.QueryRow(`INSERT INTO monitors (name, status) VALUES ($1, 'draft') RETURNING id`, "collection monitor "+suffix).Scan(&monitorID); err != nil {
		t.Fatalf("seed %s monitor: %v", suffix, err)
	}
	if err := runtime.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision, state, timezone, languages, regions, collection_interval_seconds, relevance_threshold, event_threshold, retention_days) VALUES ($1, 1, 'draft', 'UTC', ARRAY['en'], ARRAY[]::text[], 300, 60, 0, 30) RETURNING id`, monitorID).Scan(&result.configID); err != nil {
		t.Fatalf("seed %s config: %v", suffix, err)
	}
	if _, err := runtime.Exec(`UPDATE monitors SET draft_config_version_id = $1 WHERE id = $2`, result.configID, monitorID); err != nil {
		t.Fatalf("set %s draft pointer: %v", suffix, err)
	}
	if err := runtime.QueryRow(`INSERT INTO monitor_sources (config_version_id, source_connection_id, query_signature, enabled) VALUES ($1, $2, $3, $4) RETURNING id`, result.configID, result.sourceID, strings.Repeat("a", 64), sourceEnabled).Scan(&result.monitorSourceID); err != nil {
		t.Fatalf("seed %s monitor source: %v", suffix, err)
	}
	for _, rule := range []struct {
		ruleType string
		operator string
		value    string
		approval string
	}{
		{"keyword", "contains", "climate", "approved"},
		{"exclude_keyword", "contains", "spam", "approved"},
		{"keyword", "contains", "pending", "pending"},
	} {
		if _, err := runtime.Exec(`INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, weight, approval_status, enabled) VALUES ($1, $2, $3, $4, 0, $5, true)`, result.configID, rule.ruleType, rule.operator, rule.value, rule.approval); err != nil {
			t.Fatalf("seed %s rule: %v", suffix, err)
		}
	}
	if published {
		if _, err := runtime.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = $2 WHERE id = $3`, strings.Repeat("b", 64), nextPollAt.Add(-time.Hour), result.configID); err != nil {
			t.Fatalf("publish %s config: %v", suffix, err)
		}
		if _, err := runtime.Exec(`UPDATE monitors SET status = $1, draft_config_version_id = NULL, published_config_version_id = $2 WHERE id = $3`, monitorStatus, result.configID, monitorID); err != nil {
			t.Fatalf("publish %s monitor: %v", suffix, err)
		}
	}
	if _, err := runtime.Exec(`INSERT INTO source_checkpoints (monitor_source_id, query_hash, next_poll_at) VALUES ($1, $2, $3)`, result.monitorSourceID, strings.Repeat("a", 64), nextPollAt); err != nil {
		t.Fatalf("seed %s checkpoint: %v", suffix, err)
	}
	return result
}
