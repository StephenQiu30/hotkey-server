package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Repository struct{ runtime *database.Runtime }

var _ application.Repository = (*Repository)(nil)

func NewRepository(runtime *database.Runtime) *Repository { return &Repository{runtime: runtime} }

// ListAfter is the explicit legacy read adapter retained only for migration
// verification. Default HTTP routes use ListUserNotifications and never call
// this audience-role projection.
func (repository *Repository) ListAfter(ctx context.Context, query domain.NotificationQuery) (domain.NotificationPage, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return domain.NotificationPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return domain.NotificationPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT id,event_type,resource_type,resource_id,audience_role,occurred_at,payload
FROM notification_events
WHERE id>$1 AND (
    $2='admin'
    OR $2='editor' AND audience_role IN ('viewer','analyst','editor')
    OR $2='analyst' AND audience_role IN ('viewer','analyst')
    OR $2='viewer' AND audience_role='viewer'
)
ORDER BY id ASC LIMIT $3`, query.AfterID, query.Role, query.Limit)
	if err != nil {
		return domain.NotificationPage{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	page := domain.NotificationPage{Items: make([]domain.NotificationEvent, 0, query.Limit), NextAfterID: query.AfterID}
	for rows.Next() {
		var item domain.NotificationEvent
		var payload []byte
		if err := rows.Scan(&item.ID, &item.EventType, &item.ResourceType, &item.ResourceID, &item.Audience, &item.OccurredAt, &payload); err != nil {
			return domain.NotificationPage{}, databaserepository.MapError(err)
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return domain.NotificationPage{}, fmt.Errorf("decode legacy notification payload: %w", err)
		}
		if err := item.Validate(); err != nil {
			return domain.NotificationPage{}, fmt.Errorf("validate legacy notification: %w", err)
		}
		page.Items = append(page.Items, item)
		page.NextAfterID = item.ID
	}
	if err := rows.Err(); err != nil {
		return domain.NotificationPage{}, databaserepository.MapError(err)
	}
	return page, nil
}

type userNotificationRecord struct {
	id, version, outboxEventID, userID, monitorID, resourceID, resourceVersion int64
	eventType, resourceType, title, summary, resourceStatus, deepLink          string
	occurredAt, createdAt                                                      time.Time
}

func (record userNotificationRecord) dto() (application.UserNotificationDTO, error) {
	result := application.UserNotificationDTO{
		ID: record.id, Version: record.version, OutboxEventID: record.outboxEventID, UserID: record.userID,
		MonitorID: record.monitorID, EventType: record.eventType, ResourceType: record.resourceType,
		ResourceID: record.resourceID, ResourceVersion: record.resourceVersion, OccurredAt: record.occurredAt,
		Title: record.title, Summary: record.summary, ResourceStatus: record.resourceStatus,
		DeepLink: record.deepLink, CreatedAt: record.createdAt,
	}
	if err := application.ValidateUserNotificationDTO(result); err != nil {
		return application.UserNotificationDTO{}, fmt.Errorf("map user notification record: %w", err)
	}
	return result, nil
}

func (repository *Repository) ListUserNotifications(ctx context.Context, query application.ListUserNotificationsQuery) (application.ListUserNotificationsResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.ListUserNotificationsResult{}, sharedrepository.ErrUnavailable
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, `
SELECT notification.id,notification.version,notification.outbox_event_id,notification.user_id,
       notification.monitor_id,notification.event_type,notification.resource_type,notification.resource_id,
       notification.resource_version,notification.occurred_at,notification.title,notification.summary,
       notification.resource_status,notification.deep_link,notification.created_at
FROM user_notifications AS notification
JOIN users AS actor ON actor.id=notification.user_id
JOIN monitors AS monitor ON monitor.id=notification.monitor_id
WHERE notification.user_id=$1 AND notification.id>$2
  AND ($3::bigint IS NULL OR notification.monitor_id=$3)
  AND actor.status='active' AND actor.deleted_at IS NULL
  AND monitor.created_by=notification.user_id AND monitor.deleted_at IS NULL AND monitor.status<>'archived'
ORDER BY notification.id ASC
LIMIT $4`, query.UserID, query.AfterID, query.MonitorID, query.Limit)
	if err != nil {
		return application.ListUserNotificationsResult{}, databaserepository.MapError(err)
	}
	defer rows.Close()

	result := application.ListUserNotificationsResult{
		Items: make([]application.UserNotificationDTO, 0, query.Limit), NextAfterID: query.AfterID,
	}
	for rows.Next() {
		var record userNotificationRecord
		if err := rows.Scan(
			&record.id, &record.version, &record.outboxEventID, &record.userID, &record.monitorID,
			&record.eventType, &record.resourceType, &record.resourceID, &record.resourceVersion,
			&record.occurredAt, &record.title, &record.summary, &record.resourceStatus, &record.deepLink, &record.createdAt,
		); err != nil {
			return application.ListUserNotificationsResult{}, databaserepository.MapError(err)
		}
		item, err := record.dto()
		if err != nil {
			return application.ListUserNotificationsResult{}, err
		}
		result.Items = append(result.Items, item)
		result.NextAfterID = item.ID
	}
	if err := rows.Err(); err != nil {
		return application.ListUserNotificationsResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *Repository) RecordDeliveryAttempt(ctx context.Context, command application.RecordNotificationDeliveryAttemptCommand) (application.RecordNotificationDeliveryAttemptResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrUnavailable
	}
	var result application.RecordNotificationDeliveryAttemptResult
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var ownerID int64
		if err := transaction.SQL.QueryRowContext(transactionContext, `
SELECT user_id FROM user_notifications WHERE id=$1 FOR UPDATE`, command.UserNotificationID).Scan(&ownerID); err != nil {
			return databaserepository.MapError(err)
		}
		if ownerID != command.UserID {
			return sharedrepository.ErrNotFound
		}
		if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_attempts(
    user_notification_id,channel,delivery_target_key,attempt_no,status,provider_message_id,response_code,error_code,attempted_at
)
SELECT $1::bigint,$2::varchar,$3::varchar,COALESCE(max(attempt_no),0)+1,$4::varchar,NULLIF($5::varchar,''),$6::integer,NULLIF($7::varchar,''),$8::timestamptz
FROM notification_delivery_attempts WHERE user_notification_id=$1::bigint AND channel=$2::varchar AND delivery_target_key=$3::varchar
RETURNING id,attempt_no`, command.UserNotificationID, command.Channel, command.DeliveryTargetKey, command.Status, command.ProviderMessageID,
			command.ResponseCode, command.ErrorCode, command.AttemptedAt,
		).Scan(&result.DeliveryAttemptID, &result.AttemptNo); err != nil {
			return databaserepository.MapError(err)
		}
		return nil
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrNotFound
		}
		return application.RecordNotificationDeliveryAttemptResult{}, err
	}
	return result, nil
}
