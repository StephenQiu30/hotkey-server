package postgres

import (
	"context"
	"database/sql"
	"fmt"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type publishedMatchQueryExecutor interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type PublishedMatchTargetReader struct {
	runtime *database.Runtime
}

var _ ingestionapplication.PublishedMatchTargetReader = (*PublishedMatchTargetReader)(nil)

func NewPublishedMatchTargetReader(runtime *database.Runtime) (*PublishedMatchTargetReader, error) {
	if runtime == nil || runtime.SQL == nil {
		return nil, fmt.Errorf("published match target database runtime is required")
	}
	return &PublishedMatchTargetReader{runtime: runtime}, nil
}

func (reader *PublishedMatchTargetReader) ReadPublishedMatchTargets(ctx context.Context, query ingestionapplication.PublishedMatchTargetsQuery) (ingestionapplication.PublishedMatchTargetsResult, error) {
	if reader == nil || reader.runtime == nil || query.DocumentVersionID <= 0 || query.TriggerMonitorVersionID < 0 {
		return ingestionapplication.PublishedMatchTargetsResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var executor publishedMatchQueryExecutor = reader.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		executor = transaction.SQL
	}
	rows, err := executor.QueryContext(ctx, `
WITH trigger_document AS (
    SELECT version.id AS document_version_id,document.source_connection_id
    FROM document_versions AS version
    JOIN documents AS document ON document.id=version.document_id AND document.document_state='active'
    WHERE version.id=$1
      AND version.lifecycle_state IN ('derived_available','readable')
)
SELECT monitor.id,config.id,profile.id,relevance.id
FROM trigger_document AS trigger
JOIN monitor_sources AS source ON source.source_connection_id=trigger.source_connection_id AND source.enabled
JOIN monitor_config_versions AS config ON config.id=source.config_version_id AND config.state='published'
JOIN monitors AS monitor ON monitor.id=config.monitor_id AND monitor.status='active'
    AND monitor.deleted_at IS NULL AND monitor.published_config_version_id=config.id
	AND ($2::bigint=0 OR config.id=$2)
JOIN monitor_compiled_profiles AS profile ON profile.monitor_id=monitor.id
    AND profile.purpose='published' AND profile.config_version_id=config.id
    AND profile.monitor_version_id=config.id AND profile.status='ready'
    AND EXISTS (
        SELECT 1 FROM document_version_search_indexes AS search
        WHERE search.document_version_id=trigger.document_version_id
          AND search.source_connection_id=trigger.source_connection_id
          AND search.normalization_profile_version=profile.search_normalization_profile_version
          AND search.lifecycle_state='active' AND search.retention_until>CURRENT_TIMESTAMP
          AND current_rights_action_allowed(
              search.store_derived_rights_decision_id,search.source_connection_id,
              'document_version',search.document_version_id::text,search.normalized_text_sha256,
              'store_derived',CURRENT_TIMESTAMP
          )
          AND current_rights_action_allowed(
              search.retain_rights_decision_id,search.source_connection_id,
              'document_version',search.document_version_id::text,search.normalized_text_sha256,
              'retain',CURRENT_TIMESTAMP
          )
    )
JOIN LATERAL (
    SELECT candidate.id
    FROM relevance_decision_profiles AS candidate
    WHERE candidate.matching_algorithm_version=profile.matching_algorithm_version
      AND candidate.status IN ('active','shadow','uncalibrated')
    ORDER BY CASE candidate.status WHEN 'active' THEN 0 WHEN 'shadow' THEN 1 ELSE 2 END,candidate.id DESC
    LIMIT 1
) AS relevance ON true
ORDER BY monitor.id ASC,config.id ASC,profile.id ASC`, query.DocumentVersionID, query.TriggerMonitorVersionID)
	if err != nil {
		return ingestionapplication.PublishedMatchTargetsResult{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := ingestionapplication.PublishedMatchTargetsResult{Targets: []ingestionapplication.PublishedMatchTargetDTO{}}
	for rows.Next() {
		var record publishedMatchTargetRecord
		if err := rows.Scan(&record.MonitorID, &record.MonitorVersionID, &record.CompiledProfileID, &record.RelevanceProfileID); err != nil {
			return ingestionapplication.PublishedMatchTargetsResult{}, databaserepository.MapError(err)
		}
		mapped, err := record.dto()
		if err != nil {
			return ingestionapplication.PublishedMatchTargetsResult{}, err
		}
		result.Targets = append(result.Targets, mapped)
	}
	if err := rows.Err(); err != nil {
		return ingestionapplication.PublishedMatchTargetsResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func (reader *PublishedMatchTargetReader) ReadPublishedMonitorTrigger(ctx context.Context, query ingestionapplication.ReadPublishedMonitorTriggerQuery) (ingestionapplication.ReadPublishedMonitorTriggerResult, error) {
	if reader == nil || reader.runtime == nil || query.MonitorID <= 0 || query.MonitorVersionID <= 0 || query.CompiledProfileID <= 0 {
		return ingestionapplication.ReadPublishedMonitorTriggerResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	var executor publishedMatchQueryExecutor = reader.runtime.SQL
	if transaction, ok := database.TransactionFromContext(ctx); ok {
		executor = transaction.SQL
	}
	rows, err := executor.QueryContext(ctx, `
SELECT version.id
FROM monitors AS monitor
JOIN monitor_config_versions AS config ON config.id=$2 AND config.monitor_id=monitor.id AND config.state='published'
JOIN monitor_compiled_profiles AS profile ON profile.id=$3 AND profile.monitor_id=monitor.id
    AND profile.config_version_id=config.id AND profile.monitor_version_id=config.id
    AND profile.purpose='published' AND profile.status='ready'
JOIN monitor_sources AS source ON source.config_version_id=config.id AND source.enabled
JOIN documents AS document ON document.source_connection_id=source.source_connection_id AND document.document_state='active'
JOIN document_versions AS version ON version.document_id=document.id
    AND version.lifecycle_state IN ('derived_available','readable')
JOIN document_version_search_indexes AS search ON search.document_version_id=version.id
    AND search.source_connection_id=document.source_connection_id
    AND search.normalization_profile_version=profile.search_normalization_profile_version
    AND search.lifecycle_state='active' AND search.retention_until>CURRENT_TIMESTAMP
    AND current_rights_action_allowed(
        search.store_derived_rights_decision_id,search.source_connection_id,
        'document_version',search.document_version_id::text,search.normalized_text_sha256,
        'store_derived',CURRENT_TIMESTAMP
    )
    AND current_rights_action_allowed(
        search.retain_rights_decision_id,search.source_connection_id,
        'document_version',search.document_version_id::text,search.normalized_text_sha256,
        'retain',CURRENT_TIMESTAMP
    )
WHERE monitor.id=$1 AND monitor.status='active' AND monitor.deleted_at IS NULL
  AND monitor.published_config_version_id=config.id
ORDER BY version.id ASC
LIMIT 1`, query.MonitorID, query.MonitorVersionID, query.CompiledProfileID)
	if err != nil {
		return ingestionapplication.ReadPublishedMonitorTriggerResult{}, databaserepository.MapError(err)
	}
	defer rows.Close()
	result := ingestionapplication.ReadPublishedMonitorTriggerResult{}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return result, databaserepository.MapError(err)
		}
		return result, nil
	}
	if err := rows.Scan(&result.DocumentVersionID); err != nil {
		return ingestionapplication.ReadPublishedMonitorTriggerResult{}, databaserepository.MapError(err)
	}
	if result.DocumentVersionID <= 0 || rows.Next() {
		return ingestionapplication.ReadPublishedMonitorTriggerResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	result.Exists = true
	return result, nil
}

type publishedMatchTargetRecord struct {
	MonitorID, MonitorVersionID, CompiledProfileID, RelevanceProfileID sql.NullInt64
}

func (record publishedMatchTargetRecord) dto() (ingestionapplication.PublishedMatchTargetDTO, error) {
	values := []sql.NullInt64{record.MonitorID, record.MonitorVersionID, record.CompiledProfileID, record.RelevanceProfileID}
	for _, value := range values {
		if !value.Valid || value.Int64 <= 0 {
			return ingestionapplication.PublishedMatchTargetDTO{}, fmt.Errorf("%w: stored published match target is invalid", sharedrepository.ErrConstraint)
		}
	}
	return ingestionapplication.PublishedMatchTargetDTO{
		MonitorID: record.MonitorID.Int64, MonitorVersionID: record.MonitorVersionID.Int64,
		CompiledProfileID: record.CompiledProfileID.Int64, RelevanceProfileID: record.RelevanceProfileID.Int64,
	}, nil
}
