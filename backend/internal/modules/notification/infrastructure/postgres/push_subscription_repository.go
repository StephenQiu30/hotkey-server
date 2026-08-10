package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

var _ application.PushSubscriptionRepository = (*Repository)(nil)

type pushSubscriptionRecord struct {
	id, version, userID                             int64
	deviceLabel, timezone, status, expirationReason string
	quietStart, quietEnd                            sql.NullString
	ttlSeconds                                      int
	monitorIDsJSON                                  []byte
	lastSuccessAt, lastFailureAt                    sql.NullTime
	createdAt, updatedAt                            time.Time
}

func (record pushSubscriptionRecord) dto() (application.PushSubscriptionDTO, error) {
	monitorIDs := make([]int64, 0)
	if err := json.Unmarshal(record.monitorIDsJSON, &monitorIDs); err != nil {
		return application.PushSubscriptionDTO{}, fmt.Errorf("decode push monitor ids: %w", err)
	}
	result := application.PushSubscriptionDTO{
		ID: record.id, Version: record.version, UserID: record.userID, DeviceLabel: record.deviceLabel,
		Timezone: record.timezone, TTLSeconds: record.ttlSeconds, Status: record.status,
		ExpirationReason: record.expirationReason, MonitorIDs: monitorIDs,
		CreatedAt: record.createdAt, UpdatedAt: record.updatedAt,
	}
	if record.quietStart.Valid {
		result.QuietStart, result.QuietEnd = &record.quietStart.String, &record.quietEnd.String
	}
	if record.lastSuccessAt.Valid {
		value := record.lastSuccessAt.Time
		result.LastSuccessAt = &value
	}
	if record.lastFailureAt.Valid {
		value := record.lastFailureAt.Time
		result.LastFailureAt = &value
	}
	if err := application.ValidatePushSubscriptionDTO(result, record.userID); err != nil {
		return application.PushSubscriptionDTO{}, fmt.Errorf("map push subscription record: %w", err)
	}
	return result, nil
}

const pushSubscriptionProjection = `
SELECT subscription.id,subscription.version,subscription.user_id,subscription.device_label,subscription.timezone,
       CASE WHEN subscription.quiet_start IS NULL THEN NULL ELSE to_char(subscription.quiet_start,'HH24:MI') END,
       CASE WHEN subscription.quiet_end IS NULL THEN NULL ELSE to_char(subscription.quiet_end,'HH24:MI') END,
       subscription.ttl_seconds,subscription.status,COALESCE(subscription.expiration_reason,''),
       COALESCE((SELECT jsonb_agg(link.monitor_id ORDER BY link.monitor_id)
                 FROM web_push_subscription_monitors AS link WHERE link.subscription_id=subscription.id),'[]'::jsonb),
       subscription.last_success_at,subscription.last_failure_at,subscription.created_at,subscription.updated_at
FROM web_push_subscriptions AS subscription`

type pushSubscriptionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func scanPushSubscription(row interface{ Scan(...any) error }) (application.PushSubscriptionDTO, error) {
	var record pushSubscriptionRecord
	if err := row.Scan(
		&record.id, &record.version, &record.userID, &record.deviceLabel, &record.timezone,
		&record.quietStart, &record.quietEnd, &record.ttlSeconds, &record.status, &record.expirationReason,
		&record.monitorIDsJSON, &record.lastSuccessAt, &record.lastFailureAt, &record.createdAt, &record.updatedAt,
	); err != nil {
		return application.PushSubscriptionDTO{}, databaserepository.MapError(err)
	}
	return record.dto()
}

func readPushSubscription(ctx context.Context, queryer pushSubscriptionQueryer, subscriptionID, userID int64) (application.PushSubscriptionDTO, error) {
	return scanPushSubscription(queryer.QueryRowContext(ctx, pushSubscriptionProjection+`
WHERE subscription.id=$1 AND subscription.user_id=$2`, subscriptionID, userID))
}

