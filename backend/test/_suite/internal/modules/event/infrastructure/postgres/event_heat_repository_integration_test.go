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
	if result.Snapshot.HeatProfileID != profileID || result.Snapshot.HeatProfileVersion != "heat-v2" ||
		result.Snapshot.IndependentLineageRoots != 1 ||
		result.Snapshot.HeatScore <= 0 || result.Snapshot.NormalizedEngagement != nil || result.Snapshot.Created != true ||
		!result.Snapshot.WarmingUp || result.Snapshot.Velocity != 0 || result.Snapshot.Acceleration != 0 ||
		result.Snapshot.AvailableWeight != .5 {
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
	if _, err := repository.CommitEventHeatSnapshot(ctx, eventapplication.CommitEventHeatSnapshotCommand{
		MicroEventID: result.Snapshot.MicroEventID, MicroEventVersion: result.Snapshot.MicroEventVersion,
		HeatProfileID: result.Snapshot.HeatProfileID, HeatProfileVersion: "wrong-profile-version",
		WindowStartedAt: result.Snapshot.WindowStartedAt, WindowEndedAt: result.Snapshot.WindowEndedAt,
		IndependentLineageRoots: result.Snapshot.IndependentLineageRoots, Velocity: result.Snapshot.Velocity,
		Acceleration: result.Snapshot.Acceleration, Coverage: result.Snapshot.Coverage,
		NormalizedEngagement: result.Snapshot.NormalizedEngagement, Recency: result.Snapshot.Recency,
		AvailableWeight: result.Snapshot.AvailableWeight, HeatScore: result.Snapshot.HeatScore,
		WarmingUp: result.Snapshot.WarmingUp, ReasonCodes: result.Snapshot.ReasonCodes,
	}); err == nil {
		t.Fatal("heat snapshot accepted a profile version that did not match the profile id")
	}
}

func TestEventHeatRepositoryListsCurrentMicroEventForContent(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := seedMicroEventAssignmentFixture(t, runtime, "content-refresh", "accepted")
	microEvents, err := NewMicroEventRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	created, err := microEvents.CommitMicroEventMembership(ctx, microEventCommitFixture(
		fixture, "create", 0, 0, strings.Repeat("9", 64), "micro-event-content-refresh",
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`UPDATE documents
SET current_document_version_id=$1
WHERE source_connection_id=$2 AND external_work_id='work-content-refresh'`, fixture.documentVersionID, fixture.sourceID); err != nil {
		t.Fatal(err)
	}
	var contentID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (
source_connection_id,external_id,content_type,title,canonical_url,published_at,fetched_at,dedupe_key)
VALUES ($1,'work-content-refresh','article','content refresh','https://content-refresh.example/article',$2,$2,$3)
RETURNING id`, fixture.sourceID, fixture.occurredAt, strings.Repeat("7", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	repository, err := NewEventHeatRepository(runtime)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := repository.ListMetricMicroEventIDsForContent(ctx, contentID)
	if err != nil {
		t.Fatalf("list content micro-events: %v", err)
	}
	if len(ids) != 1 || ids[0] != created.Event.ID {
		t.Fatalf("micro-event ids = %#v, want [%d]", ids, created.Event.ID)
	}
}
