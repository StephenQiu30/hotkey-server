package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
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
	notification       userNotificationRecord
	recipientEmail     string
	attemptCount       int
	publishedConfigID  int64
	publishedRevision  int64
	alertEmailEnabled  bool
	monitorName        string
	sourceName         string
	sourceType         string
	relevanceScore     sql.NullFloat64
	originalURL        string
	maxFenceGeneration int64
	currentlyEligible  bool
}

func (record claimedEmailDeliveryRecord) dto(claimToken string) (application.ClaimedEmailDeliveryDTO, error) {
	notification, err := record.notification.dto()
	if err != nil {
		return application.ClaimedEmailDeliveryDTO{}, err
	}
	result := application.ClaimedEmailDeliveryDTO{
		Claimed: true, ClaimToken: claimToken, AttemptCount: record.attemptCount,
		RecipientEmail: record.recipientEmail, Notification: notification,
		PublishedConfigID: record.publishedConfigID, PublishedRevision: record.publishedRevision,
		AlertEmailEnabled: record.alertEmailEnabled, MonitorName: record.monitorName,
		SourceName: record.sourceName, SourceType: record.sourceType, OriginalURL: record.originalURL,
	}
	if record.relevanceScore.Valid {
		result.RelevanceScore = &record.relevanceScore.Float64
	}
	return result, nil
}

