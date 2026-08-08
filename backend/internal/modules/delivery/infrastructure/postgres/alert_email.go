package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/delivery/domain"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const alertDeliveryColumns = `id, occurrence_id, idempotency_key, recipient, subject, text_body, html_body, severity, status, next_attempt_at, succeeded_at, coalesce(last_error, ''), (SELECT count(*) FROM alert_email_attempts attempt WHERE attempt.delivery_id = alert_email_deliveries.id AND attempt.status = 'started')`

func (repository *Repository) CreateAlertDelivery(ctx context.Context, delivery domain.AlertDelivery) (domain.AlertDelivery, bool, error) {
	if repository == nil || repository.runtime == nil {
		return domain.AlertDelivery{}, false, sharedrepository.ErrUnavailable
	}
	if err := delivery.ValidateForCreate(); err != nil {
		return domain.AlertDelivery{}, false, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	created, err := scanAlertDelivery(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
INSERT INTO alert_email_deliveries (occurrence_id,idempotency_key,recipient,subject,text_body,html_body,severity,status)
VALUES ($1,$2,$3,$4,$5,$6,$7,'queued')
ON CONFLICT (occurrence_id) DO NOTHING
RETURNING `+alertDeliveryColumns, delivery.OccurrenceID, delivery.IdempotencyKey, delivery.Recipient, delivery.Subject, delivery.TextBody, delivery.HTMLBody, delivery.Severity))
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.AlertDelivery{}, false, err
	}
	existing, err := repository.GetAlertDeliveryForOccurrence(ctx, delivery.OccurrenceID)
	return existing, false, err
}

func (repository *Repository) GetAlertDeliveryForOccurrence(ctx context.Context, occurrenceID int64) (domain.AlertDelivery, error) {
	if repository == nil || repository.runtime == nil || occurrenceID <= 0 {
		return domain.AlertDelivery{}, sharedrepository.ErrInvalidInput
	}
	return scanAlertDelivery(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT `+alertDeliveryColumns+` FROM alert_email_deliveries WHERE occurrence_id=$1`, occurrenceID))
}

func (repository *Repository) GetAlertDelivery(ctx context.Context, id int64) (domain.AlertDelivery, error) {
	if repository == nil || repository.runtime == nil || id <= 0 {
		return domain.AlertDelivery{}, sharedrepository.ErrInvalidInput
	}
	return scanAlertDelivery(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `SELECT `+alertDeliveryColumns+` FROM alert_email_deliveries WHERE id=$1`, id))
}

func (repository *Repository) ClaimAlertDelivery(ctx context.Context, id int64) (domain.AlertDelivery, error) {
	if repository == nil || repository.runtime == nil || id <= 0 {
		return domain.AlertDelivery{}, sharedrepository.ErrInvalidInput
	}
	delivery, err := scanAlertDelivery(deliveryQueryerFor(ctx, repository.runtime).QueryRowContext(ctx, `
UPDATE alert_email_deliveries SET status='claimed',updated_at=now()
WHERE id=$1 AND status IN ('queued','retrying') AND (next_attempt_at IS NULL OR next_attempt_at <= now())
RETURNING `+alertDeliveryColumns, id))
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return domain.AlertDelivery{}, sharedrepository.ErrConflict
	}
	return delivery, err
}

func (repository *Repository) UpdateAlertDelivery(ctx context.Context, delivery domain.AlertDelivery) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	if err := delivery.Validate(); err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	result, err := deliveryQueryerFor(ctx, repository.runtime).ExecContext(ctx, `UPDATE alert_email_deliveries SET status=$1,next_attempt_at=$2,succeeded_at=$3,last_error=NULLIF($4,''),updated_at=now() WHERE id=$5 AND occurrence_id=$6`, delivery.Status, delivery.NextAttemptAt, delivery.SucceededAt, delivery.LastError, delivery.ID, delivery.OccurrenceID)
	if err != nil {
		return databaserepository.MapError(err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return sharedrepository.ErrNotFound
	}
	return nil
}

func (repository *Repository) AppendAlertAttempt(ctx context.Context, deliveryID int64, attemptNo int, status, message string) error {
	if repository == nil || repository.runtime == nil || deliveryID <= 0 || attemptNo < 1 || attemptNo > 5 || status != "started" && status != "succeeded" && status != "failed" {
		return sharedrepository.ErrInvalidInput
	}
	_, err := deliveryQueryerFor(ctx, repository.runtime).ExecContext(ctx, `INSERT INTO alert_email_attempts (delivery_id,attempt_no,status,error) VALUES ($1,$2,$3,NULLIF($4,'')) ON CONFLICT (delivery_id,attempt_no,status) DO NOTHING`, deliveryID, attemptNo, status, message)
	return databaserepository.MapError(err)
}

type alertDeliveryScanner interface{ Scan(...any) error }

func scanAlertDelivery(scanner alertDeliveryScanner) (domain.AlertDelivery, error) {
	var delivery domain.AlertDelivery
	var next, succeeded sql.NullTime
	if err := scanner.Scan(&delivery.ID, &delivery.OccurrenceID, &delivery.IdempotencyKey, &delivery.Recipient, &delivery.Subject, &delivery.TextBody, &delivery.HTMLBody, &delivery.Severity, &delivery.Status, &next, &succeeded, &delivery.LastError, &delivery.AttemptCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.AlertDelivery{}, sharedrepository.ErrNotFound
		}
		return domain.AlertDelivery{}, databaserepository.MapError(err)
	}
	if next.Valid {
		value := next.Time.UTC()
		delivery.NextAttemptAt = &value
	}
	if succeeded.Valid {
		value := succeeded.Time.UTC()
		delivery.SucceededAt = &value
	}
	return delivery, nil
}