func (repository *Repository) PersistPushSubscription(ctx context.Context, command application.PersistPushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.UserID <= 0 ||
		command.EndpointSHA256 == "" || len(command.MonitorIDs) == 0 || command.CreatedAt.IsZero() {
		return application.PushSubscriptionDTO{}, sharedrepository.ErrInvalidInput
	}
	var result application.PushSubscriptionDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		if err := verifyPushUserAndMonitors(transactionContext, transaction.SQL, command.UserID, command.MonitorIDs); err != nil {
			return err
		}
		var subscriptionID int64
		err := transaction.SQL.QueryRowContext(transactionContext, `
INSERT INTO web_push_subscriptions(
    user_id,endpoint_sha256,endpoint_ciphertext,p256dh_ciphertext,auth_ciphertext,encryption_key_version,
    device_label,timezone,quiet_start,quiet_end,ttl_seconds,status,idempotency_key,command_fingerprint,created_at,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::time,$10::time,$11,'active',$12,$13,$14,$14)
ON CONFLICT DO NOTHING RETURNING id`, command.UserID, command.EndpointSHA256,
			command.EndpointCiphertext, command.P256DHCiphertext, command.AuthCiphertext, command.EncryptionKeyVersion,
			command.DeviceLabel, command.Timezone, command.QuietStart, command.QuietEnd, command.TTLSeconds,
			command.IdempotencyKey, command.CommandFingerprint, command.CreatedAt).Scan(&subscriptionID)
		if err == sql.ErrNoRows {
			var existingUserID int64
			var existingFingerprint string
			idempotencyErr := transaction.SQL.QueryRowContext(transactionContext, `
SELECT id,user_id,command_fingerprint FROM web_push_subscriptions WHERE idempotency_key=$1 FOR UPDATE`,
				command.IdempotencyKey).Scan(&subscriptionID, &existingUserID, &existingFingerprint)
			if idempotencyErr == nil {
				if existingUserID != command.UserID || existingFingerprint != command.CommandFingerprint {
					return sharedrepository.ErrConflict
				}
				result, err = readPushSubscription(transactionContext, transaction.SQL, subscriptionID, command.UserID)
				return err
			}
			if idempotencyErr != sql.ErrNoRows {
				return databaserepository.MapError(idempotencyErr)
			}

			var existingVersion int64
			var existingStatus string
			endpointErr := transaction.SQL.QueryRowContext(transactionContext, `
SELECT id,version,user_id,status,command_fingerprint
FROM web_push_subscriptions WHERE endpoint_sha256=$1 FOR UPDATE`, command.EndpointSHA256).
				Scan(&subscriptionID, &existingVersion, &existingUserID, &existingStatus, &existingFingerprint)
			if endpointErr != nil {
				return databaserepository.MapError(endpointErr)
			}
			if existingUserID != command.UserID {
				return sharedrepository.ErrConflict
			}
			if existingStatus == "active" && existingFingerprint == command.CommandFingerprint {
				result, err = readPushSubscription(transactionContext, transaction.SQL, subscriptionID, command.UserID)
				return err
			}
			updated, updateErr := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions
SET version=version+1,endpoint_ciphertext=$1,p256dh_ciphertext=$2,auth_ciphertext=$3,
    encryption_key_version=$4,device_label=$5,timezone=$6,quiet_start=$7::time,quiet_end=$8::time,
    ttl_seconds=$9,status='active',expiration_reason=NULL,idempotency_key=$10,command_fingerprint=$11,
    last_success_at=NULL,last_failure_at=NULL,updated_at=$12
WHERE id=$13 AND user_id=$14 AND version=$15`, command.EndpointCiphertext, command.P256DHCiphertext,
				command.AuthCiphertext, command.EncryptionKeyVersion, command.DeviceLabel, command.Timezone,
				command.QuietStart, command.QuietEnd, command.TTLSeconds, command.IdempotencyKey,
				command.CommandFingerprint, command.CreatedAt, subscriptionID, command.UserID, existingVersion)
			if updateErr != nil {
				return databaserepository.MapError(updateErr)
			}
			if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
				return sharedrepository.ErrConflict
			}
			err = nil
		}
		if err != nil {
			return databaserepository.MapError(err)
		}
		if _, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM web_push_subscription_monitors WHERE subscription_id=$1`, subscriptionID); err != nil {
			return databaserepository.MapError(err)
		}
		for _, monitorID := range command.MonitorIDs {
			if _, err := transaction.SQL.ExecContext(transactionContext, `
INSERT INTO web_push_subscription_monitors(subscription_id,monitor_id) VALUES ($1,$2)`, subscriptionID, monitorID); err != nil {
				return databaserepository.MapError(err)
			}
		}
		result, err = readPushSubscription(transactionContext, transaction.SQL, subscriptionID, command.UserID)
		return err
	})
	if err != nil {
		return application.PushSubscriptionDTO{}, err
	}
	return result, nil
}

