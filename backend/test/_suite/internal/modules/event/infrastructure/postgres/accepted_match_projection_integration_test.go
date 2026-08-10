//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestAcceptedMatchProjectionCreatesMicroEventStorylineAndHeatV2(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	var actorID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('heat-activator@example.test','fixture','Heat Activator','admin') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_heat_profiles (
profile_version,status,lineage_weight,velocity_weight,acceleration_weight,coverage_weight,engagement_weight,
recency_weight,activated_by_user_id,activated_at)
VALUES ('heat-v2-integration','active',.25,.20,.15,.15,.10,.15,$1,$2)`, actorID,
		time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "accepted-projection", "accepted")
	microRepository, _ := NewMicroEventRepository(runtime)
	microService, _ := eventapplication.NewMicroEventService(microRepository)
	storylineRepository, _ := NewStorylinePostgresRepository(runtime)
	storylineService, _ := eventapplication.NewStorylineService(storylineRepository)
	heatRepository, _ := NewEventHeatRepository(runtime)
	heatService, _ := eventapplication.NewEventHeatService(heatRepository)
	service, err := eventapplication.NewAcceptedMatchEventProjectionService(microRepository, microService,
		storylineService, heatService)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := service.Project(ctx, eventapplication.ProjectAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: fixture.matchDecisionID, DocumentVersionID: fixture.documentVersionID})
	if err != nil || projected.MicroEvent.ID <= 0 || projected.Storyline == nil || projected.StorylineEvent == nil ||
		projected.HeatUnavailable || len(projected.HeatSnapshots) != 3 {
		t.Fatalf("accepted projection = %#v / %v", projected, err)
	}
	for _, snapshot := range projected.HeatSnapshots {
		if snapshot.MicroEventID != projected.MicroEvent.ID || snapshot.HeatProfileID <= 0 {
			t.Fatalf("heat snapshot = %#v", snapshot)
		}
	}
	replayed, err := service.Project(ctx, eventapplication.ProjectAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: fixture.matchDecisionID, DocumentVersionID: fixture.documentVersionID})
	if err != nil || replayed.MicroEvent.ID != projected.MicroEvent.ID || replayed.Storyline == nil ||
		replayed.Storyline.ID != projected.Storyline.ID || replayed.HeatSnapshots[0].ID != projected.HeatSnapshots[0].ID {
		t.Fatalf("accepted projection replay = %#v / %v", replayed, err)
	}
}

func TestAcceptedMatchProjectionKeepsEventWhenHeatProfileIsUnavailable(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "accepted-without-heat", "accepted")
	microRepository, _ := NewMicroEventRepository(runtime)
	microService, _ := eventapplication.NewMicroEventService(microRepository)
	storylineRepository, _ := NewStorylinePostgresRepository(runtime)
	storylineService, _ := eventapplication.NewStorylineService(storylineRepository)
	heatRepository, _ := NewEventHeatRepository(runtime)
	heatService, _ := eventapplication.NewEventHeatService(heatRepository)
	service, err := eventapplication.NewAcceptedMatchEventProjectionService(microRepository, microService,
		storylineService, heatService)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := service.Project(ctx, eventapplication.ProjectAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: fixture.matchDecisionID, DocumentVersionID: fixture.documentVersionID})
	if err != nil || projected.MicroEvent.ID <= 0 || projected.Storyline == nil || !projected.HeatUnavailable || len(projected.HeatSnapshots) != 0 {
		t.Fatalf("accepted projection without heat profile = %#v / %v", projected, err)
	}
}
