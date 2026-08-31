// Package postgres owns Alert persistence. It never reads Event or Monitor
// business tables except through foreign keys; eligibility is supplied by the
// application layer's narrow cross-module ports.
package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/alert/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const threadColumns = `id, version, monitor_id, event_id, trigger_type, policy_version,
       monitor_config_version_id, monitor_revision, monitor_config_hash, event_threshold_snapshot,
       alert_min_heat_snapshot, alert_min_momentum_snapshot, alert_min_breadth_snapshot,
       alert_warning_threshold_snapshot, alert_critical_threshold_snapshot, alert_cooldown_minutes_snapshot,
       state, severity, title_snapshot, reason_snapshot, first_triggered_at, last_triggered_at,
       occurrence_count, cooldown_until, acknowledged_at, acknowledged_by_user_id,
       resolved_at, resolved_by_user_id, suppressed_at, suppressed_by_user_id, created_at, updated_at`

const occurrenceColumns = `id, alert_thread_id, event_update_id, severity, final_score_snapshot,
       threshold_snapshot, heat_score_snapshot, momentum_score_snapshot, breadth_score_snapshot,
       array_to_json(reason_codes), fingerprint, triggered_at, created_at`

const (
	alertCursorContext = "alert-cursor-v1"
	alertCursorVersion = 1
	alertCursorTTL     = 15 * time.Minute
)

type Repository struct {
	runtime   *database.Runtime
	cursorKey []byte
}

// NewRepository keeps the infrastructure-friendly constructor and derives a
// purpose-scoped cursor key from the runtime's parsed connection string. The
// derived key is never serialized or returned to callers.
func NewRepository(runtime *database.Runtime) *Repository {
	var connectionSecret string
	if runtime != nil && runtime.Pool != nil {
		connectionSecret = runtime.Pool.Config().ConnString()
	}
	return newRepository(runtime, connectionSecret)
}

// NewRepositoryWithCursorSecret is the explicit production constructor. The
// caller supplies process secret material; this adapter derives an independent
// alert-cursor-v1 key so the source secret is never used directly as a MAC key.
func NewRepositoryWithCursorSecret(runtime *database.Runtime, secret string) (*Repository, error) {
	if len([]byte(strings.TrimSpace(secret))) < sha256.Size {
		return nil, fmt.Errorf("alert cursor secret must be at least %d bytes", sha256.Size)
	}
	return newRepository(runtime, secret), nil
}

func newRepository(runtime *database.Runtime, secret string) *Repository {
	var key []byte
	if secret != "" {
		derive := hmac.New(sha256.New, []byte(secret))
		_, _ = derive.Write([]byte(alertCursorContext))
		key = derive.Sum(nil)
	}
	return &Repository{runtime: runtime, cursorKey: key}
}

func (repository *Repository) RecordOccurrence(ctx context.Context, command domain.RecordOccurrenceCommand) (domain.RecordOccurrenceResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.RecordOccurrenceResult{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.RecordOccurrenceResult{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	var result domain.RecordOccurrenceResult
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		insertedThread, err := insertOrFindThread(ctx, transaction.SQL, command)
		if err != nil {
			return err
		}
		thread, err := lockThreadByIdentity(ctx, transaction.SQL, command)
		if err != nil {
			return err
		}
		if err := frozenIdentityMatches(thread, command); err != nil {
			return err
		}

		occurrence, created, err := insertOccurrence(ctx, transaction.SQL, thread.ID, command)
		if err != nil {
			return err
		}
		if !created {
			result = domain.RecordOccurrenceResult{Thread: thread, Occurrence: occurrence}
			return nil
		}

		advancesLatest, err := occurrenceAdvancesLatest(ctx, transaction.SQL, thread.ID, occurrence.ID)
		if err != nil {
			return err
		}
		priorState := thread.State
		reopened := advancesLatest && domain.ShouldReopen(priorState, thread.CooldownUntil, command.TriggeredAt)
		disturb := insertedThread || advancesLatest && priorState != domain.StateSuppressed && !command.TriggeredAt.UTC().Before(thread.CooldownUntil.UTC())
		thread = applyOccurrence(thread, command, insertedThread, advancesLatest, reopened, time.Now().UTC())
		if err := updateThread(ctx, transaction.SQL, thread); err != nil {
			return err
		}
		if reopened {
			if err := insertStateAudit(ctx, transaction.SQL, domain.StateAudit{
				ThreadID: thread.ID, ActorType: domain.ActorSystem, FromState: priorState, ToState: domain.StateOpen,
				ExpectedVersion: thread.Version - 1, ReasonCode: "cooldown_elapsed_new_occurrence", CreatedAt: command.TriggeredAt.UTC(),
			}); err != nil {
				return err
			}
		}
		result = domain.RecordOccurrenceResult{Thread: thread, Occurrence: occurrence, Created: true, Reopened: reopened, Disturb: disturb}
		return nil
	})
	return result, err
}

