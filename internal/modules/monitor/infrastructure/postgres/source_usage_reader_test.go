package postgres_test

import (
	"context"
	"strings"
	"testing"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func TestSourceUsageReaderFindsSoleActivePublishedSourceWithoutWrites(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	var sourceID, monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, config, enabled, health_status) VALUES ('rss', 'usage source', 'https://feeds.example.test/usage', 'none', '{}'::jsonb, true, 'unknown') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ('usage monitor', 'draft') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("seed monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision, state, timezone, languages, regions, collection_interval_seconds, relevance_threshold, event_threshold, retention_days) VALUES ($1, 1, 'draft', 'UTC', ARRAY['en'], ARRAY[]::text[], 300, 60, 0, 30) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("set draft pointer: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_sources (config_version_id, source_connection_id, enabled) VALUES ($1, $2, true)`, configID, sourceID); err != nil {
		t.Fatalf("seed monitor source: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("a", 64), configID); err != nil {
		t.Fatalf("publish config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'active', draft_config_version_id = NULL, published_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("publish monitor: %v", err)
	}
	reader := monitorpostgres.NewSourceUsageReader(runtime)
	var usage sourcedomain.SourceUsage
	if err := runtime.WithinTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		var err error
		usage, err = reader.UsageForSource(ctx, sourceID)
		return err
	}); err != nil {
		t.Fatalf("UsageForSource(): %v", err)
	}
	if !usage.ReferencedByActiveMonitor || usage.ActiveMonitorCount != 1 || !usage.SoleSchedulableForActive || usage.ReferencedByPausedMonitor {
		t.Fatalf("usage=%#v", usage)
	}
}
