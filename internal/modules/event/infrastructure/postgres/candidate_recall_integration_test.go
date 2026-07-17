//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
	"github.com/pgvector/pgvector-go"
)

type candidateRecallFixture struct {
	contentID, sourceID int64
	eventIDs            []int64
}

func TestCandidateRecallRepositoryBoundsChannelsAndDegradesWithoutVectors(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedCandidateRecallFixture(t, runtime, 20)
	repository := NewRepository(runtime)

	lexical, err := repository.Lexical(ctx, fixture.contentID, application.LexicalLimit)
	if err != nil || len(lexical) != application.LexicalLimit {
		t.Fatalf("Lexical() = %d/%v, want %d/nil", len(lexical), err, application.LexicalLimit)
	}
	temporal, err := repository.Temporal(ctx, fixture.contentID, application.TemporalLimit)
	if err != nil || len(temporal) != application.TemporalLimit {
		t.Fatalf("Temporal() = %d/%v, want %d/nil", len(temporal), err, application.TemporalLimit)
	}
	fingerprint, err := repository.Fingerprint(ctx, fixture.contentID, application.FingerprintLimit)
	if err != nil || len(fingerprint) != application.FingerprintLimit {
		t.Fatalf("Fingerprint() = %d/%v, want %d/nil", len(fingerprint), err, application.FingerprintLimit)
	}
	if _, err := repository.Vector(ctx, fixture.contentID, application.VectorLimit); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Vector() error = %v, want unavailable when no matching embedding space exists", err)
	}
	recalled, err := application.NewRecallService(repository).Recall(ctx, application.RecallInput{ContentID: fixture.contentID})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !recalled.VectorUnavailable || len(recalled.Candidates) != application.LexicalLimit {
		t.Fatalf("Recall() = %#v, want bounded lexical union with vector downgrade", recalled)
	}
}

func TestCandidateRecallRepositoryUsesHNSWVectorPath(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedCandidateRecallFixture(t, runtime, 20)
	profileID, profileVersion := createCandidateRecallEmbeddingProfile(t, runtime)
	seedCandidateRecallEmbedding(t, runtime, profileID, profileVersion, "content", fixture.contentID, 1)
	for index, eventID := range fixture.eventIDs {
		seedCandidateRecallEmbedding(t, runtime, profileID, profileVersion, "event", eventID, float32(index+1))
	}
	repository := NewRepository(runtime)
	candidates, err := repository.Vector(ctx, fixture.contentID, application.VectorLimit)
	if err != nil || len(candidates) != application.VectorLimit {
		t.Fatalf("Vector() = %d/%v, want %d/nil", len(candidates), err, application.VectorLimit)
	}
	assertCandidateRecallVectorHNSW(t, runtime, fixture.contentID)
}

