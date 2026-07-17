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
	if _, err := repository.Get(ctx, eventID); err != nil {
		return domain.EventMemberPage{}, err
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
			return fmt.Errorf("%w: event version conflict", sharedrepository.ErrConflict)
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
		// Discover the target chain without holding locks, then acquire the
		// complete set (including source) in stable event_key order. If the
		// chain changed before locking, resolution below returns a conflict
		// before any mutation; this avoids an out-of-order root lock.
		requestedTarget, err := readEvent(ctx, transaction.SQL, command.TargetEventID)
		if err != nil {
			return err
		}
		chain, err := readMergeTargetChain(ctx, transaction.SQL, requestedTarget)
		if err != nil {
			return err
		}
		sourcePreview, err := readEvent(ctx, transaction.SQL, command.SourceEventID)
		if err != nil {
			return err
		}
		toLock := append(chain, sourcePreview)
		locked, err := lockEventsByStableKey(ctx, transaction.SQL, toLock)
		if err != nil {
			return err
		}
		source := locked[command.SourceEventID]
		target = locked[command.TargetEventID]
		if source.ManualLocked || source.LifecycleStatus == domain.LifecycleMerged || source.LifecycleStatus == domain.LifecycleArchived || source.LifecycleStatus == domain.LifecycleRejected {
			return fmt.Errorf("%w: event merge is locked or unavailable", sharedrepository.ErrConflict)
		}
		seen := make(map[int64]bool, len(locked))
		for target.MergedIntoID != nil {
			if seen[target.ID] {
				return fmt.Errorf("%w: merge target contains a cycle", sharedrepository.ErrConflict)
			}
			seen[target.ID] = true
			canonical, found := locked[*target.MergedIntoID]
			if !found {
				return fmt.Errorf("%w: merge target changed while acquiring locks", sharedrepository.ErrConflict)
			}
			target = canonical
		}
		if source.Version != command.SourceExpectedVersion || target.Version != command.TargetExpectedVersion {
			return fmt.Errorf("%w: event version conflict", sharedrepository.ErrConflict)
		}
		if target.ID == source.ID || target.ManualLocked || target.LifecycleStatus == domain.LifecycleArchived || target.LifecycleStatus == domain.LifecycleRejected || target.LifecycleStatus == domain.LifecycleMerged {
			return fmt.Errorf("%w: event merge target is locked or version-conflicted", sharedrepository.ErrConflict)
		}
		if err := mergeMembers(ctx, transaction.SQL, source.ID, target.ID, command.ActorUserID); err != nil {
			return err
		}
		if err := mergeMonitorEvents(ctx, transaction.SQL, source.ID, target.ID, command.ActorUserID); err != nil {
			return err
		}
		result, err := transaction.SQL.ExecContext(ctx, `UPDATE events SET lifecycle_status = 'merged', merged_into_id = $1, representative_content_id = NULL, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, target.ID, source.ID, source.Version)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return fmt.Errorf("%w: source event version conflict", sharedrepository.ErrConflict)
		}
		target, err = repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: target.ID, ReasonCode: "merge_evidence_recompute", ActorUserID: command.ActorUserID})
		if err != nil {
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
		return nil
	})
	if err != nil {
		return domain.Event{}, err
	}
	return target, nil
}

func readEvent(ctx context.Context, query rowQuery, eventID int64) (domain.Event, error) {
	return scanEvent(query.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL`, eventID))
}

func readMergeTargetChain(ctx context.Context, query rowQuery, first domain.Event) ([]domain.Event, error) {
	chain := []domain.Event{first}
	seen := map[int64]bool{first.ID: true}
	current := first
	for current.MergedIntoID != nil {
		nextID := *current.MergedIntoID
		if seen[nextID] {
			return nil, fmt.Errorf("%w: merge target contains a cycle", sharedrepository.ErrConflict)
		}
		next, err := readEvent(ctx, query, nextID)
		if err != nil {
			return nil, err
		}
		chain = append(chain, next)
		seen[next.ID] = true
		current = next
	}
	return chain, nil
}

