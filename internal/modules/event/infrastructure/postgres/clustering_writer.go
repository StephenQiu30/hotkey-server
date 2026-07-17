package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

var _ application.ClusteringWriter = (*Repository)(nil)

type clusteringContent struct {
	ID          int64
	Title       string
	Excerpt     string
	PublishedAt time.Time
	DedupeKey   string
}

// ApplyClustering persists every candidate decision and applies exactly one
// deterministic outcome while holding the Content row lock. That lock makes a
// retry or concurrent replay idempotent without silently moving a Content
// between Events.
func (repository *Repository) ApplyClustering(ctx context.Context, decisions []domain.Decision) (application.ClusteringWriteResult, error) {
	contentID, err := validateClusteringDecisions(decisions)
	if err != nil {
		return application.ClusteringWriteResult{}, err
	}
	if !repository.available() {
		return application.ClusteringWriteResult{}, sharedrepository.ErrUnavailable
	}
	result := application.ClusteringWriteResult{}
	err = repository.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		content, err := lockClusteringContent(ctx, transaction.SQL, contentID)
		if err != nil {
			return err
		}
		if err := saveDecisions(ctx, transaction.SQL, decisions); err != nil {
			return err
		}
		if accepted := acceptedDecision(decisions); accepted != nil {
			event, created, err := repository.attachAcceptedDecision(ctx, transaction.SQL, content, *accepted)
			if err != nil {
				return err
			}
			result.Event, result.Created = &event, created
			return nil
		}
		if hasReviewDecision(decisions) {
			result.PendingReview = true
			return nil
		}
		if !hasNewEventDecision(decisions) {
			return fmt.Errorf("clustering decisions do not select an outcome")
		}
		event, created, err := repository.createEventForContent(ctx, transaction.SQL, content, decisions)
		if err != nil {
			return err
		}
		result.Event, result.Created = &event, created
		return nil
	})
	if err != nil {
		return application.ClusteringWriteResult{}, err
	}
	return result, nil
}

func validateClusteringDecisions(decisions []domain.Decision) (int64, error) {
	if len(decisions) == 0 {
		return 0, fmt.Errorf("clustering decisions are required")
	}
	contentID := decisions[0].ContentID
	version, featureHash := decisions[0].ClusteringVersion, decisions[0].FeatureInputHash
	for _, decision := range decisions {
		if err := decision.Validate(); err != nil {
			return 0, err
		}
		if decision.ContentID != contentID || decision.ClusteringVersion != version || decision.FeatureInputHash != featureHash {
			return 0, fmt.Errorf("clustering decisions must describe one input")
		}
	}
	return contentID, nil
}

func lockClusteringContent(ctx context.Context, transaction *sql.Tx, contentID int64) (clusteringContent, error) {
	var content clusteringContent
	err := transaction.QueryRowContext(ctx, `
SELECT id, title, excerpt, published_at, dedupe_key
FROM contents
WHERE id = $1 AND content_status = 'active'
FOR UPDATE`, contentID).Scan(&content.ID, &content.Title, &content.Excerpt, &content.PublishedAt, &content.DedupeKey)
	if errors.Is(err, sql.ErrNoRows) {
		return clusteringContent{}, fmt.Errorf("%w: active content %d", sharedrepository.ErrNotFound, contentID)
	}
	if err != nil {
		return clusteringContent{}, sharedrepository.MapError(err)
	}
	return content, nil
}

func acceptedDecision(decisions []domain.Decision) *domain.Decision {
	var selected *domain.Decision
	for index := range decisions {
		decision := &decisions[index]
		if decision.Decision != domain.DecisionAccept {
			continue
		}
		if selected == nil || decision.MembershipScore > selected.MembershipScore || decision.MembershipScore == selected.MembershipScore && decision.CandidateEventKey < selected.CandidateEventKey {
			selected = decision
		}
	}
	return selected
}

func hasReviewDecision(decisions []domain.Decision) bool {
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionReview {
			return true
		}
	}
	return false
}

func hasNewEventDecision(decisions []domain.Decision) bool {
	for _, decision := range decisions {
		if decision.Decision == domain.DecisionNewEvent {
			return true
		}
	}
	return false
}

