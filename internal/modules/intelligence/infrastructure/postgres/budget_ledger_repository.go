package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

func reserveBudget(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, profile claimProfile, budgetDay string) (string, error) {
	if _, err := queryer.ExecContext(ctx, `
INSERT INTO ai_budget_ledgers (model_profile_id,budget_day,reserved_cost,settled_cost)
VALUES ($1,$2,0,0) ON CONFLICT (model_profile_id,budget_day) DO NOTHING`, profile.ID, budgetDay); err != nil {
		return "", fmt.Errorf("create AI budget ledger: %w", err)
	}
	var reserved, settled string
	var blocked bool
	if err := queryer.QueryRowContext(ctx, `
SELECT reserved_cost::text,settled_cost::text,overage_blocked
FROM ai_budget_ledgers WHERE model_profile_id = $1 AND budget_day = $2 FOR UPDATE`, profile.ID, budgetDay).Scan(&reserved, &settled, &blocked); err != nil {
		return "", fmt.Errorf("lock AI budget ledger: %w", err)
	}
	if blocked || exceedsBudget(reserved, settled, profile.MaxCost, profile.DailyBudget) {
		return "", intelligencedomain.NewError(intelligencedomain.CodeAIBudgetExhausted)
	}
	if _, err := queryer.ExecContext(ctx, `UPDATE ai_budget_ledgers SET reserved_cost = reserved_cost + $1::numeric, updated_at = now() WHERE model_profile_id = $2 AND budget_day = $3`, profile.MaxCost, profile.ID, budgetDay); err != nil {
		return "", fmt.Errorf("reserve AI budget: %w", err)
	}
	return profile.MaxCost, nil
}

func lockBudget(ctx context.Context, queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, profileID int64, budgetDay string) error {
	if _, err := queryer.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, fmt.Sprintf("ai-budget:%d:%s", profileID, budgetDay)); err != nil {
		return fmt.Errorf("lock AI budget: %w", err)
	}
	return nil
}

func exceedsBudget(reserved, settled, maximum string, daily sql.NullString) bool {
	if !daily.Valid {
		return false
	}
	current, ok := new(big.Rat).SetString(reserved)
	if !ok {
		return true
	}
	settledCost, ok := new(big.Rat).SetString(settled)
	if !ok {
		return true
	}
	maximumCost, ok := new(big.Rat).SetString(maximum)
	if !ok {
		return true
	}
	dailyCost, ok := new(big.Rat).SetString(daily.String)
	if !ok {
		return true
	}
	return current.Add(current, settledCost).Add(current, maximumCost).Cmp(dailyCost) > 0
}
