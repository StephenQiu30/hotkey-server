//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventjobs "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/infrastructure/jobs"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestProductEventRefreshRiverReplaysMembersMetricsSnapshotsUpdatesAndNotifications(t *testing.T) {
	ctx := context.Background()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}

	first := seedMicroEventAssignmentFixture(t, runtime, "product-refresh-first", "accepted")
	second := seedMicroEventAssignmentFixture(t, runtime, "product-refresh-second", "accepted")
	var adminID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO users (email,password_hash,display_name,role)
VALUES ('product-refresh-admin@example.test','fixture','Refresh Admin','admin') RETURNING id`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO event_heat_profiles (
profile_version,status,lineage_weight,velocity_weight,acceleration_weight,coverage_weight,engagement_weight,
recency_weight,activated_by_user_id,activated_at)
		VALUES ('heat-v2-refresh','active',.25,.20,.15,.15,.10,.15,$1,$2)`, adminID,
		time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.Exec(`INSERT INTO evidence_state_profiles
(algorithm_version,status,activated_by_user_id,activated_at) VALUES ($1,'active',$2,$3)`,
		eventapplication.CanonicalEvidenceStateAlgorithmVersion, adminID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	allowRefreshNotificationForMonitor(t, runtime, first.monitorID, adminID)

	microRepository, _ := NewMicroEventRepository(runtime)
	created, err := microRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(first, "create", 0, 0,
		strings.Repeat("8", 64), "product-refresh-create"))
	if err != nil {
		t.Fatal(err)
	}
	heatRepository, _ := NewEventHeatRepository(runtime)
	heatService, _ := eventapplication.NewEventHeatService(heatRepository)
	evidenceRepository, _ := NewClaimEvidencePostgresRepository(runtime)
	evidenceService, _ := eventapplication.NewClaimEvidenceService(evidenceRepository)
	refreshRepository, _ := NewProductEventRefreshPostgresRepository(runtime)
	refreshService, _ := eventapplication.NewProductEventRefreshService(refreshRepository, heatService,
		evidenceService, refreshRepository)
	store := queue.NewStore(runtime)
	scheduler, _ := eventjobs.NewProductEventRefreshScheduler(refreshRepository, store)
	handler, _ := eventjobs.NewProductEventRefreshHandler(refreshService)
	worker := queue.NewWorker(runtime, map[string]queue.Handler{queue.KindRefreshProductEvent: handler.Handle})
	windowEnd := time.Now().UTC().Add(time.Minute).Truncate(time.Minute)

	firstSchedule, err := scheduler.ScheduleProductEventRefresh(ctx, eventapplication.ScheduleProductEventRefreshCommand{
		MicroEventID: created.Event.ID, ExpectedEventVersion: created.Event.Version, WindowEndedAt: windowEnd})
	if err != nil {
		t.Fatal(err)
	}
	replayedSchedule, err := scheduler.ScheduleProductEventRefresh(ctx, eventapplication.ScheduleProductEventRefreshCommand{
		MicroEventID: created.Event.ID, ExpectedEventVersion: created.Event.Version, WindowEndedAt: windowEnd})
	if err != nil || !firstSchedule.Created || replayedSchedule.Created || firstSchedule.JobID != replayedSchedule.JobID {
		t.Fatalf("refresh schedule replay = %#v / %#v / %v", firstSchedule, replayedSchedule, err)
	}
	makeRefreshJobsDue(t, runtime)
	if ran, err := worker.RunOnce(ctx); err != nil || !ran {
		t.Fatalf("first refresh worker = %t/%v", ran, err)
	}
	assertProductEventRefreshCounts(t, runtime, 1, 3, 1, 1, 1, 1)

	replayed, err := refreshService.Refresh(ctx, eventapplication.RefreshProductEventCommand{
		MicroEventID: created.Event.ID, ExpectedEventVersion: created.Event.Version, WindowEndedAt: windowEnd,
		WindowProfile: eventapplication.ProductEventRefreshWindowProfile, HeatProfileVersion: "heat-v2-refresh",
		EvidenceStateAlgorithmVersion: eventapplication.CanonicalEvidenceStateAlgorithmVersion})
	if err != nil || replayed.Update.Created || replayed.AlertEvaluation.NotificationCount != 0 ||
		replayed.AlertEvaluation.DuplicateCount != 1 {
		t.Fatalf("refresh service replay = %#v / %v", replayed, err)
	}
	assertProductEventRefreshCounts(t, runtime, 1, 3, 1, 1, 1, 1)

	joined, err := microRepository.CommitMicroEventMembership(ctx, microEventCommitFixture(second, "join",
		created.Event.ID, created.Event.Version, strings.Repeat("9", 64), "product-refresh-join"))
	if err != nil {
		t.Fatal(err)
	}
	allowRefreshNotificationForMonitor(t, runtime, second.monitorID, adminID)
	if _, err := scheduler.ScheduleProductEventRefresh(ctx, eventapplication.ScheduleProductEventRefreshCommand{
		MicroEventID: joined.Event.ID, ExpectedEventVersion: joined.Event.Version, WindowEndedAt: windowEnd}); err != nil {
		t.Fatal(err)
	}
	makeRefreshJobsDue(t, runtime)
	if ran, err := worker.RunOnce(ctx); err != nil || !ran {
		t.Fatalf("member refresh worker = %t/%v", ran, err)
	}
	assertProductEventRefreshCounts(t, runtime, 2, 6, 2, 2, 3, 3)

	contentID := attachMetricContentToMicroEventFixture(t, runtime, first)
	metricRefresh, _ := eventapplication.NewContentMetricRefreshServiceWithClock(heatRepository, scheduler,
		func() time.Time { return windowEnd.Add(time.Minute) })
	if err := metricRefresh.RecomputeMetricsForContent(ctx, contentID); err != nil {
		t.Fatal(err)
	}
	makeRefreshJobsDue(t, runtime)
	if ran, err := worker.RunOnce(ctx); err != nil || !ran {
		t.Fatalf("late metric refresh worker = %t/%v", ran, err)
	}
	assertProductEventRefreshCounts(t, runtime, 2, 9, 2, 3, 5, 5)

	var legacyEvents, legacyTopics, legacyUpdates int
	if err := runtime.SQL.QueryRow(`SELECT (SELECT count(*) FROM events),(SELECT count(*) FROM topics),
(SELECT count(*) FROM event_updates)`).Scan(&legacyEvents, &legacyTopics, &legacyUpdates); err != nil {
		t.Fatal(err)
	}
	if legacyEvents != 0 || legacyTopics != 0 || legacyUpdates != 0 {
		t.Fatalf("legacy writes = events %d topics %d updates %d", legacyEvents, legacyTopics, legacyUpdates)
	}
}