func (repository *Repository) ListPushSubscriptions(ctx context.Context, query application.ListPushSubscriptionsQuery) (application.ListPushSubscriptionsResult, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || query.UserID <= 0 {
		return application.ListPushSubscriptionsResult{}, sharedrepository.ErrInvalidInput
	}
	rows, err := repository.runtime.SQL.QueryContext(ctx, pushSubscriptionProjection+`
JOIN users AS actor ON actor.id=subscription.user_id
WHERE subscription.user_id=$1 AND actor.status='active' AND actor.deleted_at IS NULL
ORDER BY subscription.id ASC`, query.UserID)
	if err != nil {
		return application.ListPushSubscriptionsResult{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := application.ListPushSubscriptionsResult{Items: make([]application.PushSubscriptionDTO, 0)}
	for rows.Next() {
		item, err := scanPushSubscription(rows)
		if err != nil {
			return application.ListPushSubscriptionsResult{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return application.ListPushSubscriptionsResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (repository *Repository) UpdatePushSubscription(ctx context.Context, command application.UpdatePushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.UserID <= 0 ||
		command.SubscriptionID <= 0 || command.ExpectedVersion <= 0 || command.UpdatedAt.IsZero() || len(command.MonitorIDs) == 0 {
		return application.PushSubscriptionDTO{}, sharedrepository.ErrInvalidInput
	}
	var result application.PushSubscriptionDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		if err := verifyPushUserAndMonitors(transactionContext, transaction.SQL, command.UserID, command.MonitorIDs); err != nil {
			return err
		}
		updateResult, err := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions SET version=version+1,device_label=$1,timezone=$2,quiet_start=$3::time,
    quiet_end=$4::time,ttl_seconds=$5,updated_at=$6
WHERE id=$7 AND user_id=$8 AND version=$9 AND status='active'`, command.DeviceLabel, command.Timezone,
			command.QuietStart, command.QuietEnd, command.TTLSeconds, command.UpdatedAt,
			command.SubscriptionID, command.UserID, command.ExpectedVersion)
		if err != nil {
			return databaserepository.MapError(err)
		}
		if affected, err := updateResult.RowsAffected(); err != nil || affected != 1 {
			return sharedrepository.ErrConflict
		}
		if _, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM web_push_subscription_monitors WHERE subscription_id=$1`, command.SubscriptionID); err != nil {
			return databaserepository.MapError(err)
		}
		for _, monitorID := range command.MonitorIDs {
			if _, err := transaction.SQL.ExecContext(transactionContext, `
INSERT INTO web_push_subscription_monitors(subscription_id,monitor_id) VALUES ($1,$2)`, command.SubscriptionID, monitorID); err != nil {
				return databaserepository.MapError(err)
			}
		}
		result, err = readPushSubscription(transactionContext, transaction.SQL, command.SubscriptionID, command.UserID)
		return err
	})
	if err != nil {
		return application.PushSubscriptionDTO{}, err
	}
	return result, nil
}

func (repository *Repository) DisablePushSubscription(ctx context.Context, command application.DisablePushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	if repository == nil || repository.runtime == nil || repository.runtime.SQL == nil || command.UserID <= 0 ||
		command.SubscriptionID <= 0 || command.ExpectedVersion <= 0 || command.DisabledAt.IsZero() {
		return application.PushSubscriptionDTO{}, sharedrepository.ErrInvalidInput
	}
	var result application.PushSubscriptionDTO
	err := repository.runtime.WithinTransaction(ctx, func(transactionContext context.Context, transaction database.Transaction) error {
		updateResult, err := transaction.SQL.ExecContext(transactionContext, `
UPDATE web_push_subscriptions SET version=version+1,status='disabled',updated_at=$1
WHERE id=$2 AND user_id=$3 AND version=$4 AND status='active'`, command.DisabledAt, command.SubscriptionID,
			command.UserID, command.ExpectedVersion)
		if err != nil {
			return databaserepository.MapError(err)
		}
		if affected, err := updateResult.RowsAffected(); err != nil || affected != 1 {
			return sharedrepository.ErrConflict
		}
		if _, err := transaction.SQL.ExecContext(transactionContext, `
DELETE FROM notification_delivery_claims
WHERE channel='web_push' AND delivery_target_key='push_subscription:' || ($1::bigint)::text`, command.SubscriptionID); err != nil {
			return databaserepository.MapError(err)
		}
		result, err = readPushSubscription(transactionContext, transaction.SQL, command.SubscriptionID, command.UserID)
		return err
	})
	if err != nil {
		return application.PushSubscriptionDTO{}, err
	}
	return result, nil
}

func verifyPushUserAndMonitors(ctx context.Context, queryer pushSubscriptionQueryer, userID int64, monitorIDs []int64) error {
	var count int
	encoded, err := json.Marshal(monitorIDs)
	if err != nil {
		return sharedrepository.ErrInvalidInput
	}
	if err := queryer.QueryRowContext(ctx, `
SELECT count(DISTINCT monitor.id)
FROM users AS actor
JOIN monitors AS monitor ON monitor.created_by=actor.id
WHERE actor.id=$1 AND actor.status='active' AND actor.deleted_at IS NULL
  AND monitor.deleted_at IS NULL AND monitor.status<>'archived'
  AND monitor.id IN (SELECT jsonb_array_elements_text($2::jsonb)::bigint)`, userID, encoded).Scan(&count); err != nil {
		return databaserepository.MapError(err)
	}
	if count != len(monitorIDs) {
		return sharedrepository.ErrNotFound
	}
	return nil
}
