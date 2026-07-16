package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	ingestiondomain "github.com/StephenQiu30/hotkey-server/internal/modules/ingestion/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

// RelevanceCandidateReader owns the two SQL-backed recall paths and the one
// capped batch configuration load. Vector recall belongs to intelligence and
// is intentionally absent from this adapter.
type RelevanceCandidateReader struct{ runtime *database.Runtime }

var _ ingestiondomain.RelevanceCandidateReader = (*RelevanceCandidateReader)(nil)

func NewRelevanceCandidateReader(runtime *database.Runtime) *RelevanceCandidateReader {
	return &RelevanceCandidateReader{runtime: runtime}
}

func (reader *RelevanceCandidateReader) SourceCandidates(ctx context.Context, sourceConnectionID int64, limit int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	if !reader.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	if sourceConnectionID <= 0 || limit < 1 || limit > 8 {
		return nil, fmt.Errorf("%w: bounded source candidate query", sharedrepository.ErrInvalidInput)
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
WITH source_candidates AS MATERIALIZED (
    SELECT source.config_version_id, source.priority
    FROM monitor_sources AS source
    WHERE source.source_connection_id=$1 AND source.enabled
    ORDER BY source.priority ASC, source.config_version_id ASC
), eligible AS MATERIALIZED (
    SELECT source.priority, (
        SELECT monitor.id
        FROM monitors AS monitor
        WHERE monitor.id=(
            SELECT config.monitor_id
            FROM monitor_config_versions AS config
            WHERE config.id=source.config_version_id AND config.state='published'
        )
          AND monitor.status='active' AND monitor.deleted_at IS NULL
          AND monitor.published_config_version_id=source.config_version_id
    ) AS monitor_id
    FROM source_candidates AS source
)
SELECT monitor_id
FROM eligible
WHERE monitor_id IS NOT NULL
ORDER BY priority ASC, monitor_id ASC
LIMIT $2`, sourceConnectionID, limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	return scanSourceCandidateHits(rows)
}

func (reader *RelevanceCandidateReader) LexicalCandidates(ctx context.Context, terms []string, limit int) ([]ingestiondomain.RelevanceCandidateHit, error) {
	if !reader.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	terms = normalizedLookupTerms(terms)
	if len(terms) == 0 || limit < 1 || limit > 12 {
		return nil, fmt.Errorf("%w: bounded lexical candidate query", sharedrepository.ErrInvalidInput)
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
WITH matched_rules AS (
    SELECT rule.config_version_id, MAX(
        CASE WHEN rule.origin='ai' THEN LEAST(60::numeric, GREATEST(0::numeric, rule.weight))
             ELSE LEAST(100::numeric, GREATEST(0::numeric, rule.weight)) END
    ) AS lexical_score
    FROM monitor_rules AS rule
    WHERE rule.enabled AND rule.approval_status='approved'
      AND rule.rule_type IN ('keyword','phrase','entity','exclude_keyword')
      AND lower(rule.value)=ANY($1::text[])
    GROUP BY rule.config_version_id
)
SELECT monitor.id, matched_rules.lexical_score::float8
FROM matched_rules
JOIN monitor_config_versions AS config ON config.id=matched_rules.config_version_id
JOIN monitors AS monitor ON monitor.id=config.monitor_id
WHERE monitor.status='active' AND monitor.deleted_at IS NULL
  AND monitor.published_config_version_id=config.id AND config.state='published'
ORDER BY matched_rules.lexical_score DESC, monitor.id ASC
LIMIT $2`, terms, limit)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()
	return scanLexicalCandidateHits(rows)
}

