// Package postgres contains Event-owned persistence. Cross-module facts are
// consumed through bounded query methods; this adapter never calls services.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/shared/id"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type Repository struct {
	runtime *database.Runtime
	ids     id.Generator
}

type rowQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

var _ application.EventStore = (*Repository)(nil)
var _ application.GovernanceRepository = (*Repository)(nil)
var _ application.ReadRepository = (*Repository)(nil)
var _ application.DecisionStore = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository {
	return &Repository{runtime: runtime, ids: id.UUID{}}
}

func (repository *Repository) Get(ctx context.Context, eventID int64) (domain.Event, error) {
	if !repository.available() || eventID <= 0 {
		return domain.Event{}, sharedrepository.ErrUnavailable
	}
	var query rowQuery = repository.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		query = transaction.SQL
	}
	return scanEvent(query.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL`, eventID))
}

func (repository *Repository) List(ctx context.Context, query domain.EventListQuery) (domain.EventPage, error) {
	if !repository.available() || query.Limit < 1 || query.Limit > 100 || query.Cursor < 0 {
		return domain.EventPage{}, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE deleted_at IS NULL AND id > $1 AND lifecycle_status <> 'archived'
ORDER BY id ASC LIMIT $2`, query.Cursor, query.Limit+1)
	if err != nil {
		return domain.EventPage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.Event, 0, query.Limit)
	for rows.Next() {
		var event domain.Event
		var fingerprint, fingerprintVersion, titleEN string
		var representative, merged sql.NullInt64
		if err := rows.Scan(&event.ID, &event.Version, &event.EventKey, &fingerprint, &fingerprintVersion, &event.TitleZH, &titleEN, &event.Summary, &event.LifecycleStatus, &event.FirstSeenAt, &event.LastSeenAt, &representative, &merged, &event.ManualLocked); err != nil {
			return domain.EventPage{}, sharedrepository.MapError(err)
		}
		event.EventFingerprint, event.FingerprintVersion, event.TitleEN = fingerprint, fingerprintVersion, titleEN
		if representative.Valid {
			event.RepresentativeContentID = &representative.Int64
		}
		if merged.Valid {
			event.MergedIntoID = &merged.Int64
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return domain.EventPage{}, sharedrepository.MapError(err)
	}
	page := domain.EventPage{Items: items}
	if len(items) > query.Limit {
		page.NextCursor = items[query.Limit-1].ID
		page.Items = items[:query.Limit]
	}
	return page, nil
}

