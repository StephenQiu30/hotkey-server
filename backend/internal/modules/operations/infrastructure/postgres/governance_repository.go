package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type GovernanceRepository struct{ runtime *database.Runtime }

func NewGovernanceRepository(runtime *database.Runtime) *GovernanceRepository {
	return &GovernanceRepository{runtime: runtime}
}

type governanceQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (repository *GovernanceRepository) queryer(ctx context.Context) governanceQueryer {
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		return transaction.SQL
	}
	return repository.runtime.SQL
}

func (repository *GovernanceRepository) CheckActiveMonitor(ctx context.Context, monitorID int64) error {
	if repository == nil || repository.runtime == nil {
		return sharedrepository.ErrUnavailable
	}
	var used int64
	if err := repository.queryer(ctx).QueryRowContext(ctx, `
SELECT count(*) FROM monitors
WHERE deleted_at IS NULL AND status = 'active' AND id <> $1`, monitorID).Scan(&used); err != nil {
		return err
	}
	if used >= operationsdomain.ActiveMonitorLimit {
		return sharederrors.ProductQuotaExceeded(operationsdomain.DimensionActiveMonitors, operationsdomain.ActiveMonitorLimit, 0, nil)
	}
	return nil
}

func (repository *GovernanceRepository) RecordManualSearch(ctx context.Context, userID int64, now time.Time) error {
	if repository == nil || repository.runtime == nil || userID <= 0 {
		return sharedrepository.ErrUnavailable
	}
	windowStart := now.UTC().Truncate(24 * time.Hour)
	windowEnd := windowStart.Add(24 * time.Hour)
	_, err := repository.queryer(ctx).ExecContext(ctx, `
INSERT INTO quota_usage_ledgers (dimension, subject_type, subject_id, window_start, window_end, used)
VALUES ('manual_searches', 'user', $1, $2, $3, 1)
ON CONFLICT (dimension, subject_type, subject_id, window_start) DO UPDATE
SET used = quota_usage_ledgers.used + 1, updated_at = now()`, userID, windowStart, windowEnd)
	return err
}

func (repository *GovernanceRepository) UsageOverview(ctx context.Context, userID int64, now time.Time) (operationsdomain.UsageOverview, error) {
	if repository == nil || repository.runtime == nil || userID <= 0 {
		return operationsdomain.UsageOverview{}, sharedrepository.ErrUnavailable
	}
	now = now.UTC()
	start := now.Truncate(24 * time.Hour)
	end := start.Add(24 * time.Hour)
	queryer := repository.queryer(ctx)
	var active, manual, sourceCalls, aiTokens, deliveries int64
	if err := queryer.QueryRowContext(ctx, `SELECT count(*) FROM monitors WHERE deleted_at IS NULL AND status='active'`).Scan(&active); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	if err := queryer.QueryRowContext(ctx, `SELECT coalesce(sum(used),0) FROM quota_usage_ledgers WHERE dimension='manual_searches' AND subject_type='user' AND subject_id=$1 AND window_start=$2`, userID, start).Scan(&manual); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	if err := queryer.QueryRowContext(ctx, `SELECT count(*) FROM collection_runs WHERE created_at >= $1 AND created_at < $2`, start, end).Scan(&sourceCalls); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	if err := queryer.QueryRowContext(ctx, `SELECT coalesce(sum(tokens),0) FROM ai_runs WHERE created_at >= $1 AND created_at < $2`, start, end).Scan(&aiTokens); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	if err := queryer.QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM alert_email_deliveries WHERE created_at >= $1 AND created_at < $2) +
  (SELECT count(*) FROM report_deliveries WHERE created_at >= $1 AND created_at < $2)`, start, end).Scan(&deliveries); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	var reserved, settled, budget, consumed, aiRemaining string
	var limitedProfiles int64
	if err := queryer.QueryRowContext(ctx, `WITH usage AS (
  SELECT coalesce(sum(ledger.reserved_cost),0) reserved, coalesce(sum(ledger.settled_cost),0) settled
  FROM ai_budget_ledgers ledger JOIN ai_model_profiles profile ON profile.id=ledger.model_profile_id
  WHERE ledger.budget_day=$1::date AND profile.daily_budget IS NOT NULL
), budget AS (
  SELECT coalesce(sum(daily_budget),0) amount, count(*) total FROM ai_model_profiles
  WHERE enabled AND deleted_at IS NULL AND daily_budget IS NOT NULL
)
SELECT usage.reserved::text, usage.settled::text, budget.amount::text,
       (usage.reserved + usage.settled)::text,
       greatest(budget.amount - usage.reserved - usage.settled, 0)::text, budget.total
