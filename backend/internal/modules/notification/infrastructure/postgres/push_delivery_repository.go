package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ application.WebPushDeliveryRepository = (*Repository)(nil)

type claimedWebPushDeliveryRecord struct {
	notification                                         userNotificationRecord
	attemptCount                                         int
	subscriptionID, subscriptionVersion                  int64
	ttlSeconds                                           int
	endpointSHA256                                       string
	endpointCiphertext, p256dhCiphertext, authCiphertext []byte
	encryptionKeyVersion                                 int
}

func (record claimedWebPushDeliveryRecord) dto(claimToken string) (application.ClaimedWebPushDeliveryDTO, error) {
	notification, err := record.notification.dto()
	if err != nil {
		return application.ClaimedWebPushDeliveryDTO{}, err
	}
	return application.ClaimedWebPushDeliveryDTO{
		Claimed: true, ClaimToken: claimToken, AttemptCount: record.attemptCount,
		SubscriptionID: record.subscriptionID, SubscriptionVersion: record.subscriptionVersion,
		TTLSeconds: record.ttlSeconds, EndpointSHA256: record.endpointSHA256,
		EndpointCiphertext:   append([]byte(nil), record.endpointCiphertext...),
		P256DHCiphertext:     append([]byte(nil), record.p256dhCiphertext...),
		AuthCiphertext:       append([]byte(nil), record.authCiphertext...),
		EncryptionKeyVersion: record.encryptionKeyVersion, Notification: notification,
	}, nil
}