func (repository *Repository) ClaimNextEmailDelivery(ctx context.Context, command application.ClaimNextEmailDeliveryCommand) (application.ClaimedEmailDeliveryDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.ClaimedEmailDeliveryDTO{}, sharedrepository.ErrUnavailable
	}
	if !lowerHexClaimToken.MatchString(command.ClaimToken) || command.LeaseDuration <= 0 || command.LeaseDuration > 5*time.Minute {
		return application.ClaimedEmailDeliveryDTO{}, sharedrepository.ErrInvalidInput
	}
	var claimed application.ClaimedEmailDeliveryDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var record claimedEmailDeliveryRecord
		err := transaction.SQL.QueryRowContext(transactionContext, `
WITH email_attempts AS (
    SELECT user_notification_id,count(*)::integer AS attempt_count,max(attempted_at) AS last_attempted_at,
           bool_or(status IN ('succeeded','permanent_failure','unknown')) AS terminal,
           COALESCE(max(fencing_generation),0) AS max_fencing_generation
    FROM notification_delivery_attempts
    WHERE channel='email' AND delivery_target_key=$1
    GROUP BY user_notification_id
)
SELECT notification.id,notification.version,notification.outbox_event_id,notification.user_id,
       notification.monitor_id,notification.resource_id,notification.resource_version,
       notification.event_type,notification.resource_type,notification.title,notification.summary,
       notification.resource_status,notification.deep_link,notification.occurred_at,notification.created_at,
       actor.email,COALESCE(attempts.attempt_count,0),config.id,config.revision,config.alert_email_enabled,
       monitor.name,COALESCE(source.name,''),COALESCE(source.source_type,''),match.final_score,
       COALESCE(content.canonical_url,''),COALESCE(attempts.max_fencing_generation,0),
       (actor.status='active' AND actor.deleted_at IS NULL AND btrim(actor.email)<>''
        AND monitor.created_by=notification.user_id AND monitor.status='active' AND monitor.deleted_at IS NULL
        AND config.alert_email_enabled
        AND (notification.resource_status='urgent'
             OR notification.resource_status='high' AND config.alert_email_min_severity='warning')) AS currently_eligible
FROM user_notifications AS notification
JOIN users AS actor ON actor.id=notification.user_id
JOIN monitors AS monitor ON monitor.id=notification.monitor_id
JOIN monitor_config_versions AS config ON config.id=monitor.published_config_version_id
    AND config.monitor_id=monitor.id AND config.state='published'
LEFT JOIN contents AS content ON notification.resource_type='hotspot'
    AND content.id=notification.resource_id AND content.deleted_at IS NULL
LEFT JOIN source_connections AS source ON source.id=content.source_connection_id AND source.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT candidate.final_score
    FROM monitor_matches AS candidate
    WHERE candidate.monitor_id=notification.monitor_id AND candidate.content_id=content.id
      AND candidate.decision='accepted'
    ORDER BY candidate.created_at DESC,candidate.id DESC
    LIMIT 1
) AS match ON true
LEFT JOIN email_attempts AS attempts ON attempts.user_notification_id=notification.id
LEFT JOIN notification_delivery_claims AS claim ON claim.user_notification_id=notification.id
    AND claim.channel='email' AND claim.delivery_target_key=$1
WHERE COALESCE(attempts.terminal,false)=false AND COALESCE(attempts.attempt_count,0)<$2
  AND (claim.user_notification_id IS NULL OR claim.lease_until<=clock_timestamp())
  AND (claim.dispatch_started_at IS NOT NULL OR (
      actor.status='active' AND actor.deleted_at IS NULL AND btrim(actor.email)<>''
      AND monitor.created_by=notification.user_id AND monitor.status='active' AND monitor.deleted_at IS NULL
      AND config.alert_email_enabled
      AND (notification.resource_status='urgent'
           OR notification.resource_status='high' AND config.alert_email_min_severity='warning')
      AND (attempts.last_attempted_at IS NULL OR attempts.last_attempted_at <= clock_timestamp() -
          make_interval(mins => (1 << LEAST(GREATEST(attempts.attempt_count-1,0),3))))
  ))
ORDER BY notification.id ASC
FOR UPDATE OF notification SKIP LOCKED
LIMIT 1`, application.PrimaryEmailDeliveryTarget, application.MaximumEmailAttempts).Scan(
			&record.notification.id, &record.notification.version, &record.notification.outboxEventID,
			&record.notification.userID, &record.notification.monitorID, &record.notification.resourceID,
			&record.notification.resourceVersion, &record.notification.eventType, &record.notification.resourceType,
			&record.notification.title, &record.notification.summary, &record.notification.resourceStatus,
			&record.notification.deepLink, &record.notification.occurredAt, &record.notification.createdAt,
			&record.recipientEmail, &record.attemptCount, &record.publishedConfigID, &record.publishedRevision,
			&record.alertEmailEnabled, &record.monitorName, &record.sourceName, &record.sourceType,
			&record.relevanceScore, &record.originalURL, &record.maxFenceGeneration, &record.currentlyEligible,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return databaserepository.MapError(err)
		}

		type existingClaimRecord struct {
			token               string
			generation          int64
			dispatchKey         sql.NullString
			supportsIdempotency bool
			supportsReceipt     bool
			dispatchStartedAt   sql.NullTime
		}
		var existing existingClaimRecord
		hasExisting := true
		err = transaction.SQL.QueryRowContext(transactionContext, `
SELECT claim_token,fencing_generation,dispatch_key,provider_supports_idempotency,
       provider_supports_receipt_lookup,dispatch_started_at
FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2
FOR UPDATE`, record.notification.id, application.PrimaryEmailDeliveryTarget).Scan(
			&existing.token, &existing.generation, &existing.dispatchKey, &existing.supportsIdempotency,
			&existing.supportsReceipt, &existing.dispatchStartedAt,
		)
		if errors.Is(err, sql.ErrNoRows) {
			hasExisting = false
		} else if err != nil {
			return databaserepository.MapError(err)
		}

		generation := record.maxFenceGeneration + 1
		dispatchKey := stableNotificationEmailDispatchKey(record.notification.id, record.attemptCount+1)
		reconcileRequired := false
		var dispatchStartedAt any
		if hasExisting {
			generation = existing.generation + 1
			if existing.dispatchStartedAt.Valid {
				canReconcile := record.currentlyEligible && existing.dispatchKey.Valid && lowerHexClaimToken.MatchString(existing.dispatchKey.String) &&
					(existing.supportsIdempotency && command.ProviderCapabilities.SupportsIdempotency ||
						existing.supportsReceipt && command.ProviderCapabilities.SupportsReceiptLookup)
				if !canReconcile {
					if !existing.dispatchKey.Valid || !lowerHexClaimToken.MatchString(existing.dispatchKey.String) {
						return sharedrepository.ErrConstraint
					}
					var unknownAttemptID int64
					if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_attempts(
    user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,
    provider_supports_idempotency,provider_supports_receipt_lookup,error_code,attempted_at
)
VALUES ($1,'email',$2,$3,'unknown',$4,$5,$6,$7,'provider_outcome_unconfirmed',clock_timestamp())
RETURNING id`, record.notification.id, application.PrimaryEmailDeliveryTarget, record.attemptCount+1,
						existing.dispatchKey.String, existing.generation, existing.supportsIdempotency,
						existing.supportsReceipt).Scan(&unknownAttemptID); err != nil {
						return databaserepository.MapError(err)
					}
					deleteResult, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2
  AND claim_token=$3 AND fencing_generation=$4`, record.notification.id,
						application.PrimaryEmailDeliveryTarget, existing.token, existing.generation)
					if err != nil {
						return databaserepository.MapError(err)
					}
					if affected, err := deleteResult.RowsAffected(); err != nil || affected != 1 {
						return sharedrepository.ErrConflict
					}
					mapped, err := record.dto(command.ClaimToken)
					if err != nil {
						return fmt.Errorf("map recovered unknown notification email: %w", err)
					}
					mapped.RecoveredUnknown = true
					mapped.AttemptCount++
					claimed = mapped
					return nil
				}
				dispatchKey = existing.dispatchKey.String
				dispatchStartedAt = existing.dispatchStartedAt.Time
				reconcileRequired = true
			}
		}

		leaseMicroseconds := command.LeaseDuration.Microseconds()
		if hasExisting {
			if err := transaction.SQL.QueryRowContext(transactionContext, `
UPDATE notification_delivery_claims
SET claim_token=$3,fencing_generation=$4,dispatch_key=$5,
    provider_supports_idempotency=$6,provider_supports_receipt_lookup=$7,
    dispatch_started_at=$8,claimed_at=clock_timestamp(),
    lease_until=clock_timestamp()+($9::bigint * interval '1 microsecond')
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2
  AND claim_token=$10 AND fencing_generation=$11
RETURNING fencing_generation`, record.notification.id, application.PrimaryEmailDeliveryTarget,
				command.ClaimToken, generation, dispatchKey, command.ProviderCapabilities.SupportsIdempotency,
				command.ProviderCapabilities.SupportsReceiptLookup, dispatchStartedAt, leaseMicroseconds,
				existing.token, existing.generation).Scan(&generation); err != nil {
				return databaserepository.MapError(err)
			}
		} else if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_claims(
    user_notification_id,channel,delivery_target_key,claim_token,fencing_generation,dispatch_key,
    provider_supports_idempotency,provider_supports_receipt_lookup,claimed_at,lease_until
)
VALUES ($1,'email',$2,$3,$4,$5,$6,$7,clock_timestamp(),
        clock_timestamp()+($8::bigint * interval '1 microsecond'))
