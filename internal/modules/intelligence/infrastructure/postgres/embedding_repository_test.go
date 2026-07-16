package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/pgvector/pgvector-go"
)

func TestEmbeddingRepositoryAtomicallyReplacesEachTargetEmbedding(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	targets := seedEmbeddingTargets(t, runtime.SQL)
	for _, target := range targets {
		input := intelligencepostgres.EmbeddingWrite{
			Target: intelligencepostgres.EmbeddingTarget(target.kind), TargetID: target.id,
			ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
			InputHash: strings.Repeat("a", 64), Vector: embeddingVector(1), QueryText: "hotkey query",
		}
		if _, err := repository.ReplaceEmbedding(context.Background(), input); err != nil {
			t.Fatalf("ReplaceEmbedding(%s first) error = %v", target.kind, err)
		}
		input.InputHash = strings.Repeat("b", 64)
		input.Vector = embeddingVector(2)
		if _, err := repository.ReplaceEmbedding(context.Background(), input); err != nil {
			t.Fatalf("ReplaceEmbedding(%s second) error = %v", target.kind, err)
		}
		var total, active int
		query := "SELECT count(*),count(*) FILTER (WHERE active) FROM " + target.table + " WHERE " + target.column + "=$1 AND model_profile_id=$2"
		if err := runtime.SQL.QueryRow(query, target.id, profile.ID).Scan(&total, &active); err != nil {
			t.Fatalf("read %s replacement state: %v", target.kind, err)
		}
		if total != 2 || active != 1 {
			t.Fatalf("%s embeddings total/active = %d/%d, want 2/1", target.kind, total, active)
		}
	}
}

func TestEmbeddingRepositoryUsesFilteredHNSWForEveryTarget(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	profile.Name = "hnsw-filtered-profile"
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	targets := seedHNSWTargets(t, runtime.SQL, 512)
	for _, target := range targets {
		for index, targetID := range target.ids {
			input := intelligencepostgres.EmbeddingWrite{
				Target: intelligencepostgres.EmbeddingTarget(target.kind), TargetID: targetID,
				ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
				InputHash: fmt.Sprintf("%064x", index+1), Vector: embeddingVector(float32(index + 1)), QueryText: "hnsw query",
			}
			if _, err := repository.ReplaceEmbedding(context.Background(), input); err != nil {
				t.Fatalf("ReplaceEmbedding(%s/%d) error = %v", target.kind, index, err)
			}
		}
		matches, err := repository.NearestEmbeddings(context.Background(), intelligencepostgres.EmbeddingTarget(target.kind), profile.ID, profile.ModelVersion, embeddingVector(1), 5)
		if err != nil || len(matches) != 5 {
			t.Fatalf("NearestEmbeddings(%s) matches/error = %d / %v, want 5/nil", target.kind, len(matches), err)
		}
		assertFilteredHNSWPlan(t, runtime, embeddingTargetSeed{kind: target.kind, table: target.table, column: target.column}, profile)
	}
}

func TestEmbeddingRepositoryFiltersCurrentActiveModelAndUsesHNSW(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	targets := seedEmbeddingTargets(t, runtime.SQL)
	input := intelligencepostgres.EmbeddingWrite{
		Target: intelligencepostgres.EmbeddingTargetContent, TargetID: targets[0].id,
		ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion,
		InputHash: strings.Repeat("a", 64), Vector: embeddingVector(1),
	}
	if _, err := repository.ReplaceEmbedding(context.Background(), input); err != nil {
		t.Fatalf("ReplaceEmbedding() error = %v", err)
	}
	matches, err := repository.NearestEmbeddings(context.Background(), intelligencepostgres.EmbeddingTargetContent, profile.ID, profile.ModelVersion, embeddingVector(1), 5)
	if err != nil || len(matches) != 1 || matches[0].TargetID != targets[0].id {
		t.Fatalf("NearestEmbeddings(current model) = %#v / %v", matches, err)
	}
	stale, err := repository.NearestEmbeddings(context.Background(), intelligencepostgres.EmbeddingTargetContent, profile.ID, "stale-model", embeddingVector(1), 5)
	if err != nil || len(stale) != 0 {
		t.Fatalf("NearestEmbeddings(stale model) = %#v / %v, want no match", stale, err)
	}
	profile.Enabled = false
	if _, err := repository.UpdateProfile(context.Background(), profile, profile.Version); err != nil {
		t.Fatalf("UpdateProfile(disable) error = %v", err)
	}
	disabled, err := repository.NearestEmbeddings(context.Background(), intelligencepostgres.EmbeddingTargetContent, profile.ID, profile.ModelVersion, embeddingVector(1), 5)
	if err != nil || len(disabled) != 0 {
		t.Fatalf("NearestEmbeddings(disabled profile) = %#v / %v, want no match", disabled, err)
	}

}

