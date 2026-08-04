//go:build integration

package postgres_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/alert/domain"
	alertpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/alert/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/test/postgresfixture"
)

func TestRecordOccurrenceIsAtomicAndIdempotentUnderConcurrency(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	command := fixture.command(fixture.updateID, time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC))

	type outcome struct {
		result domain.RecordOccurrenceResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := fixture.repository.RecordOccurrence(context.Background(), command)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	workers.Wait()
	close(outcomes)

	created := 0
	var threadID, occurrenceID int64
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.result.Created {
			created++
		}
		if threadID == 0 {
			threadID, occurrenceID = outcome.result.Thread.ID, outcome.result.Occurrence.ID
		}
		if outcome.result.Thread.ID != threadID || outcome.result.Occurrence.ID != occurrenceID {
			t.Fatalf("concurrent workers returned distinct facts: %#v", outcome.result)
		}
	}
	if created != 1 {
		t.Fatalf("created results = %d, want 1", created)
	}
	assertAlertCounts(t, fixture, 1, 1, 1)

	before, err := fixture.repository.Get(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fixture.repository.RecordOccurrence(context.Background(), command)
	if err != nil || replayed.Created {
		t.Fatalf("replay = %#v/%v", replayed, err)
	}
	after, err := fixture.repository.Get(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Thread.Version != before.Thread.Version || after.Thread.OccurrenceCount != before.Thread.OccurrenceCount || !after.Thread.CooldownUntil.Equal(before.Thread.CooldownUntil) {
		t.Fatalf("duplicate changed thread: before=%#v after=%#v", before.Thread, after.Thread)
	}
}

func TestRecordOccurrenceRollsBackWhenThreadUpdateFails(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	first, err := fixture.repository.RecordOccurrence(context.Background(), fixture.command(fixture.updateID, time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	secondUpdateID := fixture.insertUpdate(t, "metric_changed", 2)
	if _, err := fixture.runtime.SQL.ExecContext(context.Background(), `
CREATE FUNCTION fail_alert_thread_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'injected thread update failure'; END $$;
CREATE TRIGGER fail_alert_thread_update BEFORE UPDATE ON alert_threads
FOR EACH ROW EXECUTE FUNCTION fail_alert_thread_update()`); err != nil {
		t.Fatal(err)
	}
	command := fixture.command(secondUpdateID, first.Thread.CooldownUntil.Add(time.Minute))
	if _, err := fixture.repository.RecordOccurrence(context.Background(), command); err == nil {
		t.Fatal("RecordOccurrence() error = nil after injected thread update failure")
	}
	assertAlertCounts(t, fixture, 1, 1, 1)
	detail, err := fixture.repository.Get(context.Background(), first.Thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Thread.Version != first.Thread.Version || detail.Thread.OccurrenceCount != 1 || !detail.Thread.CooldownUntil.Equal(first.Thread.CooldownUntil) {
		t.Fatalf("failed transaction partially changed thread: %#v", detail.Thread)
	}
}

func TestOutOfOrderOccurrenceOnlyAdvancesCountAndCannotReopen(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	newerUpdateID := fixture.insertUpdate(t, "metric_changed", 2)
	triggeredAt := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Microsecond)

	newer := fixture.command(newerUpdateID, triggeredAt)
	newer.FinalScoreSnapshot = 95
	newer.Severity = domain.SeverityCritical
	newer.TitleSnapshot = "Newest alert snapshot"
	newer.ReasonSnapshot = "newest reason"
	newer.Fingerprint, _ = domain.OccurrenceFingerprint(domain.FingerprintInput{
		MonitorConfigVersionID: fixture.configID, EventUpdateID: newerUpdateID,
		TriggerType: newer.TriggerType, PolicyVersion: newer.PolicyVersion,
	})
	first, err := fixture.repository.RecordOccurrence(context.Background(), newer)
	if err != nil {
		t.Fatal(err)
	}
	acknowledgedAt := triggeredAt.Add(2 * time.Hour)
	acknowledged, err := fixture.repository.Transition(context.Background(), domain.TransitionCommand{
		ThreadID: first.Thread.ID, ExpectedVersion: first.Thread.Version,
		To: domain.StateAcknowledged, ActorUserID: fixture.userID,
		ReasonCode: "reviewed", At: acknowledgedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A legacy/evolved policy could have a cooldown at the exact tuple time.
	// This makes the tuple tie-break, rather than the cooldown comparison, the
	// fact that prevents a delayed lower event_update_id from reopening.
	if _, err := fixture.runtime.SQL.ExecContext(context.Background(), `UPDATE alert_threads SET cooldown_until=$1 WHERE id=$2`, triggeredAt, acknowledged.ID); err != nil {
		t.Fatal(err)
	}

	delayed := fixture.command(fixture.updateID, triggeredAt)
	delayed.TitleSnapshot = "Delayed stale snapshot"
	delayed.ReasonSnapshot = "stale reason"
	writeStartedAt := time.Now().UTC()
	result, err := fixture.repository.RecordOccurrence(context.Background(), delayed)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Reopened {
		t.Fatalf("delayed result = %#v, want created without reopen", result)
	}
	thread := result.Thread
	if thread.State != domain.StateAcknowledged || thread.OccurrenceCount != 2 || thread.Version != acknowledged.Version+1 {
		t.Fatalf("delayed occurrence state/count/version = %q/%d/%d", thread.State, thread.OccurrenceCount, thread.Version)
	}
	if thread.Severity != newer.Severity || thread.TitleSnapshot != newer.TitleSnapshot || thread.ReasonSnapshot != newer.ReasonSnapshot {
		t.Fatalf("delayed occurrence replaced latest snapshot: %#v", thread)
	}
	if !thread.LastTriggeredAt.Equal(triggeredAt) || !thread.CooldownUntil.Equal(triggeredAt) {
		t.Fatalf("delayed occurrence advanced trigger/cooldown = %s/%s", thread.LastTriggeredAt, thread.CooldownUntil)
	}
	if !thread.UpdatedAt.After(acknowledgedAt) || thread.UpdatedAt.Before(writeStartedAt) {
		t.Fatalf("delayed occurrence updated_at = %s, want a write time at or after %s without regressing below %s", thread.UpdatedAt, writeStartedAt, acknowledgedAt)
	}
}

func TestRepositoryUsesCASAuditAndStableListDetailReads(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	first, err := fixture.repository.RecordOccurrence(context.Background(), fixture.command(fixture.updateID, time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	acknowledged, err := fixture.repository.Transition(context.Background(), domain.TransitionCommand{ThreadID: first.Thread.ID, ExpectedVersion: first.Thread.Version, To: domain.StateAcknowledged, ActorUserID: fixture.userID, ReasonCode: "reviewed", At: time.Date(2026, 8, 4, 7, 5, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if acknowledged.State != domain.StateAcknowledged || acknowledged.Version != first.Thread.Version+1 {
		t.Fatalf("acknowledged thread = %#v", acknowledged)
	}
	if _, err := fixture.repository.Transition(context.Background(), domain.TransitionCommand{ThreadID: first.Thread.ID, ExpectedVersion: first.Thread.Version, To: domain.StateResolved, ActorUserID: fixture.userID, ReasonCode: "stale", At: time.Now().UTC()}); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("stale transition error = %v, want ErrConflict", err)
	}

	secondEventID, secondUpdateID := fixture.insertEventAndUpdate(t)
	secondCommand := fixture.command(secondUpdateID, time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC))
	secondCommand.EventID = secondEventID
	secondCommand.FinalScoreSnapshot = 95
	secondCommand.Severity = domain.SeverityCritical
	secondCommand.Fingerprint, err = domain.OccurrenceFingerprint(domain.FingerprintInput{MonitorConfigVersionID: fixture.configID, EventUpdateID: secondUpdateID, TriggerType: secondCommand.TriggerType, PolicyVersion: domain.PolicyVersionV1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.repository.RecordOccurrence(context.Background(), secondCommand)
	if err != nil {
		t.Fatal(err)
	}

	page, err := fixture.repository.List(context.Background(), domain.ListQuery{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second.Thread.ID || page.NextCursor == "" {
		t.Fatalf("first list page = %#v/%v", page, err)
	}
	next, err := fixture.repository.List(context.Background(), domain.ListQuery{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != first.Thread.ID {
		t.Fatalf("second list page = %#v/%v", next, err)
	}
	state := domain.StateOpen
	if _, err := fixture.repository.List(context.Background(), domain.ListQuery{Limit: 1, Cursor: page.NextCursor, State: &state}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("cross-shape cursor error = %v, want ErrInvalidInput", err)
	}
	for name, cursor := range map[string]string{
		"rewritten id":    rewriteAlertCursorWithoutSigning(t, page.NextCursor, "id", float64(first.Thread.ID)),
		"rewritten shape": rewriteAlertCursorWithoutSigning(t, page.NextCursor, "shape", strings.Repeat("f", 64)),
		"expired":         resignAlertCursor(t, page.NextCursor, fixture.cursorSecret, "issued_at", time.Now().UTC().Add(-16*time.Minute).Format(time.RFC3339Nano)),
	} {
		if _, err := fixture.repository.List(context.Background(), domain.ListQuery{Limit: 1, Cursor: cursor}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("%s cursor error = %v, want ErrInvalidInput", name, err)
		}
	}

	detail, err := fixture.repository.Get(context.Background(), first.Thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Occurrences) != 1 || len(detail.Audits) != 1 || detail.Audits[0].ActorType != domain.ActorUser || detail.Audits[0].ReasonCode != "reviewed" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestGetReadsThreadOccurrencesAndAuditsFromOneRepeatableReadSnapshot(t *testing.T) {
	fixture := newAlertRepositoryFixture(t)
	first, err := fixture.repository.RecordOccurrence(context.Background(), fixture.command(fixture.updateID, time.Date(2026, 8, 4, 7, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	secondUpdateID := fixture.insertUpdate(t, "metric_changed", 2)
	second := fixture.command(secondUpdateID, time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC))
	second.FinalScoreSnapshot, second.Severity = 95, domain.SeverityCritical
	second.Fingerprint, _ = domain.OccurrenceFingerprint(domain.FingerprintInput{
		MonitorConfigVersionID: fixture.configID, EventUpdateID: secondUpdateID,
		TriggerType: second.TriggerType, PolicyVersion: second.PolicyVersion,
	})

	writer, err := fixture.runtime.SQL.BeginTx(context.Background(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.ExecContext(context.Background(), `LOCK TABLE alert_occurrences IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(context.Background(), `
UPDATE alert_threads SET version=version+1,state='acknowledged',severity=$1,title_snapshot='committed snapshot',
    reason_snapshot='committed reason',last_triggered_at=$2,occurrence_count=occurrence_count+1,
    cooldown_until=$3,acknowledged_at=$2,acknowledged_by_user_id=$4,updated_at=$2
WHERE id=$5`, second.Severity, second.TriggeredAt, domain.CooldownUntil(second.TriggeredAt), fixture.userID, first.Thread.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(context.Background(), `
INSERT INTO alert_occurrences (alert_thread_id,event_update_id,severity,final_score_snapshot,threshold_snapshot,reason_codes,fingerprint,triggered_at)
VALUES ($1,$2,$3,$4,$5,'{}',$6,$7)`, first.Thread.ID, secondUpdateID, second.Severity, second.FinalScoreSnapshot,
		second.EventThresholdSnapshot, second.Fingerprint, second.TriggeredAt); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(context.Background(), `
INSERT INTO alert_state_audits (alert_thread_id,actor_type,actor_user_id,from_state,to_state,expected_version,reason_code,created_at)
VALUES ($1,'user',$2,'open','acknowledged',$3,'snapshot_test',$4)`, first.Thread.ID, fixture.userID, first.Thread.Version, second.TriggeredAt); err != nil {
		t.Fatal(err)
	}

	type detailResult struct {
		detail domain.ThreadDetail
		err    error
	}
	result := make(chan detailResult, 1)
	go func() {
		detail, err := fixture.repository.Get(context.Background(), first.Thread.ID)
		result <- detailResult{detail: detail, err: err}
	}()
	if !waitForBlockedOccurrenceRead(fixture.runtime.SQL, 3*time.Second) {
		t.Fatal("Get did not block on the occurrence relation")
	}
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	read := <-result
	if read.err != nil {
		t.Fatal(read.err)
	}
	if read.detail.Thread.OccurrenceCount != 1 || len(read.detail.Occurrences) != 1 || len(read.detail.Audits) != 0 {
		t.Fatalf("Get combined different commit snapshots: %#v", read.detail)
	}

	latest, err := fixture.repository.Get(context.Background(), first.Thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Thread.OccurrenceCount != 2 || len(latest.Occurrences) != 2 || len(latest.Audits) != 1 {
		t.Fatalf("latest committed detail = %#v", latest)
	}
}

func TestAlertRepositoryProductionConstructorRequiresStrongPurposeDerivedSecret(t *testing.T) {
	if _, err := alertpostgres.NewRepositoryWithCursorSecret(nil, "too-short"); err == nil {
		t.Fatal("NewRepositoryWithCursorSecret() error = nil for a weak secret")
	}
	repository, err := alertpostgres.NewRepositoryWithCursorSecret(nil, strings.Repeat("s", sha256.Size))
	if err != nil {
		t.Fatal(err)
	}
	if repository == nil {
		t.Fatal("NewRepositoryWithCursorSecret() repository = nil")
	}
}

type alertRepositoryFixture struct {
	runtime      *database.Runtime
	repository   *alertpostgres.Repository
	userID       int64
	monitorID    int64
	configID     int64
	eventID      int64
	updateID     int64
	configHash   string
	cursorSecret string
}

func newAlertRepositoryFixture(t *testing.T) *alertRepositoryFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		t.Fatal(err)
	}
	fixture := &alertRepositoryFixture{runtime: runtime, configHash: strings.Repeat("a", 64), cursorSecret: strings.Repeat("cursor-secret-", 3)}
	fixture.repository, err = alertpostgres.NewRepositoryWithCursorSecret(runtime, fixture.cursorSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO users (email,password_hash,display_name,role) VALUES ('alert@example.test','hash','Alert Tester','editor') RETURNING id`).Scan(&fixture.userID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO monitors (name,status,created_by,updated_by) VALUES ('Alert monitor','draft',$1,$1) RETURNING id`, fixture.userID).Scan(&fixture.monitorID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SQL.QueryRowContext(ctx, `INSERT INTO monitor_config_versions (monitor_id,revision,state,languages,event_threshold,created_by,updated_by) VALUES ($1,7,'draft',ARRAY['en'],75,$2,$2) RETURNING id`, fixture.monitorID, fixture.userID).Scan(&fixture.configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE monitor_config_versions SET state='published',config_hash=$1,published_at=$2 WHERE id=$3`, fixture.configHash, time.Now().UTC(), fixture.configID); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SQL.ExecContext(ctx, `UPDATE monitors SET status='active',published_config_version_id=$1 WHERE id=$2`, fixture.configID, fixture.monitorID); err != nil {
		t.Fatal(err)
	}
	fixture.eventID, fixture.updateID = fixture.insertEventAndUpdate(t)
	return fixture
}

func rewriteAlertCursorWithoutSigning(t *testing.T, cursor, field string, value any) string {
	t.Helper()
	parts := strings.Split(cursor, ".")
	if len(parts) != 2 {
		t.Fatalf("cursor parts = %d, want signed payload", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	claims[field] = value
	payload, err = json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + parts[1]
}

func resignAlertCursor(t *testing.T, cursor, secret, field string, value any) string {
	t.Helper()
	unsigned := rewriteAlertCursorWithoutSigning(t, cursor, field, value)
	parts := strings.Split(unsigned, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	derive := hmac.New(sha256.New, []byte(secret))
	_, _ = derive.Write([]byte("alert-cursor-v1"))
	sign := hmac.New(sha256.New, derive.Sum(nil))
	_, _ = sign.Write(payload)
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(sign.Sum(nil))
}

func waitForBlockedOccurrenceRead(database *sql.DB, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var blocked bool
		err := database.QueryRowContext(context.Background(), `
SELECT EXISTS (
    SELECT 1 FROM pg_locks
    WHERE relation='alert_occurrences'::regclass AND mode='AccessShareLock' AND NOT granted
)`).Scan(&blocked)
		if err == nil && blocked {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (fixture *alertRepositoryFixture) insertEventAndUpdate(t *testing.T) (int64, int64) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	var eventID int64
	key := fmt.Sprintf("alert-event-%d", now.UnixNano())
	if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `INSERT INTO events (event_key,title_zh,lifecycle_status,first_seen_at,last_seen_at) VALUES ($1,'Alert event','active',$2,$2) RETURNING id`, key, now).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	return eventID, fixture.insertUpdateForEvent(t, eventID, "rising", 1)
}

func (fixture *alertRepositoryFixture) insertUpdate(t *testing.T, kind string, sequence int64) int64 {
	t.Helper()
	return fixture.insertUpdateForEvent(t, fixture.eventID, kind, sequence)
}

func (fixture *alertRepositoryFixture) insertUpdateForEvent(t *testing.T, eventID int64, kind string, sequence int64) int64 {
	t.Helper()
	observedAt := time.Now().UTC().Truncate(time.Microsecond).Add(time.Duration(sequence) * time.Second)
	var updateID int64
	err := fixture.runtime.SQL.QueryRowContext(context.Background(), `
INSERT INTO event_updates (event_id,sequence_no,kind,summary,observed_at,reason_codes,before_state,after_state,evidence_set_hash,idempotency_key)
VALUES ($1,$2,$3,'deterministic update',$4,ARRAY['heat_changed'],'{}'::jsonb,'{"heat":80,"trend":20,"trend_status":"rising","source_count":2,"content_count":3,"window_end":"2026-08-04T07:00:00Z","heat_version":"heat-v1","evidence_version":"evidence-v1"}'::jsonb,$5,$6)
RETURNING id`, eventID, sequence, kind, observedAt, strings.Repeat("e", 64), fmt.Sprintf("%064x", eventID*1000+sequence)).Scan(&updateID)
	if err != nil {
		t.Fatal(err)
	}
	return updateID
}

func (fixture *alertRepositoryFixture) command(updateID int64, triggeredAt time.Time) domain.RecordOccurrenceCommand {
	fingerprint, _ := domain.OccurrenceFingerprint(domain.FingerprintInput{MonitorConfigVersionID: fixture.configID, EventUpdateID: updateID, TriggerType: domain.TriggerRising, PolicyVersion: domain.PolicyVersionV1})
	return domain.RecordOccurrenceCommand{
		MonitorID: fixture.monitorID, EventID: fixture.eventID, EventUpdateID: updateID,
		TriggerType: domain.TriggerRising, PolicyVersion: domain.PolicyVersionV1,
		MonitorConfigVersionID: fixture.configID, MonitorRevision: 7, MonitorConfigHash: fixture.configHash,
		EventThresholdSnapshot: 75, FinalScoreSnapshot: 80, Severity: domain.SeverityWarning,
		TitleSnapshot: "Alert event", ReasonSnapshot: "heat entered rising", TriggeredAt: triggeredAt, Fingerprint: fingerprint,
	}
}

func assertAlertCounts(t *testing.T, fixture *alertRepositoryFixture, threads, occurrences, occurrenceCount int) {
	t.Helper()
	var gotThreads, gotOccurrences, gotOccurrenceCount int
	if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `SELECT count(*) FROM alert_threads`).Scan(&gotThreads); err != nil {
		t.Fatal(err)
	}
	if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `SELECT count(*) FROM alert_occurrences`).Scan(&gotOccurrences); err != nil {
		t.Fatal(err)
	}
	if err := fixture.runtime.SQL.QueryRowContext(context.Background(), `SELECT COALESCE(sum(occurrence_count),0) FROM alert_threads`).Scan(&gotOccurrenceCount); err != nil {
		t.Fatal(err)
	}
	if gotThreads != threads || gotOccurrences != occurrences || gotOccurrenceCount != occurrenceCount {
		t.Fatalf("thread/occurrence/count = %d/%d/%d, want %d/%d/%d", gotThreads, gotOccurrences, gotOccurrenceCount, threads, occurrences, occurrenceCount)
	}
}