RETURNING fencing_generation`, record.notification.id, application.PrimaryEmailDeliveryTarget,
			command.ClaimToken, generation, dispatchKey, command.ProviderCapabilities.SupportsIdempotency,
			command.ProviderCapabilities.SupportsReceiptLookup, leaseMicroseconds).Scan(&generation); err != nil {
			return databaserepository.MapError(err)
		}
		mapped, err := record.dto(command.ClaimToken)
		if err != nil {
			return fmt.Errorf("map claimed notification email: %w", err)
		}
		mapped.FencingGeneration = generation
		mapped.DispatchKey = dispatchKey
		mapped.ReconcileRequired = reconcileRequired
		mapped.ProviderCapabilities = command.ProviderCapabilities
		claimed = mapped
		return nil
	})
	if err != nil {
		return application.ClaimedEmailDeliveryDTO{}, err
	}
	return claimed, nil
}

func stableNotificationEmailDispatchKey(notificationID int64, attemptNo int) string {
	value := fmt.Sprintf("notification-email:v1:%d:%s:%d", notificationID, application.PrimaryEmailDeliveryTarget, attemptNo)
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest[:])
}

func (repository *Repository) StartEmailDelivery(ctx context.Context, command application.StartEmailDeliveryCommand) error {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return sharedrepository.ErrUnavailable
	}
	if command.UserNotificationID <= 0 || command.UserID <= 0 || command.FencingGeneration <= 0 ||
		!lowerHexClaimToken.MatchString(command.ClaimToken) || !lowerHexClaimToken.MatchString(command.DispatchKey) {
		return sharedrepository.ErrInvalidInput
	}
	var generation int64
	err := repository.runtime.SQL.QueryRowContext(ctx, `
