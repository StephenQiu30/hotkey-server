//go:build integration

package postgres

import (
	"context"
	"testing"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestStorylineRepositoryCreatesSpecificMicroEventsThenRelatesThemWithoutCollapsing(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	microEventRepository, _ := NewMicroEventRepository(runtime)
	microEventService, _ := eventapplication.NewMicroEventService(microEventRepository)
	storylineRepository, _ := NewStorylinePostgresRepository(runtime)
	storylineService, _ := eventapplication.NewStorylineService(storylineRepository)

	firstFixture := seedMicroEventAssignmentFixture(t, runtime, "storyline-first", "accepted")
	firstEvent, err := microEventService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: firstFixture.familyID, DocumentMatchDecisionID: firstFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	firstRelation, err := storylineService.Assign(ctx, eventapplication.AssignMicroEventToStorylineCommand{
		MicroEventID: firstEvent.Event.ID, MicroEventVersion: firstEvent.Event.Version,
		RelationProfileVersion: eventapplication.CanonicalStorylineRelationProfileVersion})
	if err != nil || firstRelation.Storyline.Version != 1 {
		t.Fatalf("first storyline = %#v / %v", firstRelation, err)
	}

	secondFixture := seedMicroEventAssignmentFixture(t, runtime, "storyline-second", "accepted")
	fixtureTransaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixtureTransaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		fixtureTransaction.Rollback()
		t.Fatal(err)
	}
	if _, err := fixtureTransaction.Exec(`UPDATE document_version_search_indexes
SET action_keys=ARRAY['action:response'] WHERE document_version_id=$1`, secondFixture.documentVersionID); err != nil {
		fixtureTransaction.Rollback()
		t.Fatal(err)
	}
	if err := fixtureTransaction.Commit(); err != nil {
		t.Fatal(err)
	}
	secondEvent, err := microEventService.Assign(ctx, eventapplication.AssignContentFamilyToMicroEventCommand{
		ContentFamilyID: secondFixture.familyID, DocumentMatchDecisionID: secondFixture.matchDecisionID,
		ClusteringProfileVersion: eventapplication.CanonicalMicroEventClusteringProfileVersion})
	if err != nil {
		t.Fatal(err)
	}
	if secondEvent.Event.ID == firstEvent.Event.ID {
		t.Fatalf("storyline fixture unexpectedly collapsed micro-events: %#v", secondEvent)
	}
	secondRelation, err := storylineService.Assign(ctx, eventapplication.AssignMicroEventToStorylineCommand{
		MicroEventID: secondEvent.Event.ID, MicroEventVersion: secondEvent.Event.Version,
		RelationProfileVersion: eventapplication.CanonicalStorylineRelationProfileVersion})
	if err != nil || secondRelation.Storyline.ID != firstRelation.Storyline.ID ||
		secondRelation.Storyline.Version != 2 || secondRelation.Relation.RelationType != "continues" {
		t.Fatalf("second storyline = %#v / %v", secondRelation, err)
	}

	replayed, err := storylineService.Assign(ctx, eventapplication.AssignMicroEventToStorylineCommand{
		MicroEventID: firstEvent.Event.ID, MicroEventVersion: firstEvent.Event.Version,
		RelationProfileVersion: eventapplication.CanonicalStorylineRelationProfileVersion})
	if err != nil || replayed.Storyline.Version != 1 || replayed.Relation.ID != firstRelation.Relation.ID {
		t.Fatalf("stable historical replay = %#v / %v", replayed, err)
	}
	var storylineCount, relationCount int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM storylines`).Scan(&storylineCount); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM storyline_events`).Scan(&relationCount); err != nil {
		t.Fatal(err)
	}
	if storylineCount != 1 || relationCount != 2 {
		t.Fatalf("storyline counts = %d / %d", storylineCount, relationCount)
	}
}