func (repository *Repository) ListMembers(ctx context.Context, eventID int64) (domain.EventMemberPage, error) {
	if !repository.available() || eventID <= 0 {
		return domain.EventMemberPage{}, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked
FROM event_contents WHERE event_id = $1 ORDER BY membership_score DESC, content_id ASC`, eventID)
	if err != nil {
		return domain.EventMemberPage{}, sharedrepository.MapError(err)
	}
	defer rows.Close()
	items := make([]domain.EventMember, 0)
	for rows.Next() {
		var member domain.EventMember
		var role, origin string
		if err := rows.Scan(&member.ID, &member.Version, &member.EventID, &member.ContentID, &member.MembershipScore, &role, &member.Representative, &origin, &member.ManualLocked); err != nil {
			return domain.EventMemberPage{}, sharedrepository.MapError(err)
		}
		member.EvidenceRole, member.Origin = domain.EvidenceRole(role), domain.MemberOrigin(origin)
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		return domain.EventMemberPage{}, sharedrepository.MapError(err)
	}
	return domain.EventMemberPage{Items: items}, nil
}

func (repository *Repository) Save(ctx context.Context, event domain.Event, expectedVersion int64, audit domain.GovernanceAudit) error {
	if !repository.available() || expectedVersion <= 0 {
		return sharedrepository.ErrUnavailable
	}
	return repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		result, err := transaction.SQL.ExecContext(ctx, `
UPDATE events SET lifecycle_status = $1, merged_into_id = $2, representative_content_id = $3,
       version = version + 1, updated_at = now()
WHERE id = $4 AND version = $5`, event.LifecycleStatus, nullableInt64(event.MergedIntoID), nullableInt64(event.RepresentativeContentID), event.ID, expectedVersion)
		if err != nil {
			return sharedrepository.MapError(err)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return fmt.Errorf("event version conflict")
		}
		audit.EventID = event.ID
		return insertAudit(ctx, transaction.SQL, audit)
	})
}

func (repository *Repository) Merge(ctx context.Context, command application.MergeCommand) (domain.Event, error) {
	if !repository.available() {
		return domain.Event{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, err
	}
	var target domain.Event
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		ids := []int64{command.SourceEventID, command.TargetEventID}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		locked := make(map[int64]domain.Event, 2)
		for _, eventID := range ids {
			event, err := scanEvent(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, eventID))
			if err != nil {
				return err
			}
			locked[eventID] = event
		}
		source := locked[command.SourceEventID]
		target = locked[command.TargetEventID]
		if source.Version != command.SourceExpectedVersion {
			return fmt.Errorf("event version conflict")
		}
		if source.ManualLocked || source.LifecycleStatus == domain.LifecycleMerged || source.LifecycleStatus == domain.LifecycleArchived || source.LifecycleStatus == domain.LifecycleRejected {
			return fmt.Errorf("event merge is locked or unavailable")
		}
		seen := map[int64]bool{target.ID: true}
		for target.MergedIntoID != nil {
			if seen[*target.MergedIntoID] {
				return fmt.Errorf("merge target contains a cycle")
			}
			seen[*target.MergedIntoID] = true
			canonical, err := scanEvent(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, *target.MergedIntoID))
			if err != nil {
				return err
			}
			target = canonical
		}
		if target.ID == source.ID || target.Version != command.TargetExpectedVersion || target.ManualLocked || target.LifecycleStatus == domain.LifecycleArchived || target.LifecycleStatus == domain.LifecycleRejected {
			return fmt.Errorf("event merge target is locked or version-conflicted")
		}
		if err := mergeMembers(ctx, transaction.SQL, source.ID, target.ID, command.ActorUserID); err != nil {
			return err
		}
		if err := mergeMonitorEvents(ctx, transaction.SQL, source.ID, target.ID); err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE events SET lifecycle_status = 'merged', merged_into_id = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, target.ID, source.ID, source.Version); err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE events SET version = version + 1, updated_at = now() WHERE id = $1 AND version = $2`, target.ID, target.Version); err != nil {
			return err
		}
		from, to := source.LifecycleStatus, domain.LifecycleMerged
		expected := source.Version
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: source.ID, Action: domain.AuditMerge, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode, FromStatus: &from, ToStatus: &to, SourceEventID: &source.ID, TargetEventID: &target.ID, ExpectedVersion: &expected}); err != nil {
			return err
		}
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: target.ID, Action: domain.AuditDownstreamReconcile, ActorUserID: command.ActorUserID, ReasonCode: "merge_downstream_reconcile", SourceEventID: &source.ID, TargetEventID: &target.ID}); err != nil {
			return err
		}
		target.Version++
		return nil
	})
	if err != nil {
		return domain.Event{}, err
	}
	return target, nil
}

func (repository *Repository) Split(ctx context.Context, command application.SplitCommand) (domain.Event, error) {
	if !repository.available() {
		return domain.Event{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, err
	}
	var created domain.Event
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		source, err := scanEvent(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, command.SourceEventID))
		if err != nil {
			return err
		}
		if source.Version != command.SourceExpectedVersion || source.ManualLocked || source.LifecycleStatus == domain.LifecycleMerged || source.LifecycleStatus == domain.LifecycleArchived || source.LifecycleStatus == domain.LifecycleRejected {
			return fmt.Errorf("event version conflict or source is locked")
		}
		members := make(map[int64]domain.EventMember, len(command.Members))
		for _, requested := range command.Members {
			member, err := scanMember(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked
FROM event_contents WHERE event_id = $1 AND content_id = $2 FOR UPDATE`, source.ID, requested.ContentID))
			if err != nil {
				return err
			}
			if member.Version != requested.ExpectedVersion || member.ManualLocked || member.EvidenceRole == domain.EvidenceDuplicate {
				return fmt.Errorf("member version conflict or member is locked")
			}
			members[requested.ContentID] = member
		}
		newKey := "evt_" + repository.ids.New()
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO events (event_key, title_zh, title_en, summary, lifecycle_status, first_seen_at, last_seen_at, representative_content_id)
VALUES ($1, $2, $3, $4, 'detected', $5, $6, NULL) RETURNING id, version, created_at, updated_at`, newKey, source.TitleZH, source.TitleEN, source.Summary, source.FirstSeenAt, source.LastSeenAt).Scan(&created.ID, &created.Version, &created.FirstSeenAt, &created.LastSeenAt); err != nil {
			return err
		}
		created.EventKey, created.TitleZH, created.TitleEN, created.Summary, created.LifecycleStatus = newKey, source.TitleZH, source.TitleEN, source.Summary, domain.LifecycleDetected
		created.FirstSeenAt, created.LastSeenAt = source.FirstSeenAt, source.LastSeenAt
		contentIDs := make([]int64, 0, len(members))
		for contentID, member := range members {
			if _, err := transaction.SQL.ExecContext(ctx, `UPDATE event_contents SET event_id = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, created.ID, member.ID, member.Version); err != nil {
				return err
			}
			contentIDs = append(contentIDs, contentID)
		}
		sort.Slice(contentIDs, func(i, j int) bool { return contentIDs[i] < contentIDs[j] })
		if err := copyMonitorEvents(ctx, transaction.SQL, source.ID, created.ID); err != nil {
			return err
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE events SET version = version + 1, updated_at = now() WHERE id = $1 AND version = $2`, source.ID, source.Version); err != nil {
			return err
		}
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: source.ID, Action: domain.AuditSplit, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode, SourceEventID: &source.ID, TargetEventID: &created.ID}); err != nil {
			return err
		}
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: created.ID, Action: domain.AuditDownstreamReconcile, ActorUserID: command.ActorUserID, ReasonCode: "split_downstream_reconcile", SourceEventID: &source.ID, TargetEventID: &created.ID, Metadata: map[string]any{"content_ids": contentIDs}}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domain.Event{}, err
	}
	return created, nil
}