func (reader *RelevanceCandidateReader) LoadRelevanceCandidates(ctx context.Context, monitorIDs []int64) ([]ingestiondomain.RelevanceCandidate, error) {
	if !reader.available() {
		return nil, sharedrepository.ErrUnavailable
	}
	monitorIDs = uniquePositiveIDs(monitorIDs)
	if len(monitorIDs) == 0 || len(monitorIDs) > 20 {
		return nil, fmt.Errorf("%w: capped relevance candidate batch", sharedrepository.ErrInvalidInput)
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
SELECT
    monitor.id, config.id, config.config_hash, config.relevance_threshold,
    array_to_json(config.languages), array_to_json(config.regions),
    rule.id, rule.rule_type, rule.operator, rule.value, rule.weight, rule.priority, rule.origin
FROM monitors AS monitor
JOIN monitor_config_versions AS config ON config.id=monitor.published_config_version_id
LEFT JOIN monitor_rules AS rule
  ON rule.config_version_id=config.id AND rule.enabled AND rule.approval_status='approved'
WHERE monitor.id=ANY($1::bigint[])
  AND monitor.status='active' AND monitor.deleted_at IS NULL AND config.state='published'
ORDER BY monitor.id ASC, rule.priority ASC, rule.id ASC`, monitorIDs)
	if err != nil {
		return nil, sharedrepository.MapError(err)
	}
	defer rows.Close()

	byID := map[int64]*ingestiondomain.RelevanceCandidate{}
	for rows.Next() {
		var candidate ingestiondomain.RelevanceCandidate
		var languagesJSON, regionsJSON []byte
		var ruleID sql.NullInt64
		var ruleType, operator, value, origin sql.NullString
		var weight sql.NullFloat64
		var priority sql.NullInt16
		if err := rows.Scan(&candidate.MonitorID, &candidate.MonitorConfigVersionID, &candidate.ConfigHash, &candidate.RelevanceThreshold,
			&languagesJSON, &regionsJSON, &ruleID, &ruleType, &operator, &value, &weight, &priority, &origin); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		stored, exists := byID[candidate.MonitorID]
		if !exists {
			if err := json.Unmarshal(languagesJSON, &candidate.Languages); err != nil {
				return nil, fmt.Errorf("%w: relevance candidate languages", sharedrepository.ErrConstraint)
			}
			if err := json.Unmarshal(regionsJSON, &candidate.Regions); err != nil {
				return nil, fmt.Errorf("%w: relevance candidate regions", sharedrepository.ErrConstraint)
			}
			candidate.Rules = []ingestiondomain.RelevanceRule{}
			stored = &candidate
			byID[candidate.MonitorID] = stored
		}
		if ruleID.Valid {
			stored.Rules = append(stored.Rules, ingestiondomain.RelevanceRule{ID: ruleID.Int64, RuleType: ruleType.String, Operator: operator.String,
				Value: value.String, Weight: weight.Float64, Priority: priority.Int16, Origin: origin.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	result := make([]ingestiondomain.RelevanceCandidate, 0, len(byID))
	for _, candidate := range byID {
		result = append(result, *candidate)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].MonitorID < result[right].MonitorID })
	return result, nil
}

func scanSourceCandidateHits(rows *sql.Rows) ([]ingestiondomain.RelevanceCandidateHit, error) {
	result := []ingestiondomain.RelevanceCandidateHit{}
	for rows.Next() {
		var hit ingestiondomain.RelevanceCandidateHit
		if err := rows.Scan(&hit.MonitorID); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		result = append(result, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}

func scanLexicalCandidateHits(rows *sql.Rows) ([]ingestiondomain.RelevanceCandidateHit, error) {
	result := []ingestiondomain.RelevanceCandidateHit{}
	for rows.Next() {
		var hit ingestiondomain.RelevanceCandidateHit
		if err := rows.Scan(&hit.MonitorID, &hit.LexicalScore); err != nil {
			return nil, sharedrepository.MapError(err)
		}
		result = append(result, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, sharedrepository.MapError(err)
	}
	return result, nil
}

func normalizedLookupTerms(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > 160 {
			continue
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func uniquePositiveIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func (reader *RelevanceCandidateReader) available() bool {
	return reader != nil && reader.runtime != nil && reader.runtime.SQL != nil
}