func TestEmbeddingRepositoryRefusesWrongDimensionsAndNonFiniteValues(t *testing.T) {
	runtime := openIntelligenceRuntime(t)
	defer func() { _ = runtime.Close() }()
	repository := intelligencepostgres.NewRepository(runtime)
	profile := testEmbeddingProfile()
	if err := repository.CreateProfile(context.Background(), &profile); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	targets := seedEmbeddingTargets(t, runtime.SQL)
	input := intelligencepostgres.EmbeddingWrite{Target: intelligencepostgres.EmbeddingTargetContent, TargetID: targets[0].id, ModelProfileID: profile.ID, ModelProfileVersion: profile.Version, ModelVersion: profile.ModelVersion, InputHash: strings.Repeat("a", 64), Vector: make([]float32, intelligencedomain.EmbeddingDimensions-1)}
	if _, err := repository.ReplaceEmbedding(context.Background(), input); err == nil {
		t.Fatal("ReplaceEmbedding(1023 values) error = nil, want 70008")
	} else if code, ok := intelligencedomain.CodeOf(err); !ok || code != intelligencedomain.CodeAIEmbeddingInvalid {
		t.Fatalf("ReplaceEmbedding(1023 values) code = %d/%t, want 70008", code, ok)
	}
	input.Vector = embeddingVector(1)
	input.Vector[4] = float32(math.NaN())
	if _, err := repository.ReplaceEmbedding(context.Background(), input); err == nil {
		t.Fatal("ReplaceEmbedding(NaN) error = nil, want 70008")
	}
}

type embeddingTargetSeed struct {
	kind, table, column string
	id                  int64
}

type embeddingTargetBatch struct {
	kind, table, column string
	ids                 []int64
}

