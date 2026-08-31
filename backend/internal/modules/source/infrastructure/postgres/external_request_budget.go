package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type ExternalRequestBudget struct {
	runtime *database.Runtime
}

var _ domain.ExternalRequestBudget = (*ExternalRequestBudget)(nil)

func NewExternalRequestBudget(runtime *database.Runtime) *ExternalRequestBudget {
	return &ExternalRequestBudget{runtime: runtime}
}

func (budget *ExternalRequestBudget) CheckExternalRequest(ctx context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetAvailability, error) {
	if budget == nil || budget.runtime == nil || budget.runtime.SQL == nil {
		return domain.ExternalRequestBudgetAvailability{}, sharedrepository.ErrUnavailable
	}
	if err := reservation.Validate(); err != nil {
		return domain.ExternalRequestBudgetAvailability{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	at := reservation.At.UTC()
	budgetDay := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	rateWindow := at.Truncate(time.Minute)
	availability := domain.ExternalRequestBudgetAvailability{Allowed: true, ResetAt: budgetDay.AddDate(0, 0, 1)}
	queryer := externalRequestBudgetQueryer(budget.runtime.SQL)
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		queryer = transaction.SQL
	}
	var persistedRateWindow time.Time
	err := queryer.QueryRowContext(ctx, `
SELECT used, rate_window_start, rate_used
FROM source_request_usage_ledgers
WHERE source_connection_id=$1 AND resource_profile_version=$2 AND budget_day=$3`, reservation.SourceConnectionID, reservation.ResourceProfileVersion, budgetDay.Format(time.DateOnly)).Scan(&availability.Used, &persistedRateWindow, &availability.RateUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return availability, nil
	}
	if err != nil {
		return domain.ExternalRequestBudgetAvailability{}, databaserepository.MapError(err)
	}
	if !persistedRateWindow.Equal(rateWindow) {
		availability.RateUsed = 0
	}
	switch {
	case availability.Used >= reservation.DailyLimit:
		availability.Allowed = false
	case availability.RateUsed >= reservation.PerMinuteLimit:
		availability.Allowed = false
		availability.ResetAt = rateWindow.Add(time.Minute)
	}
	if err := availability.Validate(reservation); err != nil {
		return domain.ExternalRequestBudgetAvailability{}, fmt.Errorf("%w: %w", sharedrepository.ErrConstraint, err)
	}
	return availability, nil
}

func (budget *ExternalRequestBudget) ReserveExternalRequest(ctx context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	if budget == nil || budget.runtime == nil || budget.runtime.SQL == nil {
		return domain.ExternalRequestBudgetDecision{}, sharedrepository.ErrUnavailable
	}
	if err := reservation.Validate(); err != nil {
		return domain.ExternalRequestBudgetDecision{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	at := reservation.At.UTC()
	windowStart := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	rateWindowStart := at.Truncate(time.Minute)
	decision := domain.ExternalRequestBudgetDecision{}
	reserve := func(transactionCtx context.Context, transaction *sql.Tx) error {
		return reserveSourceRequest(transactionCtx, transaction, reservation, windowStart, rateWindowStart, &decision)
	}
	var err error
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		err = reserve(ctx, transaction.SQL)
	} else {
		err = budget.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, transaction database.Transaction) error {
			return reserve(transactionCtx, transaction.SQL)
		})
	}
	if err != nil {
		return domain.ExternalRequestBudgetDecision{}, err
	}
	if err := decision.Validate(reservation.DailyLimit, reservation.PerMinuteLimit); err != nil {
		return domain.ExternalRequestBudgetDecision{}, fmt.Errorf("%w: %w", sharedrepository.ErrConstraint, err)
	}
	return decision, nil
}

func reserveSourceRequest(ctx context.Context, transaction *sql.Tx, reservation domain.ExternalRequestBudgetReservation, budgetDay, rateWindow time.Time, decision *domain.ExternalRequestBudgetDecision) error {
	lockSubject := fmt.Sprintf("%d:%s", reservation.SourceConnectionID, reservation.ResourceProfileVersion)
	if _, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockSubject); err != nil {
		return databaserepository.MapError(err)
	}
	var used, rateUsed int64
	var persistedRateWindow time.Time
	err := transaction.QueryRowContext(ctx, `
SELECT used, rate_window_start, rate_used
FROM source_request_usage_ledgers
WHERE source_connection_id=$1 AND resource_profile_version=$2 AND budget_day=$3
FOR UPDATE`, reservation.SourceConnectionID, reservation.ResourceProfileVersion, budgetDay.Format(time.DateOnly)).Scan(&used, &persistedRateWindow, &rateUsed)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = transaction.ExecContext(ctx, `
INSERT INTO source_request_usage_ledgers
    (source_connection_id, resource_profile_version, budget_day, used, rate_window_start, rate_used)
VALUES ($1, $2, $3, 1, $4, 1)`, reservation.SourceConnectionID, reservation.ResourceProfileVersion, budgetDay.Format(time.DateOnly), rateWindow)
		if err != nil {
			return databaserepository.MapError(err)
		}
		*decision = domain.ExternalRequestBudgetDecision{Allowed: true, Used: 1, RateUsed: 1, ResetAt: budgetDay.AddDate(0, 0, 1)}
		return nil
	}
	if err != nil {
		return databaserepository.MapError(err)
	}
	currentRateUsed := int64(0)
	if persistedRateWindow.Equal(rateWindow) {
		currentRateUsed = rateUsed
	}
	if used >= reservation.DailyLimit {
		*decision = domain.ExternalRequestBudgetDecision{Allowed: false, Used: used, RateUsed: currentRateUsed, ResetAt: budgetDay.AddDate(0, 0, 1)}
		return nil
	}
	if currentRateUsed >= reservation.PerMinuteLimit {
		*decision = domain.ExternalRequestBudgetDecision{Allowed: false, Used: used, RateUsed: currentRateUsed, ResetAt: rateWindow.Add(time.Minute)}
		return nil
	}
	nextRateUsed := currentRateUsed + 1
	if _, err := transaction.ExecContext(ctx, `
UPDATE source_request_usage_ledgers
SET used=used+1, rate_window_start=$4, rate_used=$5, updated_at=now()
WHERE source_connection_id=$1 AND resource_profile_version=$2 AND budget_day=$3`, reservation.SourceConnectionID, reservation.ResourceProfileVersion, budgetDay.Format(time.DateOnly), rateWindow, nextRateUsed); err != nil {
		return databaserepository.MapError(err)
	}
	*decision = domain.ExternalRequestBudgetDecision{Allowed: true, Used: used + 1, RateUsed: nextRateUsed, ResetAt: budgetDay.AddDate(0, 0, 1)}
	return nil
}

type externalRequestBudgetQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
