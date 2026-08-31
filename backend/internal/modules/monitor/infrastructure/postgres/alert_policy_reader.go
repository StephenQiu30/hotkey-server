package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

// AlertPolicyReader returns only active published policy snapshots. It never
// exposes draft rules, source configuration or Monitor credentials.
type AlertPolicyReader struct{ runtime *database.Runtime }

var _ application.AlertPolicyReader = (*AlertPolicyReader)(nil)

func NewAlertPolicyReader(runtime *database.Runtime) *AlertPolicyReader {
	return &AlertPolicyReader{runtime: runtime}
}

func (reader *AlertPolicyReader) ListPublishedAlertPolicies(ctx context.Context, monitorIDs []int64) ([]application.PublishedAlertPolicy, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return nil, sharedrepository.ErrUnavailable
	}
	if len(monitorIDs) == 0 {
		return []application.PublishedAlertPolicy{}, nil
	}
	seen := make(map[int64]struct{}, len(monitorIDs))
	ids := make([]int64, 0, len(monitorIDs))
	for _, monitorID := range monitorIDs {
		if monitorID <= 0 {
			return nil, fmt.Errorf("%w: monitor ids must be positive", sharedrepository.ErrInvalidInput)
		}
		if _, exists := seen[monitorID]; exists {
			continue
		}
		seen[monitorID] = struct{}{}
		ids = append(ids, monitorID)
	}
	encodedIDs, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("encode monitor ids: %w", err)
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT monitor.id, config.id, config.revision, config.config_hash, config.event_threshold,
       config.alert_min_heat, config.alert_min_momentum, config.alert_min_breadth,
       config.alert_warning_threshold, config.alert_critical_threshold, config.alert_cooldown_minutes,
       config.alert_email_enabled, config.alert_email_min_severity, coalesce(owner.email, '')
FROM monitors AS monitor
JOIN monitor_config_versions AS config
  ON config.id = monitor.published_config_version_id
 AND config.monitor_id = monitor.id
 AND config.state = 'published'
LEFT JOIN users AS owner ON owner.id = monitor.created_by AND owner.status = 'active' AND owner.deleted_at IS NULL
WHERE monitor.status = 'active'
  AND monitor.deleted_at IS NULL
  AND monitor.id IN (
      SELECT jsonb_array_elements_text($1::jsonb)::bigint
  )
ORDER BY monitor.id ASC`, encodedIDs)
	if err != nil {
		return nil, databaserepository.MapError(err)
	}
	defer func() { _ = rows.Close() }()
	policies := make([]application.PublishedAlertPolicy, 0, len(ids))
	for rows.Next() {
		var policy application.PublishedAlertPolicy
		if err := rows.Scan(&policy.MonitorID, &policy.ConfigVersionID, &policy.Revision, &policy.ConfigHash, &policy.EventThreshold, &policy.AlertMinHeat, &policy.AlertMinMomentum, &policy.AlertMinBreadth, &policy.AlertWarningThreshold, &policy.AlertCriticalThreshold, &policy.AlertCooldownMinutes, &policy.AlertEmailEnabled, &policy.AlertEmailMinSeverity, &policy.RecipientEmail); err != nil {
			return nil, databaserepository.MapError(err)
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, databaserepository.MapError(err)
	}
	return policies, nil
}