func (repository *Repository) ClaimNextWebPushDelivery(ctx context.Context, command application.ClaimNextWebPushDeliveryCommand) (application.ClaimedWebPushDeliveryDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil ||
		!lowerHexClaimToken.MatchString(command.ClaimToken) || command.ClaimedAt.IsZero() ||
		!command.LeaseUntil.After(command.ClaimedAt) || command.LeaseUntil.Sub(command.ClaimedAt) > 5*time.Minute {
		return application.ClaimedWebPushDeliveryDTO{}, sharedrepository.ErrInvalidInput
	}
	var claimed application.ClaimedWebPushDeliveryDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var record claimedWebPushDeliveryRecord
		err := transaction.SQL.QueryRowContext(transactionContext, `
WITH push_attempts AS (
    SELECT user_notification_id,delivery_target_key,count(*)::integer AS attempt_count,
           max(attempted_at) AS last_attempted_at,bool_or(status IN ('succeeded','permanent_failure')) AS terminal
    FROM notification_delivery_attempts WHERE channel='web_push'
    GROUP BY user_notification_id,delivery_target_key
)
SELECT notification.id,notification.version,notification.outbox_event_id,notification.user_id,
       notification.monitor_id,notification.resource_id,notification.resource_version,
       notification.event_type,notification.resource_type,notification.title,notification.summary,
       notification.resource_status,notification.deep_link,notification.occurred_at,notification.created_at,
       COALESCE(attempts.attempt_count,0),subscription.id,subscription.version,subscription.ttl_seconds,
       subscription.endpoint_sha256,subscription.endpoint_ciphertext,subscription.p256dh_ciphertext,
       subscription.auth_ciphertext,subscription.encryption_key_version
FROM user_notifications AS notification
JOIN users AS actor ON actor.id=notification.user_id
JOIN monitors AS monitor ON monitor.id=notification.monitor_id
JOIN web_push_subscription_monitors AS filter ON filter.monitor_id=notification.monitor_id
JOIN web_push_subscriptions AS subscription ON subscription.id=filter.subscription_id
    AND subscription.user_id=notification.user_id AND subscription.status='active'
LEFT JOIN push_attempts AS attempts ON attempts.user_notification_id=notification.id
    AND attempts.delivery_target_key='push_subscription:' || subscription.id::text
LEFT JOIN notification_delivery_claims AS claim ON claim.user_notification_id=notification.id
    AND claim.channel='web_push' AND claim.delivery_target_key='push_subscription:' || subscription.id::text
WHERE actor.status='active' AND actor.deleted_at IS NULL
  AND monitor.created_by=notification.user_id AND monitor.status='active' AND monitor.deleted_at IS NULL
  AND COALESCE(attempts.terminal,false)=false AND COALESCE(attempts.attempt_count,0)<$1
  AND (claim.user_notification_id IS NULL OR claim.lease_until<=$2)
  AND (attempts.last_attempted_at IS NULL OR attempts.last_attempted_at <= $2 -
      make_interval(mins => (1 << LEAST(GREATEST(attempts.attempt_count-1,0),3))))
  AND (subscription.quiet_start IS NULL OR NOT (
      subscription.quiet_start<subscription.quiet_end
        AND ($2 AT TIME ZONE subscription.timezone)::time>=subscription.quiet_start
        AND ($2 AT TIME ZONE subscription.timezone)::time<subscription.quiet_end
      OR subscription.quiet_start>subscription.quiet_end
        AND (($2 AT TIME ZONE subscription.timezone)::time>=subscription.quiet_start
             OR ($2 AT TIME ZONE subscription.timezone)::time<subscription.quiet_end)
  ))
ORDER BY notification.id ASC,subscription.id ASC
FOR NO KEY UPDATE OF notification SKIP LOCKED
LIMIT 1`, application.MaximumWebPushAttempts, command.ClaimedAt).Scan(
			&record.notification.id, &record.notification.version, &record.notification.outboxEventID,
			&record.notification.userID, &record.notification.monitorID, &record.notification.resourceID,
			&record.notification.resourceVersion, &record.notification.eventType, &record.notification.resourceType,
			&record.notification.title, &record.notification.summary, &record.notification.resourceStatus,
			&record.notification.deepLink, &record.notification.occurredAt, &record.notification.createdAt,
			&record.attemptCount, &record.subscriptionID, &record.subscriptionVersion, &record.ttlSeconds,
			&record.endpointSHA256, &record.endpointCiphertext, &record.p256dhCiphertext,
			&record.authCiphertext, &record.encryptionKeyVersion,
		)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		target := application.WebPushDeliveryTarget(record.subscriptionID)
		claimResult, err := transaction.SQL.ExecContext(transactionContext, `
INSERT INTO notification_delivery_claims(
    user_notification_id,channel,delivery_target_key,claim_token,claimed_at,lease_until
) VALUES ($1,'web_push',$2,$3,$4,$5)
ON CONFLICT (user_notification_id,channel,delivery_target_key) DO UPDATE
SET claim_token=EXCLUDED.claim_token,claimed_at=EXCLUDED.claimed_at,lease_until=EXCLUDED.lease_until
WHERE notification_delivery_claims.lease_until<=EXCLUDED.claimed_at`, record.notification.id,
			target, command.ClaimToken, command.ClaimedAt, command.LeaseUntil)
		if err != nil {
			return databaserepository.MapError(err)
		}
		// The candidate SELECT and the claim INSERT are one transaction, but a
		// concurrent transaction can still win the unique target after this
		// statement's snapshot was established. ON CONFLICT then affects zero
		// rows. Never hand a caller a claim token that was not persisted.
		affected, err := claimResult.RowsAffected()
		if err != nil {
			return databaserepository.MapError(err)
		}
		if affected != 1 {
			return nil
		}
		// A waiter may have evaluated the candidate using a snapshot taken
		// before the previous owner committed its terminal attempt and removed
		// the claim. The successful INSERT above starts a new statement here,
		// so re-evaluate delivery eligibility against a fresh READ COMMITTED
		// snapshot before exposing ciphertext to an external sender.
		var stillEligible bool
		if err := transaction.SQL.QueryRowContext(transactionContext, `
SELECT count(*) < $3::integer
   AND NOT COALESCE(bool_or(status IN ('succeeded','permanent_failure')),false)
   AND (max(attempted_at) IS NULL OR max(attempted_at) <= $4::timestamptz -
       make_interval(mins => (1 << LEAST(GREATEST(count(*)::integer-1,0),3))))
FROM notification_delivery_attempts
WHERE user_notification_id=$1::bigint AND channel='web_push' AND delivery_target_key=$2::varchar`,
			record.notification.id, target, application.MaximumWebPushAttempts, command.ClaimedAt).Scan(&stillEligible); err != nil {
			return databaserepository.MapError(err)
		}
		if !stillEligible {
			if _, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='web_push' AND delivery_target_key=$2 AND claim_token=$3`,
				record.notification.id, target, command.ClaimToken); err != nil {
				return databaserepository.MapError(err)
			}
			return nil
		}
		mapped, err := record.dto(command.ClaimToken)
		if err != nil {
			return fmt.Errorf("map claimed Web Push delivery: %w", err)
		}
		claimed = mapped
		return nil
	})
	if err != nil {
		return application.ClaimedWebPushDeliveryDTO{}, err
	}
	return claimed, nil
}

func (repository *Repository) ValidateWebPushDeliveryClaim(ctx context.Context, query application.ValidateWebPushDeliveryClaimQuery) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || query.UserNotificationID <= 0 ||
		query.SubscriptionID <= 0 || !lowerHexClaimToken.MatchString(query.ClaimToken) || query.ValidatedAt.IsZero() {
		return sharedrepository.ErrInvalidInput
	}
	var valid bool
	err := repository.runtime.SQL.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM notification_delivery_claims AS claim
    JOIN user_notifications AS notification ON notification.id=claim.user_notification_id
    JOIN users AS actor ON actor.id=notification.user_id
    JOIN monitors AS monitor ON monitor.id=notification.monitor_id
    JOIN web_push_subscription_monitors AS filter ON filter.monitor_id=notification.monitor_id
    JOIN web_push_subscriptions AS subscription ON subscription.id=filter.subscription_id
    WHERE claim.user_notification_id=$1 AND claim.channel='web_push'
      AND claim.delivery_target_key='push_subscription:' || ($2::bigint)::text AND claim.claim_token=$3
      AND claim.lease_until>$4 AND subscription.id=$2::bigint AND subscription.user_id=notification.user_id
      AND subscription.status='active' AND actor.status='active' AND actor.deleted_at IS NULL
      AND monitor.created_by=notification.user_id AND monitor.status='active' AND monitor.deleted_at IS NULL
)`, query.UserNotificationID, query.SubscriptionID, query.ClaimToken, query.ValidatedAt).Scan(&valid)
	if err != nil {
		return databaserepository.MapError(err)
	}
	if !valid {
		return sharedrepository.ErrNotFound
	}
	return nil
}

