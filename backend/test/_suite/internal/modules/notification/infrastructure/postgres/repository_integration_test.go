//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/test/postgresfixture"
)

func TestRepositoryFiltersAudienceAndResumesFromGlobalCursor(t *testing.T) {
	ctx := context.Background()
	runtime := openNotificationRuntime(t, ctx)
	repository := NewRepository(runtime)
	now := time.Now().UTC().Truncate(time.Microsecond)

	for index, audience := range []domain.AudienceRole{domain.AudienceViewer, domain.AudienceEditor, domain.AudienceAdmin} {
		if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO notification_events (event_type, resource_type, resource_id, audience_role, occurred_at, payload, dedupe_key)
VALUES ('event.updated', 'event', $1, $2, $3, jsonb_build_object('title', $4::text), $5)`, index+1, audience, now.Add(time.Duration(index)*time.Second), "notification", "repository:"+string(audience)); err != nil {
			t.Fatal(err)
		}
	}

	viewerPage, err := repository.ListAfter(ctx, domain.NotificationQuery{Role: domain.AudienceViewer, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(viewerPage.Items) != 1 || viewerPage.Items[0].Audience != domain.AudienceViewer {
		t.Fatalf("viewer page = %#v", viewerPage)
	}

	editorPage, err := repository.ListAfter(ctx, domain.NotificationQuery{Role: domain.AudienceEditor, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(editorPage.Items) != 1 || editorPage.NextAfterID != editorPage.Items[0].ID {
		t.Fatalf("editor first page = %#v", editorPage)
	}
	resumed, err := repository.ListAfter(ctx, domain.NotificationQuery{Role: domain.AudienceEditor, AfterID: editorPage.NextAfterID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Items) != 1 || resumed.Items[0].Audience != domain.AudienceEditor {
		t.Fatalf("editor resumed page = %#v", resumed)
	}

	adminPage, err := repository.ListAfter(ctx, domain.NotificationQuery{Role: domain.AudienceAdmin, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminPage.Items) != 3 {
		t.Fatalf("admin items = %d, want 3", len(adminPage.Items))
	}
	for index := 1; index < len(adminPage.Items); index++ {
		if adminPage.Items[index-1].ID >= adminPage.Items[index].ID {
			t.Fatalf("items are not ordered by global cursor: %#v", adminPage.Items)
		}
	}
}

func TestSchemaProjectsBusinessFactsTransactionallyAndOnce(t *testing.T) {
	ctx := context.Background()
	runtime := openNotificationRuntime(t, ctx)
	now := time.Now().UTC().Truncate(time.Microsecond)

	var sourceID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO source_connections (source_type, name, endpoint)
VALUES ('rss', 'notification fixture', 'https://example.com/feed') RETURNING id`).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	var eventID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO events (event_key, title_zh, lifecycle_status, first_seen_at, last_seen_at)
VALUES ('notification-event', '通知事件', 'active', $1, $1) RETURNING id`, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	var updateID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO event_updates (event_id, sequence_no, kind, summary, observed_at, after_state, evidence_set_hash, idempotency_key)
VALUES ($1, 1, 'rising', '热度持续上升', $2, '{}', repeat('a', 64), repeat('b', 64)) RETURNING id`, eventID, now).Scan(&updateID); err != nil {
		t.Fatal(err)
	}

	var monitorID, configID int64
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO monitors (name) VALUES ('notification monitor') RETURNING id`).Scan(&monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO monitor_config_versions (monitor_id, revision) VALUES ($1, 1) RETURNING id`, monitorID).Scan(&configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE monitor_config_versions SET state='published', config_hash=repeat('c',64), published_at=$2 WHERE id=$1`, configID, now); err != nil {
		t.Fatal(err)
	}
	var threadID int64
	if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO alert_threads (
  monitor_id, monitor_config_version_id, monitor_revision, monitor_config_hash, event_id,
  trigger_type, policy_version, severity, event_threshold_snapshot, title_snapshot,
  first_triggered_at, last_triggered_at, cooldown_until
) VALUES ($1,$2,1,repeat('c',64),$3,'rising','v1','warning',70,'通知告警',$4::timestamptz,$4::timestamptz,$4::timestamptz + interval '1 hour')
RETURNING id`, monitorID, configID, eventID, now).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `
INSERT INTO alert_occurrences (alert_thread_id,event_update_id,severity,final_score_snapshot,threshold_snapshot,fingerprint,triggered_at)
VALUES ($1,$2,'warning',80,70,repeat('d',64),$3)`, threadID, updateID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	var publishedReportID, failedReportID int64
	for reportIndex, fixture := range []struct {
		status string
		id     *int64
	}{
		{status: "published", id: &publishedReportID},
		{status: "failed", id: &failedReportID},
	} {
		if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO reports (report_type, period_start, period_end, timezone, title)
VALUES ('daily', $1::timestamptz, $1::timestamptz + interval '1 day', 'UTC', $2::text) RETURNING id`, now.Add(time.Duration(reportIndex)*24*time.Hour), "通知报告 "+fixture.status).Scan(fixture.id); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET status=$2::text, published_at=CASE WHEN $2::text='published' THEN $3::timestamptz ELSE NULL END, updated_at=$3::timestamptz WHERE id=$1`, *fixture.id, fixture.status, now.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	for index, fixture := range []struct {
		status string
	}{
		{status: "succeeded"},
		{status: "failed"},
	} {
		var runID int64
		windowStart := now.Add(time.Duration(index+1) * time.Hour)
		if err := runtime.SQL.QueryRowContext(ctx, `
INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
VALUES ($1, $2, $3::timestamptz, $3::timestamptz + interval '1 hour', 'schedule', $3::timestamptz) RETURNING id`, sourceID, repeatHex(byte('e'+index)), windowStart).Scan(&runID); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.SQL.ExecContext(ctx, `UPDATE collection_runs SET status=$2::text, finished_at=$3::timestamptz, updated_at=$3::timestamptz WHERE id=$1`, runID, fixture.status, now.Add(3*time.Second)); err != nil {
			t.Fatal(err)
		}
	}

	assertNotificationTypes(t, runtime, map[string]int{
		"event.updated":        1,
		"alert.triggered":      1,
		"report.published":     1,
		"report.failed":        1,
		"collection.succeeded": 1,
		"collection.failed":    1,
	})

	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE reports SET title='已发布报告标题变化', updated_at=now() WHERE id=$1`, publishedReportID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE collection_runs SET accepted_count=1, updated_at=now() WHERE status='succeeded'`); err != nil {
		t.Fatal(err)
	}
	assertNotificationCount(t, runtime, 6)

	tx, err := runtime.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO event_updates (event_id, sequence_no, kind, summary, observed_at, after_state, evidence_set_hash, idempotency_key)