func makeRefreshJobsDue(t *testing.T, runtime *database.Runtime) {
	t.Helper()
	if _, err := runtime.SQL.Exec(`UPDATE river_job SET scheduled_at=now()-interval '1 second'
WHERE kind=$1 AND state='available'`, queue.KindRefreshProductEvent); err != nil {
		t.Fatal(err)
	}
}

func allowRefreshNotificationForMonitor(t *testing.T, runtime *database.Runtime, monitorID, ownerID int64) {
	t.Helper()
	transaction, err := runtime.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`SET LOCAL session_replication_role='replica'`); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE monitors SET created_by=$2,published_config_version_id=(
SELECT id FROM monitor_config_versions WHERE monitor_id=$1 AND state='published' ORDER BY revision DESC,id DESC LIMIT 1)
WHERE id=$1`, monitorID, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Exec(`UPDATE monitor_config_versions SET event_threshold=0,alert_min_heat=0
WHERE id=(SELECT published_config_version_id FROM monitors WHERE id=$1)`, monitorID); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
}

func attachMetricContentToMicroEventFixture(t *testing.T, runtime *database.Runtime, fixture microEventAssignmentFixture) int64 {
	t.Helper()
	if _, err := runtime.SQL.Exec(`UPDATE documents SET current_document_version_id=$1
WHERE source_connection_id=$2 AND external_work_id=$3`, fixture.documentVersionID, fixture.sourceID,
		"work-product-refresh-first"); err != nil {
		t.Fatal(err)
	}
	var contentID int64
	if err := runtime.SQL.QueryRow(`INSERT INTO contents (
source_connection_id,external_id,content_type,title,canonical_url,published_at,fetched_at,dedupe_key)
VALUES ($1,$2,'article','late metric fixture','https://product-refresh-first.example/article',$3,$3,$4)
RETURNING id`, fixture.sourceID, "work-product-refresh-first", fixture.occurredAt, strings.Repeat("7", 64)).Scan(&contentID); err != nil {
		t.Fatal(err)
	}
	return contentID
}

func assertProductEventRefreshCounts(t *testing.T, runtime *database.Runtime, members, heatSnapshots,
	evidenceSnapshots, updates, evaluations, outbox int) {
	t.Helper()
	var gotMembers, gotHeat, gotEvidence, gotUpdates, gotEvaluations, gotOutbox int
	if err := runtime.SQL.QueryRow(`SELECT
(SELECT count(*) FROM micro_event_members WHERE active),
(SELECT count(*) FROM micro_event_heat_snapshots),
(SELECT count(*) FROM evidence_state_snapshots),
(SELECT count(*) FROM micro_event_updates),
(SELECT count(*) FROM micro_event_alert_evaluations),
(SELECT count(*) FROM notification_outbox_events WHERE event_type='micro_event.updated')`).
		Scan(&gotMembers, &gotHeat, &gotEvidence, &gotUpdates, &gotEvaluations, &gotOutbox); err != nil {
		t.Fatal(err)
	}
	if gotMembers != members || gotHeat != heatSnapshots || gotEvidence != evidenceSnapshots || gotUpdates != updates ||
		gotEvaluations != evaluations || gotOutbox != outbox {
		t.Fatalf("refresh counts = members %d heat %d evidence %d updates %d evaluations %d outbox %d; want %d/%d/%d/%d/%d/%d",
			gotMembers, gotHeat, gotEvidence, gotUpdates, gotEvaluations, gotOutbox,
			members, heatSnapshots, evidenceSnapshots, updates, evaluations, outbox)
	}
}
