package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

func TestPublishedReferenceReaderRequiresCallerTransactionAndIncludesHistoricalStates(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	ctx := context.Background()
	reader := monitorpostgres.NewPublishedReferenceReader(runtime)

	if _, err := reader.HasPublishedReference(ctx, 1); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("HasPublishedReference(outside transaction) error = %v, want unavailable", err)
	}

	var publishedSourceID, supersededSourceID, draftSourceID int64
	for _, target := range []*int64{&publishedSourceID, &supersededSourceID, &draftSourceID} {
		if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, config, enabled, health_status) VALUES ('rss', 'reference source ' || nextval('source_connections_id_seq'), 'https://feeds.example.test/reference', 'none', '{}'::jsonb, true, 'unknown') RETURNING id`).Scan(target); err != nil {
			t.Fatalf("seed source: %v", err)
		}
	}
	states := []struct {
		name     string
		state    string
		sourceID int64
		want     bool
	}{
		{name: "published", state: "published", sourceID: publishedSourceID, want: true},
		{name: "superseded", state: "superseded", sourceID: supersededSourceID, want: true},
		{name: "draft", state: "draft", sourceID: draftSourceID, want: false},
	}
	for _, state := range states {
		var monitorID, configID int64
		if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "reference "+state.name).Scan(&monitorID); err != nil {
			t.Fatalf("seed %s monitor: %v", state.name, err)
		}
		if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
			t.Fatalf("seed %s config: %v", state.name, err)
		}
		if _, err := runtime.SQL.Exec(`INSERT INTO monitor_sources (config_version_id, source_connection_id) VALUES ($1, $2)`, configID, state.sourceID); err != nil {
			t.Fatalf("seed %s association: %v", state.name, err)
		}
		if state.state != "draft" {
			if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = $1, published_at = now() WHERE id = $2`, strings.Repeat("c", 64), configID); err != nil {
				t.Fatalf("publish %s config: %v", state.name, err)
			}
			if state.state == "superseded" {
				if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'superseded' WHERE id = $1`, configID); err != nil {
					t.Fatalf("supersede config: %v", err)
				}
			}
		}
	}

	if err := runtime.WithinTransaction(ctx, func(ctx context.Context, _ database.Transaction) error {
		for _, state := range states {
			referenced, err := reader.HasPublishedReference(ctx, state.sourceID)
			if err != nil {
				return err
			}
			if referenced != state.want {
				t.Fatalf("HasPublishedReference(%s) = %v, want %v", state.name, referenced, state.want)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("transactional published reference reads: %v", err)
	}
}
