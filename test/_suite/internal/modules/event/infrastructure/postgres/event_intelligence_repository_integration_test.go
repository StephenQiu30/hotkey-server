//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestRepositoryLoadsEventIntelligenceSource(t *testing.T) {
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
	activeContentID := seedUnassignedEventContent(t, runtime)
	duplicateContentID := seedUnassignedEventContent(t, runtime)
	if _, err := runtime.SQL.Exec(`UPDATE contents SET content_status = 'deleted' WHERE id = $1`, fixture.sourceContentID); err != nil {
		t.Fatalf("delete original content: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, is_representative, origin) VALUES ($1,$2,95,'supporting',true,'rule')`, fixture.sourceID, activeContentID); err != nil {
		t.Fatalf("insert active evidence: %v", err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, origin) VALUES ($1,$2,100,'duplicate','rule')`, fixture.sourceID, duplicateContentID); err != nil {
		t.Fatalf("insert duplicate evidence: %v", err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE events SET representative_content_id = $2 WHERE id = $1`, fixture.sourceID, activeContentID); err != nil {
		t.Fatalf("set representative content: %v", err)
	}

	source, err := repository.LoadEventIntelligenceSource(ctx, fixture.sourceID)
	if err != nil {
		t.Fatalf("LoadEventIntelligenceSource() error = %v", err)
	}
	if source.Event.ID != fixture.sourceID || source.Event.RepresentativeContentID == nil || *source.Event.RepresentativeContentID != activeContentID {
		t.Fatalf("event source = %#v, want fixture event and representative", source.Event)
	}
	if len(source.Evidence) != 1 || source.Evidence[0].ContentID != activeContentID || source.Evidence[0].Locator != "excerpt" || source.Evidence[0].Excerpt != "未归属摘要" {
		t.Fatalf("evidence = %#v, want only active non-duplicate content", source.Evidence)
	}
}
