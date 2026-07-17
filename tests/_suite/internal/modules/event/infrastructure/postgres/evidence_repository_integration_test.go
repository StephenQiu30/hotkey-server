//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestRecomputeEventEvidenceUpdatesLifecycleRepresentativeAndAudit(t *testing.T) {
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
	secondContentID := seedUnassignedEventContent(t, runtime)
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,95,'supporting','rule')`, fixture.sourceID, secondContentID); err != nil {
		t.Fatal(err)
	}
	updated, err := repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: fixture.sourceID, ReasonCode: "evidence_refresh"})
	if err != nil {
		t.Fatalf("RecomputeEventEvidence() error = %v", err)
	}
	if updated.LifecycleStatus != domain.LifecycleActive || updated.RepresentativeContentID == nil || *updated.RepresentativeContentID != secondContentID || updated.Version != 2 {
		t.Fatalf("RecomputeEventEvidence() = %#v", updated)
	}
	if _, err := runtime.SQL.Exec(`UPDATE contents SET content_status = 'deleted' WHERE id IN ($1,$2)`, fixture.sourceContentID, secondContentID); err != nil {
		t.Fatal(err)
	}
	updated, err = repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: fixture.sourceID, ReasonCode: "content_deleted"})
	if err != nil {
		t.Fatalf("RecomputeEventEvidence(deleted) error = %v", err)
	}
	if updated.LifecycleStatus != domain.LifecycleRejected || updated.RepresentativeContentID != nil || updated.Version != 3 {
		t.Fatalf("RecomputeEventEvidence(deleted) = %#v", updated)
	}
	var members, audits int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_contents WHERE event_id = $1`, fixture.sourceID).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits WHERE event_id = $1 AND action = 'evidence_recompute'`, fixture.sourceID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if members != 2 || audits != 2 {
		t.Fatalf("members/audits = %d/%d, want 2/2", members, audits)
	}
}

func TestRecomputeEventEvidenceDoesNotOverrideManualLock(t *testing.T) {
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
	if _, err := runtime.SQL.Exec(`UPDATE events SET manual_locked = true WHERE id = $1`, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	updated, err := repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: fixture.sourceID, ReasonCode: "content_deleted"})
	if err != nil {
		t.Fatalf("RecomputeEventEvidence() error = %v", err)
	}
	if updated.LifecycleStatus != domain.LifecycleDetected || updated.Version != 1 {
		t.Fatalf("RecomputeEventEvidence() = %#v", updated)
	}
	var audits int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM event_governance_audits WHERE event_id = $1 AND action = 'evidence_recompute'`, fixture.sourceID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("audit count = %d, want 1", audits)
	}
}