func lockEventsByStableKey(ctx context.Context, query *sql.Tx, events []domain.Event) (map[int64]domain.Event, error) {
	unique := make(map[int64]domain.Event, len(events))
	for _, event := range events {
		unique[event.ID] = event
	}
	ordered := make([]domain.Event, 0, len(unique))
	for _, event := range unique {
		ordered = append(ordered, event)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].EventKey < ordered[right].EventKey })
	locked := make(map[int64]domain.Event, len(ordered))
	for _, event := range ordered {
		current, err := scanEvent(query.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, event.ID))
		if err != nil {
			return nil, err
		}
		locked[current.ID] = current
	}
	return locked, nil
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
			return fmt.Errorf("%w: event version conflict or source is locked", sharedrepository.ErrConflict)
		}
		requestedMembers := append([]application.SplitMember(nil), command.Members...)
		sort.Slice(requestedMembers, func(left, right int) bool {
			return requestedMembers[left].ContentID < requestedMembers[right].ContentID
		})
		members := make([]domain.EventMember, 0, len(requestedMembers))
		for _, requested := range requestedMembers {
			member, err := scanMember(transaction.SQL.QueryRowContext(ctx, `
SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked
FROM event_contents WHERE event_id = $1 AND content_id = $2 FOR UPDATE`, source.ID, requested.ContentID))
			if err != nil {
				return err
			}
			if member.Version != requested.ExpectedVersion || member.ManualLocked || member.EvidenceRole == domain.EvidenceDuplicate {
				return fmt.Errorf("%w: member version conflict or member is locked", sharedrepository.ErrConflict)
			}
			members = append(members, member)
		}
		newKey := "evt_" + repository.ids.New()
		fingerprint, hasFingerprint, err := repository.fingerprintForContent(ctx, transaction.SQL, members[0].ContentID)
		if err != nil {
			return err
		}
		var fingerprintValue, fingerprintVersion any
		if hasFingerprint {
			fingerprintValue, fingerprintVersion = fingerprint.Value, fingerprint.Version
		}
		if err := transaction.SQL.QueryRowContext(ctx, `
INSERT INTO events (event_key, event_fingerprint, fingerprint_version, title_zh, title_en, summary, lifecycle_status, first_seen_at, last_seen_at, representative_content_id)
VALUES ($1, $2, $3, $4, $5, $6, 'detected', $7, $8, NULL) RETURNING id, version, created_at, updated_at`, newKey, fingerprintValue, fingerprintVersion, source.TitleZH, source.TitleEN, source.Summary, source.FirstSeenAt, source.LastSeenAt).Scan(&created.ID, &created.Version, &created.FirstSeenAt, &created.LastSeenAt); err != nil {
			return err
		}
		created.EventKey, created.TitleZH, created.TitleEN, created.Summary, created.LifecycleStatus = newKey, source.TitleZH, source.TitleEN, source.Summary, domain.LifecycleDetected
		if hasFingerprint {
			created.EventFingerprint, created.FingerprintVersion = fingerprint.Value, fingerprint.Version
		}
		created.FirstSeenAt, created.LastSeenAt = source.FirstSeenAt, source.LastSeenAt
		contentIDs := make([]int64, 0, len(members))
		for _, member := range members {
			if _, err := transaction.SQL.ExecContext(ctx, `UPDATE event_contents SET event_id = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, created.ID, member.ID, member.Version); err != nil {
				return err
			}
			if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: created.ID, Action: domain.AuditMemberMove, ActorUserID: command.ActorUserID, ReasonCode: "split_member_moved", SourceEventID: &source.ID, TargetEventID: &created.ID, ExpectedVersion: &member.Version, Metadata: map[string]any{"content_id": member.ContentID, "member_id": member.ID, "relation": "event_content"}}); err != nil {
				return err
			}
			contentIDs = append(contentIDs, member.ContentID)
		}
		if err := copyMonitorEvents(ctx, transaction.SQL, source.ID, created.ID, command.ActorUserID); err != nil {
			return err
		}
		source, err = repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: source.ID, ReasonCode: "split_evidence_recompute", ActorUserID: command.ActorUserID})
		if err != nil {
			return err
		}
		created, err = repository.RecomputeEventEvidence(ctx, application.EvidenceRecomputeCommand{EventID: created.ID, ReasonCode: "split_evidence_recompute", ActorUserID: command.ActorUserID})
		if err != nil {
			return err
		}
		expected := command.SourceExpectedVersion
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: source.ID, Action: domain.AuditSplit, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode, SourceEventID: &source.ID, TargetEventID: &created.ID, ExpectedVersion: &expected}); err != nil {
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
			return fmt.Errorf("%w: member version conflict", sharedrepository.ErrConflict)
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
	rows, err := query.QueryContext(ctx, `SELECT id, version, event_id, content_id, membership_score, evidence_role, is_representative, origin, manual_locked, created_at FROM event_contents WHERE event_id = $1 ORDER BY content_id FOR UPDATE`, sourceID)
	if err != nil {
		return err
	}
	type persistedMember struct {
		domain.EventMember
		createdAt time.Time
	}
	members := make([]persistedMember, 0)
	for rows.Next() {
		var member persistedMember
		var role, origin string
		if err := rows.Scan(&member.ID, &member.Version, &member.EventID, &member.ContentID, &member.MembershipScore, &role, &member.Representative, &origin, &member.ManualLocked, &member.createdAt); err != nil {
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
			return fmt.Errorf("%w: locked member conflict", sharedrepository.ErrConflict)
		}
		var targetIDValue int64
		var targetScore float64
		var targetVersion int64
		var targetCreatedAt time.Time
		err := query.QueryRowContext(ctx, `SELECT id, membership_score, version, created_at FROM event_contents WHERE event_id = $1 AND content_id = $2 FOR UPDATE`, targetID, member.ContentID).Scan(&targetIDValue, &targetScore, &targetVersion, &targetCreatedAt)
		if err == sql.ErrNoRows {
			if _, err := query.ExecContext(ctx, `UPDATE event_contents SET event_id = $1, version = version + 1, updated_at = now() WHERE id = $2 AND version = $3`, targetID, member.ID, member.Version); err != nil {
				return err
			}
			expected := member.Version
			if err := insertAudit(ctx, query, domain.GovernanceAudit{EventID: targetID, Action: domain.AuditMemberMove, ActorUserID: actor, ReasonCode: "merge_member_moved", SourceEventID: &sourceID, TargetEventID: &targetID, ExpectedVersion: &expected, Metadata: map[string]any{"content_id": member.ContentID, "member_id": member.ID, "relation": "event_content"}}); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if member.MembershipScore > targetScore || member.createdAt.Before(targetCreatedAt) {
			if _, err := query.ExecContext(ctx, `UPDATE event_contents SET membership_score = GREATEST(membership_score, $1), created_at = LEAST(created_at, $2), version = version + 1, updated_at = now() WHERE id = $3 AND version = $4`, member.MembershipScore, member.createdAt, targetIDValue, targetVersion); err != nil {
				return err
			}
		}
		if _, err := query.ExecContext(ctx, `DELETE FROM event_contents WHERE id = $1`, member.ID); err != nil {
			return err
		}
		expected := member.Version
		if err := insertAudit(ctx, query, domain.GovernanceAudit{EventID: targetID, Action: domain.AuditDeduplicated, ActorUserID: actor, ReasonCode: "merge_duplicate_member", SourceEventID: &sourceID, TargetEventID: &targetID, ExpectedVersion: &expected, Metadata: map[string]any{"content_id": member.ContentID, "discarded_member_id": member.ID, "retained_member_id": targetIDValue, "relation": "event_content"}}); err != nil {
			return err
		}
	}
	return nil
}

func mergeMonitorEvents(ctx context.Context, query *sql.Tx, sourceID, targetID int64, actor *int64) error {
	rows, err := query.QueryContext(ctx, `SELECT monitor_id, relevance_score, final_score, first_matched_at, last_matched_at, status, created_at FROM monitor_events WHERE event_id = $1 ORDER BY monitor_id FOR UPDATE`, sourceID)
	if err != nil {
		return err
	}
	type monitorEvent struct {
		monitorID        int64
		relevance, final float64
		first, last      time.Time
		createdAt        time.Time
		status           string
	}
	items := make([]monitorEvent, 0)
	for rows.Next() {
		var item monitorEvent
		if err := rows.Scan(&item.monitorID, &item.relevance, &item.final, &item.first, &item.last, &item.status, &item.createdAt); err != nil {
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
		var targetMonitorEventID int64
		err := query.QueryRowContext(ctx, `SELECT id FROM monitor_events WHERE monitor_id = $1 AND event_id = $2 FOR UPDATE`, item.monitorID, targetID).Scan(&targetMonitorEventID)
		duplicate := err == nil
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if _, err := query.ExecContext(ctx, `
INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at, status, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (monitor_id, event_id) DO UPDATE SET relevance_score = GREATEST(monitor_events.relevance_score, EXCLUDED.relevance_score), final_score = GREATEST(monitor_events.final_score, EXCLUDED.final_score), first_matched_at = LEAST(monitor_events.first_matched_at, EXCLUDED.first_matched_at), last_matched_at = GREATEST(monitor_events.last_matched_at, EXCLUDED.last_matched_at), created_at = LEAST(monitor_events.created_at, EXCLUDED.created_at), updated_at = now()`, item.monitorID, targetID, item.relevance, item.final, item.first, item.last, item.status, item.createdAt); err != nil {
			return err
		}
		action, reason := domain.AuditMemberMove, "merge_monitor_moved"
		if duplicate {
			action, reason = domain.AuditDeduplicated, "merge_duplicate_monitor"
		}
		if err := insertAudit(ctx, query, domain.GovernanceAudit{EventID: targetID, Action: action, ActorUserID: actor, ReasonCode: reason, SourceEventID: &sourceID, TargetEventID: &targetID, Metadata: map[string]any{"monitor_id": item.monitorID, "relation": "monitor_event"}}); err != nil {
			return err
		}
	}
	_, err = query.ExecContext(ctx, `DELETE FROM monitor_events WHERE event_id = $1`, sourceID)
	return err
}

func copyMonitorEvents(ctx context.Context, query *sql.Tx, sourceID, targetID int64, actor *int64) error {
	rows, err := query.QueryContext(ctx, `SELECT monitor_id FROM monitor_events WHERE event_id = $1 ORDER BY monitor_id FOR UPDATE`, sourceID)
	if err != nil {
		return err
	}
	monitorIDs := make([]int64, 0)
	for rows.Next() {
		var monitorID int64
		if err := rows.Scan(&monitorID); err != nil {
			_ = rows.Close()
			return err
		}
		monitorIDs = append(monitorIDs, monitorID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, monitorID := range monitorIDs {
		result, err := query.ExecContext(ctx, `INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at, status) SELECT monitor_id, $2, relevance_score, final_score, first_matched_at, last_matched_at, status FROM monitor_events WHERE event_id = $1 AND monitor_id = $3 ON CONFLICT DO NOTHING`, sourceID, targetID, monitorID)
		if err != nil {
			return err
		}
		if inserted, _ := result.RowsAffected(); inserted != 1 {
			continue
		}
		if err := insertAudit(ctx, query, domain.GovernanceAudit{EventID: targetID, Action: domain.AuditMemberMove, ActorUserID: actor, ReasonCode: "split_monitor_copied", SourceEventID: &sourceID, TargetEventID: &targetID, Metadata: map[string]any{"monitor_id": monitorID, "relation": "monitor_event"}}); err != nil {
			return err
		}
	}
	return nil
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
		if err == sql.ErrNoRows {
			return domain.Event{}, fmt.Errorf("%w: event", sharedrepository.ErrNotFound)
		}
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
		if err == sql.ErrNoRows {
			return domain.EventMember{}, fmt.Errorf("%w: event member", sharedrepository.ErrNotFound)
		}
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
