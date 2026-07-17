//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	ingestionpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/infrastructure/postgres"
	intelligenceapplication "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/application"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/pgvector/pgvector-go"
)

func TestPlan009RelevanceCandidateQueryUsesFilteredHNSW(t *testing.T) {
	runtime, fixture := openRelevanceRuntime(t)
	defer func() { _ = runtime.Close() }()
	if _, err := runtime.SQL.Exec(`UPDATE contents SET title='OpenAI platform release', excerpt='bounded lexical fixture' WHERE id=$1`, fixture.contentID); err != nil {
		t.Fatalf("seed relevance content: %v", err)
	}
	monitorIDs := make([]int64, 0, 12)
	for index := 0; index < 12; index++ {
		monitorIDs = append(monitorIDs, createPlan009CandidateMonitor(t, runtime, fixture.sourceID, index))
	}
	// The default planner regression fixture deliberately gives one source a
	// large active Monitor population. The production query must still start at
	// monitor_sources and probe configs/Monitors by primary key rather than scan
	// either whole control-plane table to return eight candidates.
	for index := 12; index < 412; index++ {
		_ = createPlan009CandidateMonitor(t, runtime, fixture.sourceID, index)
	}
	reader := ingestionpostgres.NewRelevanceCandidateReader(runtime)
	source, err := reader.SourceCandidates(context.Background(), fixture.sourceID, 8)
	if err != nil || len(source) != 8 {
		t.Fatalf("SourceCandidates() = %d / %v, want 8/nil", len(source), err)
	}
	lexical, err := reader.LexicalCandidates(context.Background(), []string{"openai"}, 12)
	if err != nil || len(lexical) != 12 {
		t.Fatalf("LexicalCandidates() = %d / %v, want 12/nil", len(lexical), err)
	}
	loaded, err := reader.LoadRelevanceCandidates(context.Background(), monitorIDs)
	if err != nil || len(loaded) != 12 {
		t.Fatalf("LoadRelevanceCandidates() = %d / %v, want 12/nil", len(loaded), err)
	}
	for _, candidate := range loaded {
		if candidate.MonitorID <= 0 || candidate.MonitorConfigVersionID <= 0 || len(candidate.Rules) != 1 || candidate.Rules[0].Value != "OpenAI" {
			t.Fatalf("loaded candidate = %#v, want published approved rule", candidate)
		}
	}
	assertPlan009BoundedCandidateIndexes(t, runtime, fixture.sourceID)

	profileID, profileVersion := createPlan009EmbeddingProfile(t, runtime)
	seedPlan009Embedding(t, runtime, profileID, profileVersion, "content", fixture.contentID, 1)
	for index, monitorID := range monitorIDs {
		seedPlan009Embedding(t, runtime, profileID, profileVersion, "monitor", monitorID, float32(index+1))
	}
	pausedMonitorID := createPlan009CandidateMonitor(t, runtime, fixture.sourceID, 99)
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='paused' WHERE id=$1`, pausedMonitorID); err != nil {
		t.Fatalf("pause vector-only monitor: %v", err)
	}
	seedPlan009Embedding(t, runtime, profileID, profileVersion, "monitor", pausedMonitorID, 0)
	query, err := intelligenceapplication.NewEmbeddingQueryService(intelligencepostgres.NewRepository(runtime))
	if err != nil {
		t.Fatalf("NewEmbeddingQueryService() error = %v", err)
	}
	space := intelligenceapplication.EmbeddingSpace{ModelProfileID: profileID, ModelProfileVersion: profileVersion, ModelVersion: "plan009-embedding-v1"}
	active, found, err := query.ActiveContent(context.Background(), fixture.contentID, space)
	if err != nil || !found {
		t.Fatalf("ActiveContent() = %#v/%t/%v, want exact active vector", active, found, err)
	}
	neighbors, err := query.NearestMonitors(context.Background(), active, 12)
	if err != nil || len(neighbors) != 12 {
		t.Fatalf("NearestMonitors() = %d / %v, want 12/nil", len(neighbors), err)
	}
	for _, neighbor := range neighbors {
		if neighbor.TargetID == pausedMonitorID {
			t.Fatalf("NearestMonitors() returned paused monitor %d", pausedMonitorID)
		}
	}
	assertPlan009FilteredHNSW(t, runtime, profileID, profileVersion)
}

func createPlan009CandidateMonitor(t *testing.T, runtime *database.Runtime, sourceID int64, index int) int64 {
	t.Helper()
	var monitorID, configID int64
	name := fmt.Sprintf("plan009-candidate-%d-%d", index, fixtureTimestamp(t))
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ($1, 'draft') RETURNING id`, name).Scan(&monitorID); err != nil {
		t.Fatalf("create candidate monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("create candidate config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id=$1 WHERE id=$2`, configID, monitorID); err != nil {
		t.Fatalf("attach candidate draft: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_sources (config_version_id, source_connection_id, enabled) VALUES ($1,$2,true)`, configID, sourceID); err != nil {
		t.Fatalf("create candidate source: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, weight, origin, approval_status, enabled) VALUES ($1,'keyword','contains','OpenAI',100,'user','approved',true)`, configID); err != nil {
		t.Fatalf("create candidate rule: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state='published',config_hash=$1,published_at=now() WHERE id=$2`, fmt.Sprintf("%064x", index+1), configID); err != nil {
		t.Fatalf("publish candidate config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status='active',draft_config_version_id=NULL,published_config_version_id=$1 WHERE id=$2`, configID, monitorID); err != nil {
		t.Fatalf("activate candidate monitor: %v", err)
	}
	return monitorID
}

func fixtureTimestamp(t *testing.T) int64 {
	t.Helper()
	return time.Now().UnixNano()
}

func createPlan009EmbeddingProfile(t *testing.T, runtime *database.Runtime) (int64, int64) {
	t.Helper()
	var id, version int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_model_profiles (
 name,task_type,provider,model_name,credential_ref,model_version,embedding_dimensions,
 timeout_seconds,max_attempts,max_cost,fallback_priority,enabled
) VALUES ($1,'embedding','openai','text-embedding-3-large','env:OPENAI_API_KEY','plan009-embedding-v1',1024,30,1,0.1000,100,true)
RETURNING id,version`, fmt.Sprintf("plan009-embedding-%d", fixtureTimestamp(t))).Scan(&id, &version); err != nil {
		t.Fatalf("create embedding profile: %v", err)
	}
	return id, version
}

func seedPlan009Embedding(t *testing.T, runtime *database.Runtime, profileID, profileVersion int64, target string, targetID int64, value float32) {
	t.Helper()
	seed := targetID + 1_000_000
	if target == "monitor" {
		seed = targetID + 2_000_000
	}
	inputHash := fmt.Sprintf("%064x", seed)
	reuseKey := fmt.Sprintf("%064x", seed+3_000_000)
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_runs (
 task_type,target_type,target_id,model_profile_id,prompt_version,schema_version,input_hash,status,
 model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
 attempt,max_attempts,budget_day,cost
) VALUES ('embedding',$1,$2,$3,'fixture','v1',$4,'succeeded',$5,'plan009-embedding-v1','fixture','v1',$6,$7,1,1,current_date,1)
RETURNING id`, target, targetID, profileID, inputHash, profileVersion, strings.Repeat("d", 64), reuseKey).Scan(&runID); err != nil {
		t.Fatalf("create embedding run: %v", err)
	}
	vector := make([]float32, 1024)
	vector[0] = value
	if target == "content" {
		if _, err := runtime.SQL.Exec(`INSERT INTO content_embeddings (content_id,model_profile_id,model_version,input_hash,embedding,model_profile_version,ai_run_id) VALUES ($1,$2,'plan009-embedding-v1',$3,$4,$5,$6)`, targetID, profileID, inputHash, pgvector.NewHalfVector(vector), profileVersion, runID); err != nil {
			t.Fatalf("seed content embedding: %v", err)
		}
		return
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_embeddings (monitor_id,model_profile_id,model_version,input_hash,query_text,embedding,model_profile_version,ai_run_id) VALUES ($1,$2,'plan009-embedding-v1',$3,'plan009 query',$4,$5,$6)`, targetID, profileID, inputHash, pgvector.NewHalfVector(vector), profileVersion, runID); err != nil {
		t.Fatalf("seed monitor embedding: %v", err)
	}
}

func assertPlan009FilteredHNSW(t *testing.T, runtime *database.Runtime, profileID, profileVersion int64) {
	t.Helper()
	var lines []string
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		for _, statement := range []string{`ANALYZE monitor_embeddings`, `SET LOCAL enable_seqscan = off`, `SET LOCAL enable_bitmapscan = off`, `SET LOCAL enable_sort = off`} {
			if _, err := transaction.SQL.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		vector := make([]float32, 1024)
		vector[0] = 1
		rows, err := transaction.SQL.QueryContext(ctx, `
EXPLAIN (COSTS OFF)
SELECT e.monitor_id
FROM monitor_embeddings e
JOIN ai_model_profiles p ON p.id=e.model_profile_id
JOIN monitors m ON m.id=e.monitor_id
JOIN monitor_config_versions c ON c.id=m.published_config_version_id
WHERE e.active AND e.model_profile_id=$2 AND e.model_profile_version=$3 AND e.model_version=$4
  AND p.enabled AND p.deleted_at IS NULL AND p.version=$3
  AND m.status='active' AND m.deleted_at IS NULL AND c.state='published'
ORDER BY e.embedding <=> $1::halfvec
LIMIT 12`, pgvector.NewVector(vector), profileID, profileVersion, "plan009-embedding-v1")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			lines = append(lines, line)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan := strings.Join(lines, "\n"); !strings.Contains(plan, "monitor_embeddings_active_hnsw_idx") {
		t.Fatalf("filtered HNSW plan missing active index:\n%s", plan)
	}
}

func assertPlan009BoundedCandidateIndexes(t *testing.T, runtime *database.Runtime, sourceID int64) {
	t.Helper()
	var sourcePlan, lexicalPlan []string
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		for _, statement := range []string{`ANALYZE monitor_sources`, `ANALYZE monitor_rules`, `ANALYZE monitors`, `ANALYZE monitor_config_versions`} {
			if _, err := transaction.SQL.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		queries := []struct {
			plan  *[]string
			query string
			args  []any
		}{
			{&sourcePlan, `
EXPLAIN (COSTS OFF)
WITH source_candidates AS MATERIALIZED (
    SELECT source.config_version_id, source.priority
    FROM monitor_sources AS source
    WHERE source.source_connection_id=$1 AND source.enabled
    ORDER BY source.priority ASC, source.config_version_id ASC
), eligible AS MATERIALIZED (
    SELECT source.priority, (
        SELECT monitor.id
        FROM monitors AS monitor
        WHERE monitor.id=(
            SELECT config.monitor_id
            FROM monitor_config_versions AS config
            WHERE config.id=source.config_version_id AND config.state='published'
        )
          AND monitor.status='active' AND monitor.deleted_at IS NULL
          AND monitor.published_config_version_id=source.config_version_id
    ) AS monitor_id
    FROM source_candidates AS source
)
SELECT monitor_id
FROM eligible
WHERE monitor_id IS NOT NULL
ORDER BY priority ASC, monitor_id ASC
LIMIT 8`, []any{sourceID}},
			{&lexicalPlan, `
EXPLAIN (COSTS OFF)
SELECT monitor.id
FROM monitor_rules AS rule
JOIN monitor_config_versions AS config ON config.id=rule.config_version_id
JOIN monitors AS monitor ON monitor.id=config.monitor_id
WHERE rule.enabled AND rule.approval_status='approved'
  AND rule.rule_type IN ('keyword','phrase','entity','exclude_keyword')
  AND lower(rule.value)=ANY($1::text[])
  AND monitor.status='active' AND monitor.deleted_at IS NULL
  AND monitor.published_config_version_id=config.id AND config.state='published'
ORDER BY rule.weight DESC, monitor.id ASC
LIMIT 12`, []any{[]string{"openai"}}},
		}
		for _, item := range queries {
			rows, err := transaction.SQL.QueryContext(ctx, item.query, item.args...)
			if err != nil {
				return err
			}
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					_ = rows.Close()
					return err
				}
				*item.plan = append(*item.plan, line)
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan := strings.Join(sourcePlan, "\n"); strings.Contains(plan, "Seq Scan on monitors") || strings.Contains(plan, "Seq Scan on monitor_config_versions") {
		t.Fatalf("bounded source candidate plan scans monitor control-plane tables:\n%s", plan)
	}
	if plan := strings.Join(lexicalPlan, "\n"); !strings.Contains(plan, "monitor_rules_relevance_approved_lexical_idx") {
		t.Fatalf("bounded lexical candidate plan lacks inverted index:\n%s", plan)
	}
}