func TestClusteringExecutionPersistsRecalledCandidateProvenance(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedCandidateRecallFixture(t, runtime, 1)
	evidenceContentID := seedCandidateRepresentativeEvidence(t, runtime, fixture.sourceID)
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, is_representative, origin) VALUES ($1,$2,95,'primary',true,'rule')`, fixture.eventIDs[0], evidenceContentID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET representative_content_id = $1 WHERE id = $2`, evidenceContentID, fixture.eventIDs[0]); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	target, err := repository.Get(ctx, fixture.eventIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewClusteringExecutionService(application.NewRecallService(repository), application.NewClusteringService(), repository)
	result, err := service.Execute(ctx, application.ClusteringExecutionInput{
		ContentID: fixture.contentID, ClusteringVersion: "v1", FeatureInputHash: domain.FeatureInputHash("candidate-recall-provenance"),
		Scores: map[string]domain.ScoreBreakdown{
			target.EventKey: {EntityAction: 95, Semantic: 90, Temporal: 90, Location: 80, SourceContext: 80},
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Event == nil || result.Event.ID != target.ID || result.Created || !result.VectorUnavailable {
		t.Fatalf("Execute() = %#v", result)
	}
	var channelCount, reasonCount, evidenceCount int
	if err := runtime.SQL.QueryRow(`SELECT jsonb_array_length(feature_snapshot->'recall_channels'), cardinality(reason_codes), cardinality(evidence_content_ids) FROM event_clustering_decisions WHERE content_id = $1 AND candidate_event_id = $2`, fixture.contentID, target.ID).Scan(&channelCount, &reasonCount, &evidenceCount); err != nil {
		t.Fatal(err)
	}
	if channelCount != 3 || reasonCount != 4 || evidenceCount != 2 {
		t.Fatalf("persisted recalled provenance = channels=%d reasons=%d evidence=%d", channelCount, reasonCount, evidenceCount)
	}
}

func seedCandidateRecallFixture(t *testing.T, runtime *database.Runtime, eventCount int) candidateRecallFixture {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var fixture candidateRecallFixture
	var monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'event-recall-' || md5(random()::text), 'https://event.example') RETURNING id`).Scan(&fixture.sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, excerpt, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1, 'candidate-recall', 'article', 'Shared event', 'shared event evidence', 'https://event.example/candidate-recall', $2, $2, repeat('d',64)) RETURNING id`, fixture.sourceID, now).Scan(&fixture.contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitors (name, status) VALUES ('event-recall-' || md5(random()::text), 'draft') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatalf("insert monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET draft_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("set monitor draft config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitor_config_versions SET state = 'published', config_hash = repeat('a',64), published_at = now() WHERE id = $1`, configID); err != nil {
		t.Fatalf("publish monitor config: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE monitors SET status = 'active', draft_config_version_id = NULL, published_config_version_id = $1 WHERE id = $2`, configID, monitorID); err != nil {
		t.Fatalf("activate monitor: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_matches (monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, algorithm_version, input_hash, scoring_version) VALUES ($1,$2,$3,90,90,'accepted','event-recall-v1',repeat('b',64),'event-recall-v1')`, monitorID, configID, fixture.contentID); err != nil {
		t.Fatalf("insert accepted match: %v", err)
	}
	for index := 0; index < eventCount; index++ {
		var eventID int64
		seenAt := now.Add(-time.Duration(index) * time.Hour)
		if err := runtime.SQL.QueryRow(`INSERT INTO events (event_key, event_fingerprint, fingerprint_version, title_zh, title_en, summary, lifecycle_status, first_seen_at, last_seen_at) VALUES ($1,repeat('d',64),'content_dedupe_v1','共享事件','Shared event','shared event evidence','active',$2,$2) RETURNING id`, fmt.Sprintf("evt-recall-%02d", index), seenAt).Scan(&eventID); err != nil {
			t.Fatalf("insert event %d: %v", index, err)
		}
		if _, err := runtime.SQL.Exec(`INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at) VALUES ($1,$2,90,90,$3,$3)`, monitorID, eventID, seenAt); err != nil {
			t.Fatalf("insert monitor event %d: %v", index, err)
		}
		fixture.eventIDs = append(fixture.eventIDs, eventID)
	}
	return fixture
}

func seedCandidateRepresentativeEvidence(t *testing.T, runtime *database.Runtime, sourceID int64) int64 {
	t.Helper()
	var contentID int64
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, excerpt, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1, 'candidate-evidence-' || md5(random()::text), 'article', 'Shared event evidence', 'candidate representative evidence', 'https://event.example/candidate-evidence', $2, $2, md5(random()::text) || md5(random()::text)) RETURNING id`, sourceID, now).Scan(&contentID); err != nil {
		t.Fatalf("insert candidate representative evidence: %v", err)
	}
	return contentID
}

func createCandidateRecallEmbeddingProfile(t *testing.T, runtime *database.Runtime) (int64, int64) {
	t.Helper()
	var profileID, profileVersion int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_model_profiles (
 name,task_type,provider,model_name,credential_ref,model_version,embedding_dimensions,
 timeout_seconds,max_attempts,max_cost,fallback_priority,enabled
) VALUES ('event-recall-' || md5(random()::text),'embedding','openai','text-embedding-3-large','env:OPENAI_API_KEY','event-recall-v1',1024,30,1,0.1000,100,true)
RETURNING id,version`).Scan(&profileID, &profileVersion); err != nil {
		t.Fatalf("create embedding profile: %v", err)
	}
	return profileID, profileVersion
}

func seedCandidateRecallEmbedding(t *testing.T, runtime *database.Runtime, profileID, profileVersion int64, target string, targetID int64, value float32) {
	t.Helper()
	offset := int64(1_000_000)
	if target == "event" {
		offset = 2_000_000
	}
	inputHash := fmt.Sprintf("%064x", targetID+offset)
	reuseKey := fmt.Sprintf("%064x", targetID+offset+3_000_000)
	var runID int64
	if err := runtime.SQL.QueryRow(`
INSERT INTO ai_runs (
 task_type,target_type,target_id,model_profile_id,prompt_version,schema_version,input_hash,status,
 model_profile_version,model_version,parameters_version,input_schema_version,evidence_set_hash,reuse_key,
 attempt,max_attempts,budget_day,cost
) VALUES ('embedding',$1,$2,$3,'fixture','v1',$4,'succeeded',$5,'event-recall-v1','fixture','v1',repeat('c',64),$6,1,1,current_date,1)
RETURNING id`, target, targetID, profileID, inputHash, profileVersion, reuseKey).Scan(&runID); err != nil {
		t.Fatalf("create embedding run: %v", err)
	}
	vector := make([]float32, 1024)
	vector[0] = value
	if target == "content" {
		if _, err := runtime.SQL.Exec(`INSERT INTO content_embeddings (content_id,model_profile_id,model_version,input_hash,embedding,model_profile_version,ai_run_id) VALUES ($1,$2,'event-recall-v1',$3,$4,$5,$6)`, targetID, profileID, inputHash, pgvector.NewHalfVector(vector), profileVersion, runID); err != nil {
			t.Fatalf("insert content embedding: %v", err)
		}
		return
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_embeddings (event_id,model_profile_id,model_version,input_hash,embedding,model_profile_version,ai_run_id) VALUES ($1,$2,'event-recall-v1',$3,$4,$5,$6)`, targetID, profileID, inputHash, pgvector.NewHalfVector(vector), profileVersion, runID); err != nil {
		t.Fatalf("insert event embedding: %v", err)
	}
}

func assertCandidateRecallVectorHNSW(t *testing.T, runtime *database.Runtime, contentID int64) {
	t.Helper()
	var vector pgvector.HalfVector
	var profileID, profileVersion int64
	var modelVersion string
	if err := runtime.SQL.QueryRow(`SELECT embedding, model_profile_id, model_profile_version, model_version FROM content_embeddings WHERE content_id = $1 AND active`, contentID).Scan(&vector, &profileID, &profileVersion, &modelVersion); err != nil {
		t.Fatalf("read query embedding: %v", err)
	}
	var lines []string
	err := runtime.WithinTransaction(context.Background(), func(ctx context.Context, transaction database.Transaction) error {
		for _, statement := range []string{`ANALYZE content_embeddings`, `ANALYZE event_embeddings`, `SET LOCAL enable_seqscan = off`, `SET LOCAL enable_bitmapscan = off`, `SET LOCAL enable_sort = off`} {
			if _, err := transaction.SQL.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		rows, err := transaction.SQL.QueryContext(ctx, `
EXPLAIN (COSTS OFF)
SELECT e.id, e.event_key, 'vector', (100 - LEAST(100, $1::halfvec <=> ee.embedding) * 100), e.representative_content_id
FROM event_embeddings ee
JOIN ai_model_profiles p ON p.id = ee.model_profile_id
JOIN events e ON e.id = ee.event_id
WHERE ee.active AND ee.model_profile_id = $2 AND ee.model_profile_version = $3 AND ee.model_version = $4
  AND p.enabled AND p.deleted_at IS NULL AND p.version = ee.model_profile_version
  AND e.lifecycle_status IN ('detected','active','cooling','closed') AND e.deleted_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM monitor_events me
    JOIN monitor_matches mm ON mm.monitor_id = me.monitor_id AND mm.content_id = $5 AND mm.decision = 'accepted'
    WHERE me.event_id = ee.event_id
  )
ORDER BY ee.embedding <=> $1::halfvec
LIMIT 12`, vector, profileID, profileVersion, modelVersion, contentID)
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
	if plan := strings.Join(lines, "\n"); !strings.Contains(plan, "event_embeddings_active_hnsw_idx") || strings.Contains(plan, "Seq Scan on event_embeddings") {
		t.Fatalf("vector recall plan must use active HNSW without an event embedding full scan:\n%s", plan)
	}
}
