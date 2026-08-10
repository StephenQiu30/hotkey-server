package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var lowerHexClaimToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

type claimedEmailDeliveryRecord struct {
	notification      userNotificationRecord
	recipientEmail    string
	attemptCount      int
	publishedConfigID int64
	publishedRevision int64
	alertEmailEnabled bool
}

func (record claimedEmailDeliveryRecord) dto(claimToken string) (application.ClaimedEmailDeliveryDTO, error) {
	notification, err := record.notification.dto()
	if err != nil {
		return application.ClaimedEmailDeliveryDTO{}, err
	}
	return application.ClaimedEmailDeliveryDTO{
		Claimed: true, ClaimToken: claimToken, AttemptCount: record.attemptCount,
		RecipientEmail: record.recipientEmail, Notification: notification,
		PublishedConfigID: record.publishedConfigID, PublishedRevision: record.publishedRevision,
		AlertEmailEnabled: record.alertEmailEnabled,
	}, nil
}

func (repository *Repository) ClaimNextEmailDelivery(ctx context.Context, command application.ClaimNextEmailDeliveryCommand) (application.ClaimedEmailDeliveryDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.ClaimedEmailDeliveryDTO{}, sharedrepository.ErrUnavailable
	}
	if !lowerHexClaimToken.MatchString(command.ClaimToken) || command.ClaimedAt.IsZero() ||
		!command.LeaseUntil.After(command.ClaimedAt) || command.LeaseUntil.Sub(command.ClaimedAt) > 5*time.Minute {
		return application.ClaimedEmailDeliveryDTO{}, sharedrepository.ErrInvalidInput
	}
	var claimed application.ClaimedEmailDeliveryDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var record claimedEmailDeliveryRecord
		err := transaction.SQL.QueryRowContext(transactionContext, `
WITH email_attempts AS (
    SELECT user_notification_id,count(*)::integer AS attempt_count,max(attempted_at) AS last_attempted_at,
           bool_or(status IN ('succeeded','permanent_failure')) AS terminal
    FROM notification_delivery_attempts
    WHERE channel='email' AND delivery_target_key=$1
    GROUP BY user_notification_id
)
SELECT notification.id,notification.version,notification.outbox_event_id,notification.user_id,
       notification.monitor_id,notification.resource_id,notification.resource_version,
       notification.event_type,notification.resource_type,notification.title,notification.summary,
       notification.resource_status,notification.deep_link,notification.occurred_at,notification.created_at,
       actor.email,COALESCE(attempts.attempt_count,0),config.id,config.revision,config.alert_email_enabled
FROM user_notifications AS notification
JOIN users AS actor ON actor.id=notification.user_id
JOIN monitors AS monitor ON monitor.id=notification.monitor_id
JOIN monitor_config_versions AS config ON config.id=monitor.published_config_version_id
    AND config.monitor_id=monitor.id AND config.state='published'
LEFT JOIN email_attempts AS attempts ON attempts.user_notification_id=notification.id
LEFT JOIN notification_delivery_claims AS claim ON claim.user_notification_id=notification.id
    AND claim.channel='email' AND claim.delivery_target_key=$1
WHERE actor.status='active' AND actor.deleted_at IS NULL AND btrim(actor.email)<>''
  AND monitor.created_by=notification.user_id AND monitor.status='active' AND monitor.deleted_at IS NULL
  AND config.alert_email_enabled
  AND COALESCE(attempts.terminal,false)=false AND COALESCE(attempts.attempt_count,0)<$2
  AND (claim.user_notification_id IS NULL OR claim.lease_until<=$3)
  AND (attempts.last_attempted_at IS NULL OR attempts.last_attempted_at <= $3 -
      make_interval(mins => (1 << LEAST(GREATEST(attempts.attempt_count-1,0),3))))
ORDER BY notification.id ASC
FOR UPDATE OF notification SKIP LOCKED
LIMIT 1`, application.PrimaryEmailDeliveryTarget, application.MaximumEmailAttempts, command.ClaimedAt).Scan(
			&record.notification.id, &record.notification.version, &record.notification.outboxEventID,
			&record.notification.userID, &record.notification.monitorID, &record.notification.resourceID,
			&record.notification.resourceVersion, &record.notification.eventType, &record.notification.resourceType,
			&record.notification.title, &record.notification.summary, &record.notification.resourceStatus,
			&record.notification.deepLink, &record.notification.occurredAt, &record.notification.createdAt,
			&record.recipientEmail, &record.attemptCount, &record.publishedConfigID, &record.publishedRevision,
			&record.alertEmailEnabled,
		)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(transactionContext, `
INSERT INTO notification_delivery_claims(
    user_notification_id,channel,delivery_target_key,claim_token,claimed_at,lease_until
) VALUES ($1,'email',$2,$3,$4,$5)
ON CONFLICT (user_notification_id,channel,delivery_target_key) DO UPDATE
SET claim_token=EXCLUDED.claim_token,claimed_at=EXCLUDED.claimed_at,lease_until=EXCLUDED.lease_until
WHERE notification_delivery_claims.lease_until<=EXCLUDED.claimed_at`, record.notification.id,
			application.PrimaryEmailDeliveryTarget, command.ClaimToken, command.ClaimedAt, command.LeaseUntil); err != nil {
			return databaserepository.MapError(err)
		}
		mapped, err := record.dto(command.ClaimToken)
		if err != nil {
			return fmt.Errorf("map claimed notification email: %w", err)
		}
		claimed = mapped
		return nil
	})
	if err != nil {
		return application.ClaimedEmailDeliveryDTO{}, err
	}
	return claimed, nil
}

