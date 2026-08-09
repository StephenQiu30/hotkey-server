package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/infrastructure/postgres"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	platformscheduler "github.com/StephenQiu30/hotkey-server/backend/internal/platform/scheduler"
)

func TestPublishedCollectionTargetReaderUsesCallerTransactionForRetryCheckpoint(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	windowStart := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	future := windowEnd.Add(time.Hour)
	seeded := seedCollectionTarget(t, runtime.SQL, "transaction-retry", "active", true, true, true, false, future)
	reader := monitorpostgres.NewPublishedCollectionTargetReader(runtime)
	rollback := errors.New("rollback transaction-scoped checkpoint")
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE source_checkpoints SET next_poll_at = $1 WHERE monitor_source_id = $2`, windowStart, seeded.monitorSourceID); err != nil {
			return err
		}
		targets, err := reader.ListForCollection(ctx, seeded.sourceID, seeded.configID, strings.Repeat("a", 64), windowStart, windowEnd, sourcedomain.CollectionTriggerSchedule)
		if err != nil {
			t.Fatalf("ListForCollection() did not see caller transaction checkpoint: %v", err)
		}
		if len(targets) != 1 || targets[0].MonitorSourceID != seeded.monitorSourceID || !targets[0].Checkpoint.NextPollAt.Equal(windowStart) {
			t.Fatalf("transaction-scoped targets = %#v", targets)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("WithinTransaction() error = %v, want rollback sentinel", err)
	}
	var persisted time.Time
	if err := runtime.SQL.QueryRow(`SELECT next_poll_at FROM source_checkpoints WHERE monitor_source_id = $1`, seeded.monitorSourceID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if !persisted.Equal(future) {
		t.Fatalf("checkpoint after rollback = %s, want %s", persisted, future)
	}
}

func TestPublishedCollectionTargetReaderReturnsOnlyDueActivePublishedEnabledTargets(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)

	due := seedCollectionTarget(t, runtime.SQL, "due", "active", true, true, true, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "paused", "paused", true, true, true, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "disabled-association", "active", true, false, true, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "draft", "draft", false, true, true, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "future", "active", true, true, true, false, now.Add(time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "disabled-connection", "active", true, true, false, false, now.Add(-time.Minute))
	_ = seedCollectionTarget(t, runtime.SQL, "archived-connection", "active", true, true, true, true, now.Add(-time.Minute))

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

func TestPublishedCollectionTargetReaderPrefersExactCompiledIntentOverLegacyRules(t *testing.T) {
	runtime := intentRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	fixture := insertIntentRepositoryDraft(t, runtime, false)
	now := time.Date(2026, time.July, 17, 8, 0, 0, 0, time.UTC)
	var sourceID, monitorSourceID, revisionID, profileID, entityID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO source_connections (source_type,name,endpoint,auth_type,config,enabled,health_status)
VALUES ('rss','compiled intent source','https://feeds.example.test/compiled','none','{}'::jsonb,true,'unknown') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_sources (config_version_id,source_connection_id,query_signature,enabled)
VALUES ($1,$2,$3,true) RETURNING id`, fixture.configID, sourceID, strings.Repeat("e", 64)).Scan(&monitorSourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO source_checkpoints (monitor_source_id,query_hash,next_poll_at)
VALUES ($1,$2,$3)`, monitorSourceID, strings.Repeat("e", 64), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_rules (config_version_id,rule_type,operator,value,weight,approval_status,enabled)
VALUES ($1,'keyword','contains','legacy-only',100,'approved',true)`, fixture.configID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
SELECT id FROM monitor_intent_draft_revisions
WHERE draft_id=$1 AND resource_version=1`, fixture.draftID).Scan(&revisionID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_profiles (
  monitor_id,purpose,config_version_id,monitor_version_id,intent_revision_id,
  compiler_version,matching_algorithm_version,lexical_algorithm_version,semantic_algorithm_version,
  structured_algorithm_version,search_normalization_profile_version,semantic_state,semantic_unavailable_reason
) VALUES ($1,'published',$2,$2,$3,'monitor-intent-compiler-v1','rrf-k60-v1','fts-trgm-dice-v1',
          'halfvec-cosine-v1','entity-hard-rule-v1','canonical-nfc-plaintext-v1','unavailable','semantic_generation_unavailable')
RETURNING id`, fixture.monitorID, fixture.configID, revisionID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	for ordinal, clause := range []struct {
		operator, field, value, normalized string
	}{{"must", "action", "launch", "launch"}, {"must_not", "term", "noise", "noise"}} {
		if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_compiled_clauses (compiled_profile_id,ordinal,operator,field,value,normalized_value,origin)
VALUES ($1,$2,$3,$4,$5,$6,'intent_clause')`, profileID, ordinal, clause.operator, clause.field, clause.value, clause.normalized); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.SQL.QueryRow(`
INSERT INTO monitor_compiled_entities (compiled_profile_id,ordinal,canonical_id)
VALUES ($1,0,'product:hotkey') RETURNING id`, profileID).Scan(&entityID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
INSERT INTO monitor_compiled_entity_aliases (compiled_entity_id,compiled_profile_id,ordinal,alias,normalized_alias)
VALUES ($1,$2,0,'HotKey','hotkey')`, entityID, profileID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_config_versions SET state='published',config_hash=$2,published_at=$3 WHERE id=$1`, fixture.configID, strings.Repeat("f", 64), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitors SET status='active',draft_config_version_id=NULL,published_config_version_id=$2 WHERE id=$1`, fixture.monitorID, fixture.configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`
UPDATE monitor_compiled_profiles SET status='ready',profile_hash=$2,ready_at=$3 WHERE id=$1`, profileID, strings.Repeat("d", 64), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	targets, err := monitorpostgres.NewPublishedCollectionTargetReader(runtime).ListDue(context.Background(), now)
	if err != nil {
		t.Fatalf("ListDue(): %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	want := []sourcedomain.CollectionTerm{{Value: "launch"}, {Value: "noise", Excluded: true}, {Value: "HotKey"}}
	if len(targets[0].Terms) != len(want) {
		t.Fatalf("compiled terms = %#v, want %#v", targets[0].Terms, want)
	}
	for _, term := range targets[0].Terms {
		if term.Value == "legacy-only" {
			t.Fatalf("reader fell back to legacy monitor_rules: %#v", targets[0].Terms)
		}
	}
	seen := map[sourcedomain.CollectionTerm]bool{}
	for _, term := range targets[0].Terms {
		seen[term] = true
	}
	for _, term := range want {
		if !seen[term] {
			t.Fatalf("compiled terms = %#v, missing %#v", targets[0].Terms, term)
		}
	}
}

func TestCollectionSchedulerEnqueuesDueSourceWithoutWritingCollectionFacts(t *testing.T) {
	runtime := monitorRepositoryRuntime(t)
	defer func() { _ = runtime.Close() }()
	now := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	seedCollectionTarget(t, runtime.SQL, "scheduler", "active", true, true, true, false, now.Add(-5*time.Minute))
	reader := monitorpostgres.NewPublishedCollectionTargetReader(runtime)
	store := queue.NewStore(runtime)
	collectionScheduler := platformscheduler.NewCollectionScheduler(reader, store)
	first, err := collectionScheduler.RunOnce(context.Background(), now)
	if err != nil || first != 1 {
		t.Fatalf("first scheduler scan = %d/%v, want one new job", first, err)
	}
	second, err := collectionScheduler.RunOnce(context.Background(), now)
	if err != nil || second != 0 {
		t.Fatalf("second scheduler scan = %d/%v, want duplicate suppression", second, err)
	}
	var jobs, runs int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM river_job WHERE kind = 'collect_source'`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM collection_runs`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 || runs != 0 {
		t.Fatalf("scheduler facts = jobs=%d collection_runs=%d, want 1/0", jobs, runs)
	}
}

type seededCollectionTarget struct {
	sourceID, monitorSourceID, configID int64
}

func seedCollectionTarget(t *testing.T, runtime interface {
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}, suffix, monitorStatus string, published, monitorSourceEnabled, sourceConnectionEnabled, sourceConnectionDeleted bool, nextPollAt time.Time) seededCollectionTarget {
	t.Helper()
	var result seededCollectionTarget
	if err := runtime.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint, auth_type, config, enabled, health_status) VALUES ('rss', $1, 'https://feeds.example.test/collection', 'none', '{}'::jsonb, $2, 'unknown') RETURNING id`, "collection source "+suffix, sourceConnectionEnabled).Scan(&result.sourceID); err != nil {
		t.Fatalf("seed %s source: %v", suffix, err)
	}
	if sourceConnectionDeleted {
		if _, err := runtime.Exec(`UPDATE source_connections SET enabled = false, deleted_at = now() WHERE id = $1`, result.sourceID); err != nil {
			t.Fatalf("archive %s source: %v", suffix, err)
		}
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
	if err := runtime.QueryRow(`INSERT INTO monitor_sources (config_version_id, source_connection_id, query_signature, enabled) VALUES ($1, $2, $3, $4) RETURNING id`, result.configID, result.sourceID, strings.Repeat("a", 64), monitorSourceEnabled).Scan(&result.monitorSourceID); err != nil {
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
