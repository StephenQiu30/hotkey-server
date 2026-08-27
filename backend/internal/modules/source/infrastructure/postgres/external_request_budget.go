package postgres

import (
	"context"
	"database/sql"
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

func (budget *ExternalRequestBudget) ReserveExternalRequest(ctx context.Context, reservation domain.ExternalRequestBudgetReservation) (domain.ExternalRequestBudgetDecision, error) {
	if budget == nil || budget.runtime == nil || budget.runtime.SQL == nil {
		return domain.ExternalRequestBudgetDecision{}, sharedrepository.ErrUnavailable
	}
	if err := reservation.Validate(); err != nil {
		return domain.ExternalRequestBudgetDecision{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	at := reservation.At.UTC()
	windowStart := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	decision := domain.ExternalRequestBudgetDecision{ResetAt: windowStart.AddDate(0, 0, 1)}
	queryer := externalRequestBudgetQueryer(budget.runtime.SQL)
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		queryer = transaction.SQL
	}
	err := queryer.QueryRowContext(ctx, `
WITH reserved AS (
    INSERT INTO source_request_usage_ledgers
        (source_connection_id, resource_profile_version, budget_day, used)
    VALUES ($1, $2, $3, 1)
    ON CONFLICT (source_connection_id, resource_profile_version, budget_day) DO UPDATE
    SET used = source_request_usage_ledgers.used + 1, updated_at = now()
    WHERE source_request_usage_ledgers.used < $4
    RETURNING used
)
SELECT true, used FROM reserved
UNION ALL
SELECT false, ledger.used
FROM source_request_usage_ledgers AS ledger
WHERE ledger.source_connection_id = $1
  AND ledger.resource_profile_version = $2
  AND ledger.budget_day = $3
  AND NOT EXISTS (SELECT 1 FROM reserved)
LIMIT 1`, reservation.SourceConnectionID, reservation.ResourceProfileVersion, windowStart.Format(time.DateOnly), reservation.DailyLimit).Scan(&decision.Allowed, &decision.Used)
	if err != nil {
		return domain.ExternalRequestBudgetDecision{}, databaserepository.MapError(err)
	}
	if err := decision.Validate(reservation.DailyLimit); err != nil {
		return domain.ExternalRequestBudgetDecision{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return decision, nil
}

type externalRequestBudgetQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