func (repository *Repository) SetMemberLock(ctx context.Context, command application.MemberLockCommand) (domain.EventMember, error) {
	if !repository.available() {
		return domain.EventMember{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.EventMember{}, err
	}
	var updated domain.EventMember
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		member, err := scanMember(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked
FROM event_contents WHERE event_id = $1 AND content_id = $2 FOR UPDATE`, command.EventID, command.ContentID))
		if err != nil {
			return err
		}
		if member.Version != command.ExpectedVersion {
			return fmt.Errorf("member version conflict")
		}
		if _, err := transaction.SQL.ExecContext(ctx, `UPDATE event_contents SET manual_locked = $1, origin = 'user', version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, command.Locked, member.ID, command.ExpectedVersion); err != nil {
			return err
		}
		member.ManualLocked = command.Locked
		member.Origin = domain.MemberOriginUser
		member.Version++
		updated = member
		action := domain.AuditManualUnlock
		if command.Locked {
			action = domain.AuditManualLock
		}
		return insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: command.EventID, Action: action, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode})
	})
	return updated, err
}

func mergeMembers(ctx context.Context, query *sql.Tx, sourceID, targetID int64, actor *int64) error {
	rows, err := query.QueryContext(ctx, `SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked FROM event_contents WHERE event_id = $1 ORDER BY content_id FOR UPDATE`, sourceID)
	if err != nil {
		return err
	}
	members := make([]domain.EventMember, 0)
	for rows.Next() {
		var member domain.EventMember
		var role, origin string
		if err := rows.Scan(&member.ID, &member.Version, &member.EventID, &member.ContentID, &member.MembershipScore, &role, &member.Representative, &origin, &member.ManualLocked); err != nil {
			return err
		}
		member.EvidenceRole, member.Origin = domain.EvidenceRole(role), domain.MemberOrigin(origin)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, member := range members {
		if member.ManualLocked {
			return fmt.Errorf("locked member conflict")
		}
		var targetIDValue int64
		var targetScore float64
		var targetVersion int64
		err := query.QueryRowContext(ctx, `SELECT id, membership_score, version FROM event_contents WHERE event_id = $1 AND content_id = $2 FOR UPDATE`, targetID, member.ContentID).Scan(&targetIDValue, &targetScore, &targetVersion)
		if err == sql.ErrNoRows {
			if _, err := query.ExecContext(ctx, `UPDATE event_contents SET event_id = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, targetID, member.ID, member.Version); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if member.MembershipScore > targetScore {
			if _, err := query.ExecContext(ctx, `UPDATE event_contents SET membership_score = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, member.MembershipScore, targetIDValue, targetVersion); err != nil {
				return err
			}
		}
		if _, err := query.ExecContext(ctx, `DELETE FROM event_contents WHERE id = $1`, member.ID); err != nil {
			return err
		}
		if err := insertAudit(ctx, query, domain.GovernanceAudit{EventID: targetID, Action: domain.AuditDeduplicated, ActorUserID: actor, ReasonCode: "merge_duplicate_member"}); err != nil {
			return err
		}
	}
	return nil
}

func mergeMonitorEvents(ctx context.Context, query *sql.Tx, sourceID, targetID int64) error {
	rows, err := query.QueryContext(ctx, `SELECT monitor_id, relevance_score, final_score, first_matched_at, last_matched_at, status FROM monitor_events WHERE event_id = $1 ORDER BY monitor_id FOR UPDATE`, sourceID)
	if err != nil {
		return err
	}
	type monitorEvent struct {
		monitorID        int64
		relevance, final float64
		first, last      time.Time
		status           string
	}
	items := make([]monitorEvent, 0)
	for rows.Next() {
		var item monitorEvent
		if err := rows.Scan(&item.monitorID, &item.relevance, &item.final, &item.first, &item.last, &item.status); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := query.ExecContext(ctx, `
INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at, status)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (monitor_id, event_id) DO UPDATE SET relevance_score = GREATEST(monitor_events.relevance_score, EXCLUDED.relevance_score), final_score = GREATEST(monitor_events.final_score, EXCLUDED.final_score), first_matched_at = LEAST(monitor_events.first_matched_at, EXCLUDED.first_matched_at), last_matched_at = GREATEST(monitor_events.last_matched_at, EXCLUDED.last_matched_at), updated_at = now()`, item.monitorID, targetID, item.relevance, item.final, item.first, item.last, item.status); err != nil {
			return err
		}
	}
	_, err = query.ExecContext(ctx, `DELETE FROM monitor_events WHERE event_id = $1`, sourceID)
	return err
}

func copyMonitorEvents(ctx context.Context, query *sql.Tx, sourceID, targetID int64) error {
	_, err := query.ExecContext(ctx, `INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at, status) SELECT monitor_id, $2, relevance_score, final_score, first_matched_at, last_matched_at, status FROM monitor_events WHERE event_id = $1 ON CONFLICT DO NOTHING`, sourceID, targetID)
	return err
}

func insertAudit(ctx context.Context, query *sql.Tx, audit domain.GovernanceAudit) error {
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = time.Now().UTC()
	}
	if err := audit.Validate(); err != nil {
		return err
	}
	metadata := []byte(`{}`)
	if audit.Metadata != nil {
		encoded, err := json.Marshal(audit.Metadata)
		if err != nil {
			return err
		}
		metadata = encoded
	}
	_, err := query.ExecContext(ctx, `INSERT INTO event_governance_audits (event_id, action, actor_user_id, reason_code, from_status, to_status, source_event_id, target_event_id, expected_version, metadata, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, audit.EventID, audit.Action, nullableInt64(audit.ActorUserID), audit.ReasonCode, nullableStatus(audit.FromStatus), nullableStatus(audit.ToStatus), nullableInt64(audit.SourceEventID), nullableInt64(audit.TargetEventID), nullableInt64(audit.ExpectedVersion), metadata, audit.CreatedAt)
	return err
}

func scanEvent(row *sql.Row) (domain.Event, error) {
	var event domain.Event
	var fingerprint, fingerprintVersion, titleEN string
	var representative, merged sql.NullInt64
	if err := row.Scan(&event.ID, &event.Version, &event.EventKey, &fingerprint, &fingerprintVersion, &event.TitleZH, &titleEN, &event.Summary, &event.LifecycleStatus, &event.FirstSeenAt, &event.LastSeenAt, &representative, &merged, &event.ManualLocked); err != nil {
		return domain.Event{}, sharedrepository.MapError(err)
	}
	event.EventFingerprint, event.FingerprintVersion, event.TitleEN = fingerprint, fingerprintVersion, titleEN
	if representative.Valid {
		event.RepresentativeContentID = &representative.Int64
	}
	if merged.Valid {
		event.MergedIntoID = &merged.Int64
	}
	return event, nil
}

func scanMember(row *sql.Row) (domain.EventMember, error) {
	var member domain.EventMember
	var role, origin string
	if err := row.Scan(&member.ID, &member.Version, &member.EventID, &member.ContentID, &member.MembershipScore, &role, &member.Representative, &origin, &member.ManualLocked); err != nil {
		return domain.EventMember{}, sharedrepository.MapError(err)
	}
	member.EvidenceRole, member.Origin = domain.EvidenceRole(role), domain.MemberOrigin(origin)
	return member, nil
}

func (repository *Repository) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return fn(ctx, transaction)
	}
	return repository.runtime.WithinTransaction(ctx, fn)
}

func (repository *Repository) available() bool {
	return repository != nil && repository.runtime != nil && repository.runtime.SQL != nil
}
func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableStatus(value *domain.LifecycleStatus) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
