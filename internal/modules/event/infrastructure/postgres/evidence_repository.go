package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var _ application.EvidenceRepository = (*Repository)(nil)

// RecomputeEventEvidence refreshes only Event-owned evidence projections. It
// keeps inactive member rows as history, but excludes them from the current
// lifecycle and representative calculation.
func (repository *Repository) RecomputeEventEvidence(ctx context.Context, command application.EvidenceRecomputeCommand) (domain.Event, error) {
	if !repository.available() {
		return domain.Event{}, sharedrepository.ErrUnavailable
	}
	if err := command.Validate(); err != nil {
		return domain.Event{}, err
	}
	var result domain.Event
	err := repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		event, err := lockedEvent(ctx, transaction.SQL, command.EventID)
		if err != nil {
			return err
		}
		from, to := event.LifecycleStatus, event.LifecycleStatus
		metadata := map[string]any{"recomputed": false}
		if event.ManualLocked || event.LifecycleStatus == domain.LifecycleArchived || event.LifecycleStatus == domain.LifecycleMerged || event.LifecycleStatus == domain.LifecycleRejected {
			metadata["manual_or_terminal"] = true
			if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: event.ID, Action: domain.AuditEvidenceRecompute, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode, FromStatus: &from, ToStatus: &to, ExpectedVersion: &event.Version, Metadata: metadata}); err != nil {
				return err
			}
			result = event
			return nil
		}
		members, sourceIDs, err := activeEvidenceMembers(ctx, transaction.SQL, event.ID)
		if err != nil {
			return err
		}
		status, representative, err := application.RecomputeEvidence(application.EvidenceInput{Event: event, ValidMembers: members, IndependentSourceIDs: sourceIDs})
		if err != nil {
			return err
		}
		if status != event.LifecycleStatus && !domain.CanTransition(event.LifecycleStatus, status) {
			return fmt.Errorf("event evidence produced invalid lifecycle transition %s -> %s", event.LifecycleStatus, status)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE event_contents
SET is_representative = ($1::bigint IS NOT NULL AND content_id = $1), version = version + 1, updated_at = now()
WHERE event_id = $2 AND evidence_role <> 'duplicate'
  AND is_representative IS DISTINCT FROM ($1::bigint IS NOT NULL AND content_id = $1)`, nullableInt64(representative), event.ID); err != nil {
			return sharedrepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(ctx, `
UPDATE events
SET lifecycle_status = $1, representative_content_id = $2, version = version + 1, updated_at = now()
WHERE id = $3 AND version = $4`, status, nullableInt64(representative), event.ID, event.Version); err != nil {
			return sharedrepository.MapError(err)
		}
		to = status
		metadata = map[string]any{"recomputed": true, "active_member_count": len(members)}
		if representative != nil {
			metadata["representative_content_id"] = *representative
		}
		if err := insertAudit(ctx, transaction.SQL, domain.GovernanceAudit{EventID: event.ID, Action: domain.AuditEvidenceRecompute, ActorUserID: command.ActorUserID, ReasonCode: command.ReasonCode, FromStatus: &from, ToStatus: &to, ExpectedVersion: &event.Version, Metadata: metadata}); err != nil {
			return err
		}
		event.Version++
		event.LifecycleStatus, event.RepresentativeContentID = status, representative
		result = event
		return nil
	})
	if err != nil {
		return domain.Event{}, err
	}
	return result, nil
}

func activeEvidenceMembers(ctx context.Context, transaction *sql.Tx, eventID int64) ([]domain.EventMember, []int64, error) {
	rows, err := transaction.QueryContext(ctx, `
SELECT ec.id, ec.version, ec.event_id, ec.content_id, ec.membership_score, ec.evidence_role, ec.is_representative, ec.origin, ec.manual_locked, c.source_connection_id
FROM event_contents ec
JOIN contents c ON c.id = ec.content_id AND c.content_status = 'active'
WHERE ec.event_id = $1 AND ec.evidence_role <> 'duplicate'
ORDER BY ec.membership_score DESC, ec.content_id ASC
FOR UPDATE OF ec`, eventID)
	if err != nil {
		return nil, nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	members := make([]domain.EventMember, 0)
	sourceIDs := make([]int64, 0)
	for rows.Next() {
		var member domain.EventMember
		var role, origin string
		var sourceID int64
		if err := rows.Scan(&member.ID, &member.Version, &member.EventID, &member.ContentID, &member.MembershipScore, &role, &member.Representative, &origin, &member.ManualLocked, &sourceID); err != nil {
			return nil, nil, sharedrepository.MapError(err)
		}
		member.EvidenceRole, member.Origin = domain.EvidenceRole(role), domain.MemberOrigin(origin)
		members = append(members, member)
		sourceIDs = append(sourceIDs, sourceID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, sharedrepository.MapError(err)
	}
	return members, sourceIDs, nil
}