UPDATE notification_delivery_claims AS claim
SET dispatch_started_at=COALESCE(dispatch_started_at,clock_timestamp())
FROM user_notifications AS notification
WHERE claim.user_notification_id=$1 AND claim.channel='email' AND claim.delivery_target_key=$2
  AND claim.claim_token=$3 AND claim.fencing_generation=$4 AND claim.dispatch_key=$5
  AND claim.lease_until>clock_timestamp()
  AND notification.id=claim.user_notification_id AND notification.user_id=$6
RETURNING claim.fencing_generation`, command.UserNotificationID, application.PrimaryEmailDeliveryTarget,
		command.ClaimToken, command.FencingGeneration, command.DispatchKey, command.UserID).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return sharedrepository.ErrConflict
	}
	if err != nil {
		return databaserepository.MapError(err)
	}
	return nil
}

func (repository *Repository) CompleteEmailDelivery(ctx context.Context, command application.CompleteEmailDeliveryCommand) (application.RecordNotificationDeliveryAttemptResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrUnavailable
	}
	if command.UserNotificationID <= 0 || command.UserID <= 0 || command.FencingGeneration <= 0 ||
		!lowerHexClaimToken.MatchString(command.ClaimToken) || !lowerHexClaimToken.MatchString(command.DispatchKey) {
		return application.RecordNotificationDeliveryAttemptResult{}, sharedrepository.ErrInvalidInput
	}
	attempt := application.RecordNotificationDeliveryAttemptCommand{
		UserNotificationID: command.UserNotificationID, UserID: command.UserID, Channel: "email",
		DeliveryTargetKey: application.PrimaryEmailDeliveryTarget, Status: command.Status,
		ProviderMessageID: command.ProviderMessageID, ResponseCode: command.ResponseCode,
		ErrorCode: command.ErrorCode, AttemptedAt: time.Unix(1, 0).UTC(),
	}
	if err := application.ValidateNotificationDeliveryAttemptCommand(attempt); err != nil {
		return application.RecordNotificationDeliveryAttemptResult{}, err
	}
	var result application.RecordNotificationDeliveryAttemptResult
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		var ownerID int64
		if err := transaction.SQL.QueryRowContext(transactionContext, `
SELECT user_id FROM user_notifications WHERE id=$1 FOR UPDATE`, command.UserNotificationID).Scan(&ownerID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sharedrepository.ErrConflict
			}
			return databaserepository.MapError(err)
		}
		if ownerID != command.UserID {
			return sharedrepository.ErrConflict
		}
		var generation int64
		if err := transaction.SQL.QueryRowContext(transactionContext, `
SELECT fencing_generation
FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2
  AND claim_token=$3 AND fencing_generation=$4 AND dispatch_key=$5
  AND dispatch_started_at IS NOT NULL AND lease_until>clock_timestamp()
FOR UPDATE`, command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.ClaimToken,
			command.FencingGeneration, command.DispatchKey).Scan(&generation); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sharedrepository.ErrConflict
			}
			return databaserepository.MapError(err)
		}
		if err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO notification_delivery_attempts(
    user_notification_id,channel,delivery_target_key,attempt_no,status,dispatch_key,fencing_generation,
    provider_supports_idempotency,provider_supports_receipt_lookup,
    provider_message_id,response_code,error_code,attempted_at
)
SELECT $1::bigint,'email',$2::varchar,COALESCE(max(attempt_no),0)+1,$3::varchar,$4::char(64),$5::bigint,
       $6::boolean,$7::boolean,NULLIF($8::varchar,''),$9::integer,NULLIF($10::varchar,''),clock_timestamp()
FROM notification_delivery_attempts
WHERE user_notification_id=$1::bigint AND channel='email' AND delivery_target_key=$2::varchar
RETURNING id,attempt_no`, command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.Status,
			command.DispatchKey, command.FencingGeneration, command.ProviderCapabilities.SupportsIdempotency,
			command.ProviderCapabilities.SupportsReceiptLookup, command.ProviderMessageID, command.ResponseCode, command.ErrorCode,
		).Scan(&result.DeliveryAttemptID, &result.AttemptNo); err != nil {
			return databaserepository.MapError(err)
		}
		deleteResult, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE user_notification_id=$1 AND channel='email' AND delivery_target_key=$2
  AND claim_token=$3 AND fencing_generation=$4 AND dispatch_key=$5`,
			command.UserNotificationID, application.PrimaryEmailDeliveryTarget, command.ClaimToken,
			command.FencingGeneration, command.DispatchKey)
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
