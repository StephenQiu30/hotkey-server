//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestDecisionAndMetricPersistenceAreIdempotent(t *testing.T) {
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
	hash := strings.Repeat("a", 64)
	candidateID := fixture.targetID
	decision := domain.Decision{ContentID: fixture.sourceContentID, CandidateEventID: &candidateID, CandidateEventKey: "fixture-target", ClusteringVersion: "v1", FeatureInputHash: hash, Channel: domain.ChannelLexical, CandidateRank: 1, Scores: domain.ScoreBreakdown{EntityAction: 80, Semantic: 80, Temporal: 80, Location: 80, SourceContext: 80}, MembershipScore: 80, Decision: domain.DecisionAccept, DecisionOrigin: domain.DecisionOriginRule}
	if err := application.PersistDecisions(ctx, repository, []domain.Decision{decision, decision}); err != nil {
		t.Fatal(err)
	}
	var decisions int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_clustering_decisions WHERE content_id = $1`, fixture.sourceContentID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Fatalf("decision rows = %d, want 1", decisions)
	}
	when := time.Now().UTC().Truncate(time.Microsecond)
	result := domain.HeatResult{EventID: fixture.sourceID, HeatScore: 42, TrendScore: 4, SourceCount: 1, ContentCount: 1, HeatVersion: "v1", EvidenceSetHash: hash, WindowEnd: when}
	if err := repository.SaveHeatSnapshot(ctx, result); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveHeatSnapshot(ctx, result); err != nil {
		t.Fatal(err)
	}
	var snapshots int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_metric_snapshots WHERE event_id = $1`, fixture.sourceID).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if snapshots != 1 {
		t.Fatalf("metric rows = %d, want 1", snapshots)
	}
}

func TestVerifiedClaimPersistsOnlyActiveEventEvidence(t *testing.T) {
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
	claim := domain.Claim{ID: 1, Version: 1, EventID: fixture.sourceID, NormalizedClaim: "the event happened", ClaimHash: strings.Repeat("b", 64), Status: domain.ClaimSingleSource, Confidence: 80, Evidence: []domain.ClaimEvidence{{EvidenceRef: domain.EvidenceRef{ContentID: fixture.sourceContentID, Locator: "title", Excerpt: "Event content"}, Stance: "supports", Confidence: 90}}}
	active := map[int64]bool{fixture.sourceContentID: true}
	if _, err := application.SaveVerifiedClaim(ctx, repository, claim, active); err != nil {
		t.Fatal(err)
	}
	if _, err := application.SaveVerifiedClaim(ctx, repository, claim, map[int64]bool{}); err == nil {
		t.Fatal("inactive evidence accepted")
	}
	var count int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM claim_evidences WHERE content_id = $1`, fixture.sourceContentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("claim evidence rows = %d, want 1", count)
	}
}