func (repository *Repository) CompleteWebPushDelivery(ctx context.Context, command application.CompleteWebPushDeliveryCommand) (application.RecordNotificationDeliveryAttemptResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.UserNotificationID <= 0 ||
		command.UserID <= 0 || command.SubscriptionID <= 0 || !lowerHexClaimToken.MatchString(command.ClaimToken) {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrInvalidInput
	}
	target := application.WebPushDeliveryTarget(command.SubscriptionID)
	attempt := application.RecordNotificationDeliveryAttemptCommand{
		UserNotificationID: command.UserNotificationID, UserID: command.UserID, Channel: "web_push",
		DeliveryTargetKey: target, Status: command.Status, ProviderMessageID: command.ProviderMessageID,
		ResponseCode: command.ResponseCode, ErrorCode: command.ErrorCode, AttemptedAt: command.AttemptedAt,
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
JOIN web_push_subscriptions AS subscription ON subscription.id=$2 AND subscription.user_id=notification.user_id
WHERE claim.user_notification_id=$1 AND claim.channel='web_push' AND claim.delivery_target_key=$3
  AND claim.claim_token=$4
FOR UPDATE OF claim`, command.UserNotificationID, command.SubscriptionID, target, command.ClaimToken).Scan(&ownerID, &leaseUntil); err != nil {
			return databaserepository.MapError(err)
		}
		if ownerID != command.UserID || leaseUntil.Before(command.AttemptedAt) {
			return sharedrepository.ErrConflict
		}
		if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_attempts(
    user_notification_id,channel,delivery_target_key,attempt_no,status,provider_message_id,response_code,error_code,attempted_at
)
SELECT $1::bigint,'web_push',$2::varchar,COALESCE(max(attempt_no),0)+1,$3::varchar,
       NULLIF($4::varchar,''),$5::integer,NULLIF($6::varchar,''),$7::timestamptz
FROM notification_delivery_attempts
WHERE user_notification_id=$1::bigint AND channel='web_push' AND delivery_target_key=$2::varchar
RETURNING id,attempt_no`, command.UserNotificationID, target, command.Status, command.ProviderMessageID,
			command.ResponseCode, command.ErrorCode, command.AttemptedAt).Scan(&result.DeliveryAttemptID, &result.AttemptNo); err != nil {
			return databaserepository.MapError(err)
		}
		if command.Status == "succeeded" {
			if _, err := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions SET last_success_at=$1,updated_at=$1 WHERE id=$2`, command.AttemptedAt, command.SubscriptionID); err != nil {
				return databaserepository.MapError(err)
			}
		} else if command.ExpirationReason != "" {
			if _, err := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions SET version=version+1,status='expired',expiration_reason=$1,
    last_failure_at=$2,updated_at=$2 WHERE id=$3`, command.ExpirationReason, command.AttemptedAt, command.SubscriptionID); err != nil {
				return databaserepository.MapError(err)
			}
		} else if _, err := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions SET last_failure_at=$1,updated_at=$1 WHERE id=$2`, command.AttemptedAt, command.SubscriptionID); err != nil {
			return databaserepository.MapError(err)
		}
		deleteResult, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='web_push' AND delivery_target_key=$2 AND claim_token=$3`,
			command.UserNotificationID, target, command.ClaimToken)
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