func (repository *Repository) CompleteEmailDelivery(ctx context.Context, command application.CompleteEmailDeliveryCommand) (application.RecordNotificationDeliveryAttemptResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrUnavailable
	}
	if command.UserNotificationID <= 0 || command.UserID <= 0 || !lowerHexClaimToken.MatchString(command.ClaimToken) {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrInvalidInput
	}
	attempt := application.RecordNotificationDeliveryAttemptCommand{
		UserNotificationID: command.UserNotificationID, UserID: command.UserID, Channel: "email",
		DeliveryTargetKey: application.PrimaryEmailDeliveryTarget, Status: command.Status,
		ProviderMessageID: command.ProviderMessageID, ResponseCode: command.ResponseCode,
		ErrorCode: command.ErrorCode, AttemptedAt: command.AttemptedAt,
	}
	if err := application.ValidateNotificationDeliveryAttemptCommand(attempt); err != nil {
		return application.RecordNotificationDeliveryAttemptResult{}, err
	}
	var result application.RecordNotificationDeliveryAttemptResult
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var ownerID int64
		var leaseUntil time.Time
		if err := transaction.SQL.QueryRowContext(transactionContext, `
SELECT notification.user_id,claim.lease_until
FROM notification_delivery_claims AS claim
JOIN user_notifications AS notification ON notification.id=claim.user_notification_id
WHERE claim.user_notification_id=$1 AND claim.channel='email' AND claim.delivery_target_key=$2
  AND claim.claim_token=$3
FOR UPDATE OF claim`, command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.ClaimToken).Scan(&ownerID, &leaseUntil); err != nil {
			return databaserepository.MapError(err)
		}
		if ownerID != command.UserID || leaseUntil.Before(command.AttemptedAt) {
			return sharedrepository.ErrConflict
		}
		if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_attempts(
    user_notification_id,channel,delivery_target_key,attempt_no,status,provider_message_id,response_code,error_code,attempted_at
)
SELECT $1::bigint,'email',$2::varchar,COALESCE(max(attempt_no),0)+1,$3::varchar,NULLIF($4::varchar,''),$5::integer,NULLIF($6::varchar,''),$7::timestamptz
FROM notification_delivery_attempts
WHERE user_notification_id=$1::bigint AND channel='email' AND delivery_target_key=$2::varchar
RETURNING id,attempt_no`, command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.Status,
			command.ProviderMessageID, command.ResponseCode, command.ErrorCode, command.AttemptedAt,
		).Scan(&result.DeliveryAttemptID, &result.AttemptNo); err != nil {
			return databaserepository.MapError(err)
		}
		deleteResult, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2 AND claim_token=$3`,
			command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.ClaimToken)
		if err != nil {
			return databaserepository.MapError(err)
		}
		if affected, err := deleteResult.RowsAffected(); err != nil || affected != 1 {
			return sharedrepository.ErrConflict
		}
		return nil
	})
	if err != nil {
		return application.RecordNotificationDeliveryAttemptResult{}, err
	}
	return result, nil
}
