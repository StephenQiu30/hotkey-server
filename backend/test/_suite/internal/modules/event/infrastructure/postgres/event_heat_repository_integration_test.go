//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestEventHeatRepositoryCountsAuthorizedLineageAndReplaysImmutableSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "heat", "accepted")
	microEvents, _ := NewMicroEventRepository(runtime)
	created, err := microEvents.CommitMicroEventMembership(ctx, microEventCommitFixture(
		fixture, "create", 0, 0, strings.Repeat("8", 64), "micro-event-heat-create",
	))
	if err != nil {
		t.Fatal(err)
	}
	var adminID, profileID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('heat-admin@example.test','not-a-real-password-hash','heat admin','admin') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRow(`INSERT INTO event_heat_profiles (
profile_version,status,lineage_weight,velocity_weight,acceleration_weight,coverage_weight,engagement_weight,recency_weight,
activated_by_user_id,activated_at)
VALUES ('heat-v2','active',.25,.20,.15,.15,.15,.10,$1,CURRENT_TIMESTAMP) RETURNING id`, adminID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	repository, err := NewEventHeatRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	service, err := eventapplication.NewEventHeatService(repository)
	if err != nil {
		t.Fatal(err)
	}
	windowEnd := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	result, err := service.Calculate(ctx, eventapplication.CalculateEventHeatCommand{
		MicroEventID: created.Event.ID, WindowHours: 24, WindowEndedAt: windowEnd,
	})
	if err != nil {
		t.Fatalf("calculate heat: %v", err)
	}
	if result.Snapshot.HeatProfileID != profileID || result.Snapshot.IndependentLineageRoots != 1 ||
		result.Snapshot.HeatScore <= 0 || result.Snapshot.NormalizedEngagement != nil || result.Snapshot.Created != true {
		t.Fatalf("heat result = %#v", result)
	}
	replayed, err := service.Calculate(ctx, eventapplication.CalculateEventHeatCommand{
		MicroEventID: created.Event.ID, WindowHours: 24, WindowEndedAt: windowEnd,
	})
	if err != nil || replayed.Snapshot.ID != result.Snapshot.ID || replayed.Snapshot.Created {
		t.Fatalf("heat replay = %#v / %v", replayed, err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE micro_event_heat_snapshots SET heat_score=0 WHERE id=$1`, result.Snapshot.ID); err == nil {
		t.Fatal("append-only heat snapshot accepted mutation")
	}
}
