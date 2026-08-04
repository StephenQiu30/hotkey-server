//go:build integration

package postgres_test

import (
	"context"
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

	detail, err := fixture.repository.Get(context.Background(), first.Thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Occurrences) != 1 || len(detail.Audits) != 1 || detail.Audits[0].ActorType != domain.ActorUser || detail.Audits[0].ReasonCode != "reviewed" {
		t.Fatalf("detail = %#v", detail)
	}
}

type alertRepositoryFixture struct {
	runtime    *database.Runtime
	repository *alertpostgres.Repository
	userID     int64
	monitorID  int64
	configID   int64
	eventID    int64
	updateID   int64
	configHash string
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
	fixture := &alertRepositoryFixture{runtime: runtime, repository: alertpostgres.NewRepository(runtime), configHash: strings.Repeat("a", 64)}
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
