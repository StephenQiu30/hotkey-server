//go:build integration

package postgres

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestApplyClusteringCreatesExactlyOneEventAcrossRetries(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	contentID := seedUnassignedEventContent(t, runtime)
	decisions := []domain.Decision{{ContentID: contentID, CandidateEventKey: "__new_event__", ClusteringVersion: "v1", FeatureInputHash: strings.Repeat("a", 64), Channel: domain.ChannelFingerprint, Decision: domain.DecisionNewEvent, DecisionOrigin: domain.DecisionOriginRule}}
	first, err := repository.ApplyClustering(ctx, decisions)
	if err != nil {
		t.Fatalf("ApplyClustering(first) error = %v", err)
	}
	second, err := repository.ApplyClustering(ctx, decisions)
	if err != nil {
		t.Fatalf("ApplyClustering(retry) error = %v", err)
	}
	if !first.Created || first.Event == nil || second.Created || second.Event == nil || first.Event.ID != second.Event.ID {
		t.Fatalf("ApplyClustering() = first %#v, second %#v", first, second)
	}
	var events, members, persisted, audits int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM events WHERE representative_content_id = $1`, contentID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE content_id = $1`, contentID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_clustering_decisions WHERE content_id = $1`, contentID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits WHERE event_id = $1`, first.Event.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if events != 1 || members != 1 || persisted != 1 || audits != 1 {
		t.Fatalf("events/members/decisions/audits = %d/%d/%d/%d, want 1/1/1/1", events, members, persisted, audits)
	}
}

func TestApplyClusteringAttachesAcceptedDecisionAndRetainsReview(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	fixture := seedEventFixture(t, runtime)
	contentID := seedUnassignedEventContent(t, runtime)
	target, err := repository.Get(ctx, fixture.targetID)
	if err != nil {
		t.Fatal(err)
	}
	accepted := domain.Decision{ContentID: contentID, CandidateEventID: &target.ID, CandidateEventKey: target.EventKey, ClusteringVersion: "v1", FeatureInputHash: strings.Repeat("b", 64), Channel: domain.ChannelLexical, CandidateRank: 1, Scores: domain.ScoreBreakdown{EntityAction: 90, Semantic: 90, Temporal: 90, Location: 90, SourceContext: 90}, MembershipScore: 90, Decision: domain.DecisionAccept, DecisionOrigin: domain.DecisionOriginRule, ReasonCodes: []string{"recalled_lexical", "recalled_vector", "membership_threshold_accepted"}, FeatureSnapshot: map[string]any{"recall_channels": []string{"lexical", "vector"}, "hard_conflict": false}, EvidenceContentIDs: []int64{contentID, fixture.sourceContentID}}
	result, err := repository.ApplyClustering(ctx, []domain.Decision{accepted})
	if err != nil {
		t.Fatalf("ApplyClustering(accepted) error = %v", err)
	}
	if result.Created || result.PendingReview || result.Event == nil || result.Event.ID != target.ID {
		t.Fatalf("ApplyClustering(accepted) = %#v", result)
	}
	var members int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE event_id = $1 AND content_id = $2`, target.ID, contentID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 1 {
		t.Fatalf("target members = %d, want 1", members)
	}
	var hardConflict string
	var channelCount, reasonCount, evidenceCount int
	if err := runtime.SQL.QueryRow(`SELECT feature_snapshot->>'hard_conflict', jsonb_array_length(feature_snapshot->'recall_channels'), cardinality(reason_codes), cardinality(evidence_content_ids) FROM event_clustering_decisions WHERE content_id = $1`, contentID).Scan(&hardConflict, &channelCount, &reasonCount, &evidenceCount); err != nil {
		t.Fatal(err)
	}
	if hardConflict != "false" || channelCount != 2 || reasonCount != 3 || evidenceCount != 2 {
		t.Fatalf("persisted audit provenance = hard_conflict=%q channels=%d reasons=%d evidence=%d", hardConflict, channelCount, reasonCount, evidenceCount)
	}
	reviewContentID := seedUnassignedEventContent(t, runtime)
	review := accepted
	review.ContentID = reviewContentID
	review.FeatureInputHash = strings.Repeat("c", 64)
	review.Decision = domain.DecisionReview
	result, err = repository.ApplyClustering(ctx, []domain.Decision{review})
	if err != nil {
		t.Fatalf("ApplyClustering(review) error = %v", err)
	}
	if !result.PendingReview || result.Event != nil {
		t.Fatalf("ApplyClustering(review) = %#v", result)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE content_id = $1`, reviewContentID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 0 {
		t.Fatalf("review members = %d, want 0", members)
	}
}

func TestApplyClusteringConcurrentReplayCreatesOneEvent(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(runtime)
	contentID := seedUnassignedEventContent(t, runtime)
	decisions := []domain.Decision{{ContentID: contentID, CandidateEventKey: "__new_event__", ClusteringVersion: "v1", FeatureInputHash: strings.Repeat("d", 64), Channel: domain.ChannelFingerprint, Decision: domain.DecisionNewEvent, DecisionOrigin: domain.DecisionOriginRule}}
	results := make(chan struct {
		resultID int64
		err      error
	}, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := repository.ApplyClustering(ctx, decisions)
			if result.Event == nil {
				results <- struct {
					resultID int64
					err      error
				}{err: err}
				return
			}
			results <- struct {
				resultID int64
				err      error
			}{resultID: result.Event.ID, err: err}
		}()
	}
	group.Wait()
	close(results)
	var eventID int64
	for result := range results {
		if result.err != nil {
			t.Fatalf("ApplyClustering(concurrent) error = %v", result.err)
		}
		if eventID == 0 {
			eventID = result.resultID
		} else if result.resultID != eventID {
			t.Fatalf("concurrent event ids = %d and %d", eventID, result.resultID)
		}
	}
	var events, members, persisted int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM events WHERE representative_content_id = $1`, contentID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE content_id = $1`, contentID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_clustering_decisions WHERE content_id = $1`, contentID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if events != 1 || members != 1 || persisted != 1 {
		t.Fatalf("events/members/decisions = %d/%d/%d, want 1/1/1", events, members, persisted)
	}
}

func seedUnassignedEventContent(t *testing.T, runtime *database.Runtime) int64 {
	t.Helper()
	var sourceID, contentID int64
	now := time.Now().UTC()
	if err := runtime.SQL.QueryRow(`INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'event-unassigned-' || md5(random()::text), 'https://event.example') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (source_connection_id, external_id, content_type, title, excerpt, canonical_url, published_at, fetched_at, dedupe_key) VALUES ($1, md5(random()::text), 'article', '未归属事件', '未归属摘要', 'https://event.example/unassigned', $2, $2, md5(random()::text) || md5(random()::text)) RETURNING id`, sourceID, now).Scan(&contentID); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	return contentID
}