VALUES ($1, 2, 'metric_changed', '事务回滚', $2, '{}', repeat('f',64), repeat('0',64))`, eventID, now.Add(4*time.Second)); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertNotificationCount(t, runtime, 6)

	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE notification_events SET payload='{}' WHERE id=(SELECT min(id) FROM notification_events)`); err == nil {
		t.Fatal("notification outbox UPDATE succeeded, want append-only rejection")
	}
	if _, err := runtime.SQL.ExecContext(ctx, `DELETE FROM notification_events WHERE id=(SELECT min(id) FROM notification_events)`); err == nil {
		t.Fatal("notification outbox DELETE succeeded, want append-only rejection")
	}
}

func openNotificationRuntime(t *testing.T, ctx context.Context) *database.Runtime {
	t.Helper()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func assertNotificationTypes(t *testing.T, runtime *database.Runtime, want map[string]int) {
	t.Helper()
	rows, err := runtime.SQL.Query(`SELECT event_type, count(*) FROM notification_events GROUP BY event_type`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			t.Fatal(err)
		}
		got[eventType] = count
	}
	if len(got) != len(want) {
		t.Fatalf("notification types = %#v, want %#v", got, want)
	}
	for eventType, count := range want {
		if got[eventType] != count {
			t.Fatalf("notification type %s count = %d, want %d", eventType, got[eventType], count)
		}
	}
}

func assertNotificationCount(t *testing.T, runtime *database.Runtime, want int) {
	t.Helper()
	var got int
	if err := runtime.SQL.QueryRow(`SELECT count(*) FROM notification_events`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("notification count = %d, want %d", got, want)
	}
}

func repeatHex(character byte) string {
	buffer := make([]byte, 64)
	for index := range buffer {
		buffer[index] = character
	}
	return string(buffer)
}
