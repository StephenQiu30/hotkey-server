//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

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
	contentID := seedCandidateRecallFixture(t, runtime, 20)
	repository := NewRepository(runtime)

	lexical, err := repository.Lexical(ctx, contentID, application.LexicalLimit)
	if err != nil || len(lexical) != application.LexicalLimit {
		t.Fatalf("Lexical() = %d/%v, want %d/nil", len(lexical), err, application.LexicalLimit)
	}
	temporal, err := repository.Temporal(ctx, contentID, application.TemporalLimit)
	if err != nil || len(temporal) != application.TemporalLimit {
		t.Fatalf("Temporal() = %d/%v, want %d/nil", len(temporal), err, application.TemporalLimit)
	}
	fingerprint, err := repository.Fingerprint(ctx, contentID, application.FingerprintLimit)
	if err != nil || len(fingerprint) != application.FingerprintLimit {
		t.Fatalf("Fingerprint() = %d/%v, want %d/nil", len(fingerprint), err, application.FingerprintLimit)
	}
	if _, err := repository.Vector(ctx, contentID, application.VectorLimit); !errors.Is(err, sharedrepository.ErrUnavailable) {
		t.Fatalf("Vector() error = %v, want unavailable when no matching embedding space exists", err)
	}
	recalled, err := application.NewRecallService(repository).Recall(ctx, application.RecallInput{ContentID: contentID})
	if err != nil {
		t.Fatalf("Recall() error = %v", err)
	}
	if !recalled.VectorUnavailable || len(recalled.Candidates) != application.LexicalLimit {
		t.Fatalf("Recall() = %#v, want bounded lexical union with vector downgrade", recalled)
	}
}

func seedCandidateRecallFixture(t *testing.T, runtime *database.Runtime, eventCount int) int64 {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var sourceID, contentID, monitorID, configID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'event-recall-' || md5(random()::text), 'https://event.example') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, excerpt, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1, 'candidate-recall', 'article', 'Shared event', 'shared event evidence', 'https://event.example/candidate-recall', $2, $2, repeat('d',64)) RETURNING id`, sourceID, now).Scan(&contentID); err != nil {
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
	if _, err := runtime.SQL.Exec(`INSERT INTO monitor_matches (monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, algorithm_version, input_hash, scoring_version) VALUES ($1,$2,$3,90,90,'accepted','event-recall-v1',repeat('b',64),'event-recall-v1')`, monitorID, configID, contentID); err != nil {
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
	}
	return contentID
}
