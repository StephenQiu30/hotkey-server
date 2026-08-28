package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ application.UnknownDeliveryAlertReader = (*Repository)(nil)

// UnknownDeliveryAlert returns one bounded aggregate for immutable unknown
// attempts. Reading it never mutates a claim or makes the delivery replayable.
func (repository *Repository) UnknownDeliveryAlert(ctx context.Context) (application.UnknownDeliveryAlertSummary, bool, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.UnknownDeliveryAlertSummary{}, false, sharedrepository.ErrUnavailable
	}
	var summary application.UnknownDeliveryAlertSummary
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT attempt.id,notification.id,notification.resource_type,notification.resource_id,
       count(*) OVER (),attempt.attempted_at
FROM notification_delivery_attempts AS attempt
JOIN user_notifications AS notification ON notification.id=attempt.user_notification_id
WHERE attempt.status='unknown'
ORDER BY attempt.attempted_at ASC,attempt.id ASC
LIMIT 1`).Scan(
		&summary.AttemptID, &summary.NotificationID, &summary.ResourceType,
		&summary.ResourceID, &summary.AffectedCount, &summary.TriggeredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return application.UnknownDeliveryAlertSummary{}, false, nil
	}
	if err != nil {
		return application.UnknownDeliveryAlertSummary{}, false, databaserepository.MapError(err)
	}
	return summary, true, nil
}