func (repository *Repository) attachAcceptedDecision(ctx context.Context, transaction *sql.Tx, content clusteringContent, decision domain.Decision) (domain.Event, bool, error) {
	if decision.CandidateEventID == nil {
		return domain.Event{}, false, fmt.Errorf("accepted decision is missing candidate event")
	}
	if existingID, found, err := currentEventForContent(ctx, transaction, content.ID); err != nil {
		return domain.Event{}, false, err
	} else if found {
		event, err := lockedEvent(ctx, transaction, existingID)
		if err != nil {
			return domain.Event{}, false, err
		}
		if event.ID != *decision.CandidateEventID || event.EventKey != decision.CandidateEventKey {
			return domain.Event{}, false, fmt.Errorf("%w: content is already assigned to %s", sharedrepository.ErrConflict, event.EventKey)
		}
		return event, false, nil
	}
	target, err := lockedEvent(ctx, transaction, *decision.CandidateEventID)
	if err != nil {
		return domain.Event{}, false, err
	}
	if target.EventKey != decision.CandidateEventKey || target.ManualLocked || target.LifecycleStatus == domain.LifecycleMerged || target.LifecycleStatus == domain.LifecycleArchived || target.LifecycleStatus == domain.LifecycleRejected {
		return domain.Event{}, false, fmt.Errorf("%w: candidate event is unavailable", sharedrepository.ErrConflict)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, is_representative, origin)
VALUES ($1,$2,$3,'supporting',false,$4)`, target.ID, content.ID, decision.MembershipScore, domain.MemberOrigin(decision.DecisionOrigin)); err != nil {
		return domain.Event{}, false, sharedrepository.MapError(err)
	}
	if err := upsertMonitorEvents(ctx, transaction, content.ID, target.ID, content.PublishedAt); err != nil {
		return domain.Event{}, false, err
	}
	if _, err := transaction.ExecContext(ctx, `
UPDATE events
SET last_seen_at = GREATEST(last_seen_at, $1), version = version + 1, updated_at = now()
WHERE id = $2`, content.PublishedAt, target.ID); err != nil {
		return domain.Event{}, false, sharedrepository.MapError(err)
	}
	if err := insertAudit(ctx, transaction, domain.GovernanceAudit{EventID: target.ID, Action: domain.AuditEvidenceRecompute, ReasonCode: "clustering_member_attached", Metadata: map[string]any{"content_id": content.ID, "clustering_version": decision.ClusteringVersion}}); err != nil {
		return domain.Event{}, false, err
	}
	updated, err := lockedEvent(ctx, transaction, target.ID)
	return updated, false, err
}

func (repository *Repository) createEventForContent(ctx context.Context, transaction *sql.Tx, content clusteringContent, decisions []domain.Decision) (domain.Event, bool, error) {
	if existingID, found, err := currentEventForContent(ctx, transaction, content.ID); err != nil {
		return domain.Event{}, false, err
	} else if found {
		event, err := lockedEvent(ctx, transaction, existingID)
		return event, false, err
	}
	eventKey := "evt_" + repository.ids.New()
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO events (event_key, event_fingerprint, fingerprint_version, title_zh, summary, lifecycle_status, first_seen_at, last_seen_at, representative_content_id)
VALUES ($1,$2,'content_dedupe_v1',$3,'','detected',$4,$4,$5)`, eventKey, content.DedupeKey, clusteringTitle(content), content.PublishedAt, content.ID); err != nil {
		return domain.Event{}, false, sharedrepository.MapError(err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO event_contents (event_id, content_id, membership_score, evidence_role, is_representative, origin)
SELECT id, $1, 0, 'primary', true, 'rule' FROM events WHERE event_key = $2`, content.ID, eventKey); err != nil {
		return domain.Event{}, false, sharedrepository.MapError(err)
	}
	event, err := lockedEventByKey(ctx, transaction, eventKey)
	if err != nil {
		return domain.Event{}, false, err
	}
	if err := upsertMonitorEvents(ctx, transaction, content.ID, event.ID, content.PublishedAt); err != nil {
		return domain.Event{}, false, err
	}
	if err := insertAudit(ctx, transaction, domain.GovernanceAudit{EventID: event.ID, Action: domain.AuditEvidenceRecompute, ReasonCode: "clustering_new_event", Metadata: map[string]any{"content_id": content.ID, "clustering_version": decisions[0].ClusteringVersion}}); err != nil {
		return domain.Event{}, false, err
	}
	return event, true, nil
}

func currentEventForContent(ctx context.Context, transaction *sql.Tx, contentID int64) (int64, bool, error) {
	var eventID int64
	err := transaction.QueryRowContext(ctx, `
SELECT event_id FROM event_contents
WHERE content_id = $1 AND evidence_role <> 'duplicate'
FOR UPDATE`, contentID).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, sharedrepository.MapError(err)
	}
	return eventID, true, nil
}

func lockedEvent(ctx context.Context, transaction *sql.Tx, eventID int64) (domain.Event, error) {
	return scanEvent(transaction.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, eventID))
}

func lockedEventByKey(ctx context.Context, transaction *sql.Tx, eventKey string) (domain.Event, error) {
	return scanEvent(transaction.QueryRowContext(ctx, `
SELECT id, version, event_key, COALESCE(event_fingerprint, ''), COALESCE(fingerprint_version, ''), title_zh, COALESCE(title_en, ''), summary,
       lifecycle_status, first_seen_at, last_seen_at, representative_content_id, merged_into_id, manual_locked
FROM events WHERE event_key = $1 AND deleted_at IS NULL FOR UPDATE`, eventKey))
}

func upsertMonitorEvents(ctx context.Context, transaction *sql.Tx, contentID, eventID int64, matchedAt time.Time) error {
	_, err := transaction.ExecContext(ctx, `
INSERT INTO monitor_events (monitor_id, event_id, relevance_score, final_score, first_matched_at, last_matched_at)
SELECT monitor_id, $2, MAX(final_score), MAX(final_score), $3, $3
FROM monitor_matches
WHERE content_id = $1 AND decision = 'accepted'
GROUP BY monitor_id
ON CONFLICT (monitor_id, event_id) DO UPDATE
SET relevance_score = GREATEST(monitor_events.relevance_score, EXCLUDED.relevance_score),
    final_score = GREATEST(monitor_events.final_score, EXCLUDED.final_score),
    first_matched_at = LEAST(monitor_events.first_matched_at, EXCLUDED.first_matched_at),
    last_matched_at = GREATEST(monitor_events.last_matched_at, EXCLUDED.last_matched_at),
    updated_at = now()`, contentID, eventID, matchedAt)
	if err != nil {
		return sharedrepository.MapError(err)
	}
	return nil
}

func clusteringTitle(content clusteringContent) string {
	if value := strings.TrimSpace(content.Title); value != "" {
		return value
	}
	if value := strings.TrimSpace(content.Excerpt); value != "" {
		return value
	}
	return fmt.Sprintf("事件 %d", content.ID)
}