func occurrenceAdvancesLatest(ctx context.Context, transaction *sql.Tx, threadID, occurrenceID int64) (bool, error) {
	var latestID int64
	err := transaction.QueryRowContext(ctx, `
SELECT id FROM alert_occurrences
WHERE alert_thread_id=$1
ORDER BY triggered_at DESC,event_update_id DESC
LIMIT 1`, threadID).Scan(&latestID)
	if err != nil {
		return false, mapDatabaseError(err)
	}
	return latestID == occurrenceID, nil
}

func insertOrFindThread(ctx context.Context, transaction *sql.Tx, command domain.RecordOccurrenceCommand) (bool, error) {
	var id int64
	cooldownMinutes := command.AlertCooldownMinutesSnapshot
	if cooldownMinutes == 0 {
		cooldownMinutes = 60
	}
	err := transaction.QueryRowContext(ctx, `
INSERT INTO alert_threads (
    monitor_id, monitor_config_version_id, monitor_revision, monitor_config_hash,
    event_id, trigger_type, policy_version, state, severity, event_threshold_snapshot,
    alert_min_heat_snapshot, alert_min_momentum_snapshot, alert_min_breadth_snapshot,
    alert_warning_threshold_snapshot, alert_critical_threshold_snapshot, alert_cooldown_minutes_snapshot,
    title_snapshot, reason_snapshot, first_triggered_at, last_triggered_at,
    occurrence_count, cooldown_until
) VALUES ($1,$2,$3,$4,$5,$6,$7,'open',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18,0,$19)
ON CONFLICT (monitor_config_version_id,event_id,trigger_type,policy_version) DO NOTHING
RETURNING id`, command.MonitorID, command.MonitorConfigVersionID, command.MonitorRevision, command.MonitorConfigHash,
		command.EventID, command.TriggerType, command.PolicyVersion, command.Severity, command.EventThresholdSnapshot,
		command.AlertMinHeatSnapshot, command.AlertMinMomentumSnapshot, command.AlertMinBreadthSnapshot,
		command.AlertWarningThresholdSnapshot, command.AlertCriticalThresholdSnapshot, cooldownMinutes,
		command.TitleSnapshot, command.ReasonSnapshot, command.TriggeredAt.UTC(), domain.CooldownUntilMinutes(command.TriggeredAt, cooldownMinutes)).Scan(&id)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, databaserepository.MapError(err)
}

