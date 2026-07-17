//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestEventSchemaRetainsAppendOnlyFactsAndImmutableKey(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedEventFixture(t, runtime)
	if _, err := runtime.SQL.Exec(`
INSERT INTO event_clustering_decisions (
  content_id, candidate_event_id, candidate_event_key, clustering_version, feature_input_hash, channel, candidate_rank,
  entity_action_score, semantic_score, temporal_score, location_score, source_context_score, membership_score,
  decision, decision_origin, reason_codes, feature_snapshot, evidence_content_ids
) VALUES ($1,$2,'evt-schema','v1',repeat('a',64),'lexical',1,0,0,0,0,0,0,'rejected','rule',ARRAY['schema_test'],'{}',ARRAY[$1::bigint])`, fixture.sourceContentID, fixture.targetID); err != nil {
		t.Fatalf("insert clustering decision: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_governance_audits (event_id, action, reason_code, metadata) VALUES ($1,'evidence_recompute','schema_test','{}')`, fixture.sourceID); err != nil {
		t.Fatalf("insert governance audit: %v", err)
	}
	for _, statement := range []string{
		`UPDATE events SET event_key = 'evt-mutated' WHERE id = 1`,
		`UPDATE event_clustering_decisions SET decision = 'accepted'`,
		`DELETE FROM event_clustering_decisions`,
		`UPDATE event_governance_audits SET reason_code = 'mutated'`,
		`DELETE FROM event_governance_audits`,
	} {
		if _, err := runtime.SQL.Exec(statement); err == nil {
			t.Fatalf("schema allowed protected mutation: %s", statement)
		}
	}
}