FROM usage CROSS JOIN budget`, start).Scan(&reserved, &settled, &budget, &consumed, &aiRemaining, &limitedProfiles); err != nil {
		return operationsdomain.UsageOverview{}, err
	}
	activeLimit, activeRemaining := strconv.FormatInt(operationsdomain.ActiveMonitorLimit, 10), strconv.FormatInt(max(0, operationsdomain.ActiveMonitorLimit-active), 10)
	aiLimit := budget
	aiCost := operationsdomain.UsageItem{Dimension: operationsdomain.DimensionAICost, Label: "AI 成本", Scope: "workspace", Mode: "observed", Unit: "USD", Used: consumed, Reserved: &reserved, Settled: &settled, ResetAt: &end}
	if limitedProfiles > 0 {
		aiCost.Mode, aiCost.Limit, aiCost.Remaining = "hard", &aiLimit, &aiRemaining
	}
	items := []operationsdomain.UsageItem{
		{Dimension: operationsdomain.DimensionActiveMonitors, Label: "活跃监控", Scope: "workspace", Mode: "hard", Unit: "个", Used: strconv.FormatInt(active, 10), Limit: &activeLimit, Remaining: &activeRemaining},
		{Dimension: operationsdomain.DimensionManualSearches, Label: "手动搜索", Scope: "user", Mode: "observed", Unit: "次", Used: strconv.FormatInt(manual, 10), ResetAt: &end},
		{Dimension: operationsdomain.DimensionSourceCalls, Label: "来源调用", Scope: "workspace", Mode: "observed", Unit: "次", Used: strconv.FormatInt(sourceCalls, 10), ResetAt: &end},
		{Dimension: operationsdomain.DimensionAITokens, Label: "AI Token", Scope: "workspace", Mode: "observed", Unit: "tokens", Used: strconv.FormatInt(aiTokens, 10), ResetAt: &end},
		aiCost,
		{Dimension: operationsdomain.DimensionNotificationDelivery, Label: "通知投递", Scope: "workspace", Mode: "observed", Unit: "次", Used: strconv.FormatInt(deliveries, 10), ResetAt: &end},
	}
	return operationsdomain.UsageOverview{GeneratedAt: now, Items: items}, nil
}

func (repository *GovernanceRepository) ListAudit(ctx context.Context, query operationsdomain.AuditQuery) (operationsdomain.AuditPage, error) {
	if repository == nil || repository.runtime == nil {
		return operationsdomain.AuditPage{}, sharedrepository.ErrUnavailable
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > 100 || query.Cursor < 0 || len(query.Action) > 120 || len(query.ResourceType) > 64 || (query.Result != "" && query.Result != "success" && query.Result != "failure" && query.Result != "denied") {
		return operationsdomain.AuditPage{}, fmt.Errorf("%w: invalid audit query", sharedrepository.ErrInvalidInput)
	}
	clauses := []string{"($1::bigint = 0 OR id < $1)"}
	args := []any{query.Cursor}
	for column, value := range map[string]string{"action": strings.TrimSpace(query.Action), "resource_type": strings.TrimSpace(query.ResourceType), "result": strings.TrimSpace(query.Result)} {
		if value == "" {
			continue
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	args = append(args, query.Limit+1)
	rows, err := repository.queryer(ctx).QueryContext(ctx, `SELECT id,actor_type,actor_id,action,resource_type,resource_id,result,coalesce(request_id,''),created_at
FROM audit_logs WHERE `+strings.Join(clauses, " AND ")+fmt.Sprintf(" ORDER BY id DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return operationsdomain.AuditPage{}, err
	}
	defer rows.Close()
	page := operationsdomain.AuditPage{Items: make([]operationsdomain.AuditRecord, 0, query.Limit)}
	for rows.Next() {
		var record operationsdomain.AuditRecord
		var actorID, resourceID sql.NullInt64
		if err := rows.Scan(&record.ID, &record.ActorType, &actorID, &record.Action, &record.ResourceType, &resourceID, &record.Result, &record.RequestID, &record.CreatedAt); err != nil {
			return operationsdomain.AuditPage{}, err
		}
		if actorID.Valid {
			value := actorID.Int64
			record.ActorID = &value
		}
		if resourceID.Valid {
			value := resourceID.Int64
			record.ResourceID = &value
		}
		page.Items = append(page.Items, record)
	}
	if err := rows.Err(); err != nil {
		return operationsdomain.AuditPage{}, err
	}
	if len(page.Items) > query.Limit {
		page.Items = page.Items[:query.Limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}