func seedEmbeddingTargets(t *testing.T, runtime interface{ QueryRow(string, ...any) *sql.Row }) []embeddingTargetSeed {
	t.Helper()
	var sourceID, contentID, monitorID, eventID, topicID int64
	if err := runtime.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','embedding-source','https://example.test/embedding') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := runtime.QueryRow(`INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key) VALUES ($1,'embedding-content','article','https://example.test/content',now(),now(),$2) RETURNING id`, sourceID, strings.Repeat("c", 64)).Scan(&contentID); err != nil {
		t.Fatalf("seed content: %v", err)
	}
	if err := runtime.QueryRow(`INSERT INTO monitors (name) VALUES ('embedding-monitor') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("seed monitor: %v", err)
	}
	if err := runtime.QueryRow(`INSERT INTO events (event_key,title_zh,lifecycle_status,first_seen_at,last_seen_at) VALUES ('embedding-event','Embedding event','detected',now(),now()) RETURNING id`).Scan(&eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := runtime.QueryRow(`INSERT INTO topics (topic_key,title) VALUES ('embedding-topic','Embedding topic') RETURNING id`).Scan(&topicID); err != nil {
		t.Fatalf("seed topic: %v", err)
	}
	return []embeddingTargetSeed{{"content", "content_embeddings", "content_id", contentID}, {"monitor", "monitor_embeddings", "monitor_id", monitorID}, {"event", "event_embeddings", "event_id", eventID}, {"topic", "topic_embeddings", "topic_id", topicID}}
}

func seedHNSWTargets(t *testing.T, runtime interface{ QueryRow(string, ...any) *sql.Row }, count int) []embeddingTargetBatch {
	t.Helper()
	var sourceID int64
	if err := runtime.QueryRow(`INSERT INTO source_connections (source_type,name,endpoint) VALUES ('rss','hnsw-source','https://example.test/hnsw') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("seed HNSW source: %v", err)
	}
	batches := []embeddingTargetBatch{
		{kind: "content", table: "content_embeddings", column: "content_id", ids: make([]int64, 0, count)},
		{kind: "monitor", table: "monitor_embeddings", column: "monitor_id", ids: make([]int64, 0, count)},
		{kind: "event", table: "event_embeddings", column: "event_id", ids: make([]int64, 0, count)},
		{kind: "topic", table: "topic_embeddings", column: "topic_id", ids: make([]int64, 0, count)},
	}
	for index := 0; index < count; index++ {
		var contentID, monitorID, eventID, topicID int64
		suffix := fmt.Sprintf("%03d", index)
		if err := runtime.QueryRow(`INSERT INTO contents (source_connection_id,external_id,content_type,canonical_url,published_at,fetched_at,dedupe_key) VALUES ($1,$2,'article',$3,now(),now(),$4) RETURNING id`, sourceID, "hnsw-content-"+suffix, "https://example.test/hnsw/content/"+suffix, fmt.Sprintf("%064x", index+1000)).Scan(&contentID); err != nil {
			t.Fatalf("seed HNSW content %d: %v", index, err)
		}
		if err := runtime.QueryRow(`INSERT INTO monitors (name) VALUES ($1) RETURNING id`, "hnsw-monitor-"+suffix).Scan(&monitorID); err != nil {
			t.Fatalf("seed HNSW monitor %d: %v", index, err)
		}
		if err := runtime.QueryRow(`INSERT INTO events (event_key,title_zh,lifecycle_status,first_seen_at,last_seen_at) VALUES ($1,$2,'detected',now(),now()) RETURNING id`, "hnsw-event-"+suffix, "HNSW event "+suffix).Scan(&eventID); err != nil {
			t.Fatalf("seed HNSW event %d: %v", index, err)
		}
		if err := runtime.QueryRow(`INSERT INTO topics (topic_key,title) VALUES ($1,$2) RETURNING id`, "hnsw-topic-"+suffix, "HNSW topic "+suffix).Scan(&topicID); err != nil {
			t.Fatalf("seed HNSW topic %d: %v", index, err)
		}
		batches[0].ids = append(batches[0].ids, contentID)
		batches[1].ids = append(batches[1].ids, monitorID)
		batches[2].ids = append(batches[2].ids, eventID)
		batches[3].ids = append(batches[3].ids, topicID)
	}
	return batches
}

func assertFilteredHNSWPlan(t *testing.T, runtime *database.Runtime, target embeddingTargetSeed, profile intelligencedomain.ModelProfile) {
	t.Helper()
	var plan []string
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		if _, err := transaction.SQL.ExecContext(ctx, `ANALYZE `+target.table); err != nil {
			return fmt.Errorf("analyze %s embeddings: %w", target.kind, err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return fmt.Errorf("set local enable_seqscan: %w", err)
		}
		rows, err := transaction.SQL.QueryContext(ctx, `
EXPLAIN (COSTS OFF)
SELECT e.`+target.column+`
FROM `+target.table+` e
JOIN ai_model_profiles p ON p.id=e.model_profile_id
WHERE e.active AND e.model_profile_id=$2 AND e.model_version=$3
  AND p.enabled AND p.deleted_at IS NULL
ORDER BY e.embedding <=> $1::halfvec
LIMIT 5`, pgvector.NewVector(embeddingVector(1)), profile.ID, profile.ModelVersion)
		if err != nil {
			return fmt.Errorf("EXPLAIN %s HNSW: %w", target.kind, err)
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return fmt.Errorf("scan %s HNSW plan: %w", target.kind, err)
			}
			plan = append(plan, line)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate %s HNSW plan: %w", target.kind, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	index := target.table + "_active_hnsw_idx"
	if !strings.Contains(strings.Join(plan, "\n"), index) {
		t.Fatalf("%s HNSW plan lacks %s:\n%s", target.kind, index, strings.Join(plan, "\n"))
	}
}

func embeddingVector(value float32) []float32 {
	vector := make([]float32, intelligencedomain.EmbeddingDimensions)
	vector[0] = value
	return vector
}