func lockThreadByIdentity(ctx context.Context, transaction *sql.Tx, command domain.RecordOccurrenceCommand) (domain.Thread, error) {
	thread, err := scanThread(transaction.QueryRowContext(ctx, `SELECT `+threadColumns+`
FROM alert_threads
WHERE monitor_config_version_id=$1 AND event_id=$2 AND trigger_type=$3 AND policy_version=$4
FOR UPDATE`, command.MonitorConfigVersionID, command.EventID, command.TriggerType, command.PolicyVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Thread{}, sharedrepository.ErrNotFound
	}
	return thread, mapDatabaseError(err)
}

func frozenIdentityMatches(thread domain.Thread, command domain.RecordOccurrenceCommand) error {
	if thread.MonitorID != command.MonitorID || thread.EventID != command.EventID || thread.MonitorConfigVersionID != command.MonitorConfigVersionID ||
		thread.MonitorRevision != command.MonitorRevision || thread.MonitorConfigHash != command.MonitorConfigHash || thread.TriggerType != command.TriggerType ||
		thread.PolicyVersion != command.PolicyVersion || thread.EventThresholdSnapshot != command.EventThresholdSnapshot {
		return fmt.Errorf("%w: alert thread frozen policy differs", sharedrepository.ErrConflict)
	}
	if command.PolicyVersion == domain.PolicyVersionV2 && (thread.AlertMinHeatSnapshot != command.AlertMinHeatSnapshot || thread.AlertMinMomentumSnapshot != command.AlertMinMomentumSnapshot ||
		thread.AlertMinBreadthSnapshot != command.AlertMinBreadthSnapshot || thread.AlertWarningThresholdSnapshot != command.AlertWarningThresholdSnapshot ||
		thread.AlertCriticalThresholdSnapshot != command.AlertCriticalThresholdSnapshot || thread.AlertCooldownMinutesSnapshot != command.AlertCooldownMinutesSnapshot) {
		return fmt.Errorf("%w: alert thread frozen policy differs", sharedrepository.ErrConflict)
	}
	return nil
}

func insertOccurrence(ctx context.Context, transaction *sql.Tx, threadID int64, command domain.RecordOccurrenceCommand) (domain.Occurrence, bool, error) {
	reasons, err := json.Marshal(command.ReasonCodes)
	if err != nil {
		return domain.Occurrence{}, false, fmt.Errorf("encode occurrence reasons: %w", err)
	}
	if string(reasons) == "null" {
		reasons = []byte(`[]`)
	}
	occurrence, err := scanOccurrence(transaction.QueryRowContext(ctx, `
INSERT INTO alert_occurrences (
    alert_thread_id,event_update_id,severity,final_score_snapshot,threshold_snapshot,
    heat_score_snapshot,momentum_score_snapshot,breadth_score_snapshot,
    reason_codes,fingerprint,triggered_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,ARRAY(SELECT jsonb_array_elements_text($9::jsonb)),$10,$11)
ON CONFLICT DO NOTHING
RETURNING `+occurrenceColumns, threadID, command.EventUpdateID, command.Severity, command.FinalScoreSnapshot,
		command.EventThresholdSnapshot, command.HeatScoreSnapshot, command.MomentumScoreSnapshot, command.BreadthScoreSnapshot,
		reasons, command.Fingerprint, command.TriggeredAt.UTC()))
	if err == nil {
		return occurrence, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Occurrence{}, false, mapDatabaseError(err)
	}
	occurrence, err = scanOccurrence(transaction.QueryRowContext(ctx, `SELECT `+occurrenceColumns+`
FROM alert_occurrences WHERE fingerprint=$1`, command.Fingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Occurrence{}, false, fmt.Errorf("%w: conflicting occurrence identity", sharedrepository.ErrConflict)
	}
	if err != nil {
		return domain.Occurrence{}, false, mapDatabaseError(err)
	}
	if occurrence.ThreadID != threadID || occurrence.EventUpdateID != command.EventUpdateID {
		return domain.Occurrence{}, false, fmt.Errorf("%w: occurrence fingerprint collision", sharedrepository.ErrConflict)
	}
	return occurrence, false, nil
}

func applyOccurrence(thread domain.Thread, command domain.RecordOccurrenceCommand, insertedThread, advancesLatest, reopened bool, writtenAt time.Time) domain.Thread {
	priorVersion := thread.Version
	thread.OccurrenceCount++
	if advancesLatest {
		thread.Severity = command.Severity
		thread.TitleSnapshot = command.TitleSnapshot
		thread.ReasonSnapshot = command.ReasonSnapshot
		thread.LastTriggeredAt = command.TriggeredAt.UTC()
		cooldownMinutes := command.AlertCooldownMinutesSnapshot
		if cooldownMinutes == 0 {
			cooldownMinutes = 60
		}
		newCooldown := domain.CooldownUntilMinutes(command.TriggeredAt, cooldownMinutes)
		if newCooldown.After(thread.CooldownUntil) {
			thread.CooldownUntil = newCooldown
		}
	}
	if writtenAt.After(thread.UpdatedAt) {
		thread.UpdatedAt = writtenAt.UTC()
	}
	if !insertedThread || thread.OccurrenceCount > 1 {
		thread.Version = priorVersion + 1
	}
	if reopened {
		thread.State = domain.StateOpen
		thread.AcknowledgedAt, thread.AcknowledgedByUserID = nil, nil
		thread.ResolvedAt, thread.ResolvedByUserID = nil, nil
	}
	return thread
}

func updateThread(ctx context.Context, transaction *sql.Tx, thread domain.Thread) error {
	result, err := transaction.ExecContext(ctx, `
UPDATE alert_threads SET version=$1,state=$2,severity=$3,title_snapshot=$4,reason_snapshot=$5,
    last_triggered_at=$6,occurrence_count=$7,cooldown_until=$8,
    acknowledged_at=$9,acknowledged_by_user_id=$10,resolved_at=$11,resolved_by_user_id=$12,
    suppressed_at=$13,suppressed_by_user_id=$14,updated_at=$15
WHERE id=$16`, thread.Version, thread.State, thread.Severity, thread.TitleSnapshot, thread.ReasonSnapshot,
		thread.LastTriggeredAt, thread.OccurrenceCount, thread.CooldownUntil,
		nullableTime(thread.AcknowledgedAt), nullableInt64(thread.AcknowledgedByUserID), nullableTime(thread.ResolvedAt), nullableInt64(thread.ResolvedByUserID),
		nullableTime(thread.SuppressedAt), nullableInt64(thread.SuppressedByUserID), thread.UpdatedAt, thread.ID)
	if err != nil {
		return databaserepository.MapError(err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return sharedrepository.ErrConflict
	}
	return nil
}

func (repository *Repository) Transition(ctx context.Context, command domain.TransitionCommand) (domain.Thread, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.Thread{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.Thread{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	var changed domain.Thread
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		thread, err := scanThread(transaction.SQL.QueryRowContext(ctx, `SELECT `+threadColumns+` FROM alert_threads WHERE id=$1 FOR UPDATE`, command.ThreadID))
		if errors.Is(err, sql.ErrNoRows) {
			return sharedrepository.ErrNotFound
		}
		if err != nil {
			return mapDatabaseError(err)
		}
		if thread.Version != command.ExpectedVersion || !domain.CanUserTransition(thread.State, command.To) {
			return fmt.Errorf("%w: alert state or version conflict", sharedrepository.ErrConflict)
		}
		from := thread.State
		thread.State, thread.Version, thread.UpdatedAt = command.To, thread.Version+1, command.At.UTC()
		switch command.To {
		case domain.StateAcknowledged:
			thread.AcknowledgedAt, thread.AcknowledgedByUserID = timePointer(command.At), int64Pointer(command.ActorUserID)
		case domain.StateResolved:
			thread.ResolvedAt, thread.ResolvedByUserID = timePointer(command.At), int64Pointer(command.ActorUserID)
		case domain.StateSuppressed:
			thread.SuppressedAt, thread.SuppressedByUserID = timePointer(command.At), int64Pointer(command.ActorUserID)
		}
		if err := updateThread(ctx, transaction.SQL, thread); err != nil {
			return err
		}
		if err := insertStateAudit(ctx, transaction.SQL, domain.StateAudit{
			ThreadID: thread.ID, ActorType: domain.ActorUser, ActorUserID: int64Pointer(command.ActorUserID),
			FromState: from, ToState: command.To, ExpectedVersion: command.ExpectedVersion,
			ReasonCode: command.ReasonCode, CreatedAt: command.At.UTC(),
		}); err != nil {
			return err
		}
		changed = thread
		return nil
	})
	return changed, err
}

func insertStateAudit(ctx context.Context, transaction *sql.Tx, audit domain.StateAudit) error {
	_, err := transaction.ExecContext(ctx, `
INSERT INTO alert_state_audits (
    alert_thread_id,actor_type,actor_user_id,from_state,to_state,expected_version,reason_code,created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, audit.ThreadID, audit.ActorType, nullableInt64(audit.ActorUserID), audit.FromState, audit.ToState,
		audit.ExpectedVersion, audit.ReasonCode, audit.CreatedAt.UTC())
	return databaserepository.MapError(err)
}

func (repository *Repository) List(ctx context.Context, query domain.ListQuery) (domain.ThreadPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.ThreadPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.ThreadPage{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	shape := listShape(query)
	cursor, err := decodeListCursor(query.Cursor, shape, repository.cursorKey, time.Now().UTC())
	if err != nil {
		return domain.ThreadPage{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	state, severity, monitorID := "", "", int64(0)
	if query.State != nil {
		state = string(*query.State)
	}
	if query.Severity != nil {
		severity = string(*query.Severity)
	}
	if query.MonitorID != nil {
		monitorID = *query.MonitorID
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `SELECT `+threadColumns+`
FROM alert_threads
WHERE ($1='' OR state=$1) AND ($2='' OR severity=$2) AND ($3=0 OR monitor_id=$3)
  AND (NOT $4 OR (last_triggered_at,id) < ($5,$6))
ORDER BY last_triggered_at DESC,id DESC
LIMIT $7`, state, severity, monitorID, cursor.Present, cursor.LastTriggeredAt, cursor.ID, query.Limit+1)
	if err != nil {
		return domain.ThreadPage{}, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]domain.Thread, 0, query.Limit+1)
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return domain.ThreadPage{}, mapDatabaseError(err)
		}
		items = append(items, thread)
	}
	if err := rows.Err(); err != nil {
		return domain.ThreadPage{}, databaserepository.MapError(err)
	}
	page := domain.ThreadPage{Items: items}
	if len(items) > query.Limit {
		boundary := items[query.Limit-1]
		page.Items = items[:query.Limit]
		page.NextCursor, err = encodeListCursor(shape, boundary.LastTriggeredAt, boundary.ID, repository.cursorKey, time.Now().UTC())
		if err != nil {
			return domain.ThreadPage{}, err
		}
	}
	return page, nil
}

func (repository *Repository) Get(ctx context.Context, threadID int64) (domain.ThreadDetail, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.ThreadDetail{}, sharedrepository.ErrUnavailable
	}
	if threadID <= 0 {
		return domain.ThreadDetail{}, fmt.Errorf("%w: alert thread id is required", sharedrepository.ErrInvalidInput)
	}
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return readThreadDetail(ctx, transaction.SQL, threadID)
	}
	transaction, err := repository.runtime.SQL.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.ThreadDetail{}, mapDatabaseError(err)
	}
	defer func() { _ = transaction.Rollback() }()
	detail, err := readThreadDetail(ctx, transaction, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	if err := transaction.Commit(); err != nil {
		return domain.ThreadDetail{}, mapDatabaseError(err)
	}
	return detail, nil
}

type detailReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readThreadDetail(ctx context.Context, reader detailReader, threadID int64) (domain.ThreadDetail, error) {
	thread, err := scanThread(reader.QueryRowContext(ctx, `SELECT `+threadColumns+` FROM alert_threads WHERE id=$1`, threadID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ThreadDetail{}, sharedrepository.ErrNotFound
	}
	if err != nil {
		return domain.ThreadDetail{}, mapDatabaseError(err)
	}
	detail := domain.ThreadDetail{Thread: thread, Occurrences: []domain.Occurrence{}, Audits: []domain.StateAudit{}, EmailDeliveries: []domain.EmailDelivery{}}
	rows, err := reader.QueryContext(ctx, `SELECT `+occurrenceColumns+`
FROM alert_occurrences WHERE alert_thread_id=$1 ORDER BY triggered_at DESC,id DESC LIMIT 50`, threadID)
	if err != nil {
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	for rows.Next() {
		occurrence, err := scanOccurrence(rows)
		if err != nil {
			_ = rows.Close()
			return domain.ThreadDetail{}, mapDatabaseError(err)
		}
		detail.Occurrences = append(detail.Occurrences, occurrence)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	_ = rows.Close()
	deliveries, err := reader.QueryContext(ctx, `
SELECT delivery.id,delivery.occurrence_id,delivery.severity,delivery.status,
       (SELECT count(*) FROM alert_email_attempts attempt WHERE attempt.delivery_id=delivery.id AND attempt.status='started'),
       delivery.next_attempt_at,delivery.succeeded_at,coalesce(delivery.last_error,'')
FROM alert_email_deliveries delivery
JOIN alert_occurrences occurrence ON occurrence.id=delivery.occurrence_id
WHERE occurrence.alert_thread_id=$1
ORDER BY occurrence.triggered_at DESC,delivery.id DESC`, threadID)
	if err != nil {
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	for deliveries.Next() {
		var delivery domain.EmailDelivery
		var severity string
		var next, succeeded sql.NullTime
		if err := deliveries.Scan(&delivery.ID, &delivery.OccurrenceID, &severity, &delivery.Status, &delivery.AttemptCount, &next, &succeeded, &delivery.LastError); err != nil {
			_ = deliveries.Close()
			return domain.ThreadDetail{}, mapDatabaseError(err)
		}
		delivery.Severity = domain.Severity(severity)
		delivery.NextAttemptAt, delivery.SucceededAt = nullTimePointer(next), nullTimePointer(succeeded)
		detail.EmailDeliveries = append(detail.EmailDeliveries, delivery)
	}
	if err := deliveries.Err(); err != nil {
		_ = deliveries.Close()
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	_ = deliveries.Close()

	audits, err := reader.QueryContext(ctx, `
SELECT id,alert_thread_id,actor_type,actor_user_id,from_state,to_state,expected_version,reason_code,created_at
FROM alert_state_audits WHERE alert_thread_id=$1 ORDER BY created_at ASC,id ASC`, threadID)
	if err != nil {
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	defer func() { _ = audits.Close() }()
	for audits.Next() {
		audit, err := scanAudit(audits)
		if err != nil {
			return domain.ThreadDetail{}, mapDatabaseError(err)
		}
		detail.Audits = append(detail.Audits, audit)
	}
	if err := audits.Err(); err != nil {
		return domain.ThreadDetail{}, databaserepository.MapError(err)
	}
	return detail, nil
}

type rowScanner interface{ Scan(...any) error }

func scanThread(scanner rowScanner) (domain.Thread, error) {
	var thread domain.Thread
	var trigger, state, severity string
	var acknowledgedAt, resolvedAt, suppressedAt sql.NullTime
	var acknowledgedBy, resolvedBy, suppressedBy sql.NullInt64
	err := scanner.Scan(
		&thread.ID, &thread.Version, &thread.MonitorID, &thread.EventID, &trigger, &thread.PolicyVersion,
		&thread.MonitorConfigVersionID, &thread.MonitorRevision, &thread.MonitorConfigHash, &thread.EventThresholdSnapshot,
		&thread.AlertMinHeatSnapshot, &thread.AlertMinMomentumSnapshot, &thread.AlertMinBreadthSnapshot,
		&thread.AlertWarningThresholdSnapshot, &thread.AlertCriticalThresholdSnapshot, &thread.AlertCooldownMinutesSnapshot,
		&state, &severity, &thread.TitleSnapshot, &thread.ReasonSnapshot, &thread.FirstTriggeredAt, &thread.LastTriggeredAt,
		&thread.OccurrenceCount, &thread.CooldownUntil, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suppressedAt, &suppressedBy, &thread.CreatedAt, &thread.UpdatedAt,
	)
	if err != nil {
		return domain.Thread{}, err
	}
	thread.TriggerType, thread.State, thread.Severity = domain.TriggerType(trigger), domain.State(state), domain.Severity(severity)
	thread.AcknowledgedAt, thread.ResolvedAt, thread.SuppressedAt = nullTimePointer(acknowledgedAt), nullTimePointer(resolvedAt), nullTimePointer(suppressedAt)
	thread.AcknowledgedByUserID, thread.ResolvedByUserID, thread.SuppressedByUserID = nullInt64Pointer(acknowledgedBy), nullInt64Pointer(resolvedBy), nullInt64Pointer(suppressedBy)
	return thread, nil
}

func scanOccurrence(scanner rowScanner) (domain.Occurrence, error) {
	var occurrence domain.Occurrence
	var severity string
	var encodedReasons []byte
	err := scanner.Scan(&occurrence.ID, &occurrence.ThreadID, &occurrence.EventUpdateID, &severity,
		&occurrence.FinalScoreSnapshot, &occurrence.EventThresholdSnapshot, &occurrence.HeatScoreSnapshot, &occurrence.MomentumScoreSnapshot, &occurrence.BreadthScoreSnapshot, &encodedReasons,
		&occurrence.Fingerprint, &occurrence.TriggeredAt, &occurrence.CreatedAt)
	if err != nil {
		return domain.Occurrence{}, err
	}
	occurrence.Severity = domain.Severity(severity)
	occurrence.ReasonCodes = []string{}
	if len(encodedReasons) > 0 {
		if err := json.Unmarshal(encodedReasons, &occurrence.ReasonCodes); err != nil {
			return domain.Occurrence{}, fmt.Errorf("decode occurrence reasons: %w", err)
		}
	}
	return occurrence, nil
}

func scanAudit(scanner rowScanner) (domain.StateAudit, error) {
	var audit domain.StateAudit
	var actor, from, to string
	var actorUserID, expectedVersion sql.NullInt64
	err := scanner.Scan(&audit.ID, &audit.ThreadID, &actor, &actorUserID, &from, &to, &expectedVersion, &audit.ReasonCode, &audit.CreatedAt)
	if err != nil {
		return domain.StateAudit{}, err
	}
	audit.ActorType, audit.FromState, audit.ToState = domain.ActorType(actor), domain.State(from), domain.State(to)
	audit.ActorUserID = nullInt64Pointer(actorUserID)
	if expectedVersion.Valid {
		audit.ExpectedVersion = expectedVersion.Int64
	}
	return audit, nil
}

type listCursor struct {
	Version         int       `json:"v"`
	Shape           string    `json:"shape"`
	IssuedAt        time.Time `json:"issued_at"`
	LastTriggeredAt time.Time `json:"last_triggered_at"`
	ID              int64     `json:"id"`
	Present         bool      `json:"-"`
}

func listShape(query domain.ListQuery) string {
	state, severity, monitorID := "", "", int64(0)
	if query.State != nil {
		state = string(*query.State)
	}
	if query.Severity != nil {
		severity = string(*query.Severity)
	}
	if query.MonitorID != nil {
		monitorID = *query.MonitorID
	}
	payload, _ := json.Marshal(struct {
		State     string `json:"state"`
		Severity  string `json:"severity"`
		MonitorID int64  `json:"monitor_id"`
	}{state, severity, monitorID})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func encodeListCursor(shape string, lastTriggeredAt time.Time, id int64, key []byte, issuedAt time.Time) (string, error) {
	if shape == "" || lastTriggeredAt.IsZero() || id <= 0 || len(key) == 0 || issuedAt.IsZero() {
		return "", fmt.Errorf("invalid alert cursor facts")
	}
	payload, err := json.Marshal(listCursor{
		Version: alertCursorVersion, Shape: shape, IssuedAt: issuedAt.UTC(),
		LastTriggeredAt: lastTriggeredAt.UTC(), ID: id,
	})
	if err != nil {
		return "", err
	}
	signature := hmac.New(sha256.New, key)
	_, _ = signature.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}

func decodeListCursor(value, shape string, key []byte, now time.Time) (listCursor, error) {
	if value == "" {
		return listCursor{}, nil
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(key) == 0 || now.IsZero() {
		return listCursor{}, fmt.Errorf("invalid signed alert cursor")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return listCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return listCursor{}, fmt.Errorf("decode cursor signature: %w", err)
	}
	wantSignature := hmac.New(sha256.New, key)
	_, _ = wantSignature.Write(payload)
	if !hmac.Equal(providedSignature, wantSignature.Sum(nil)) {
		return listCursor{}, fmt.Errorf("alert cursor signature does not match")
	}
	var cursor listCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return listCursor{}, fmt.Errorf("parse cursor: %w", err)
	}
	now = now.UTC()
	if cursor.Version != alertCursorVersion || cursor.Shape != shape || cursor.IssuedAt.IsZero() || cursor.IssuedAt.After(now) ||
		!now.Before(cursor.IssuedAt.UTC().Add(alertCursorTTL)) || cursor.LastTriggeredAt.IsZero() || cursor.ID <= 0 {
		return listCursor{}, fmt.Errorf("cursor does not match alert query")
	}
	cursor.Present = true
	return cursor, nil
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func mapDatabaseError(err error) error {
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return databaserepository.MapError(err)
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return int64Pointer(value.Int64)
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return timePointer(value.Time)
}

func int64Pointer(value int64) *int64 { return &value }

func timePointer(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
