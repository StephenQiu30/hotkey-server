package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/pagination"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const rightsProjectionMaximumCursorSize = 4096

var _ sourceapplication.RightsManagementProjectionRepository = (*RightsManagementRepository)(nil)

var rightsActionProjectionOrder = []string{
	"fetch", "store_raw", "store_derived", "display_private", "redistribute",
	"quote", "embed_local", "send_external_model", "retain",
}

func (repository *RightsManagementRepository) FindSourceEndpointCapabilityFacts(ctx context.Context, sourceEndpointID int64) (sourceapplication.SourceEndpointCapabilityFactsDTO, error) {
	if !repository.available() {
		return sourceapplication.SourceEndpointCapabilityFactsDTO{}, sharedrepository.ErrUnavailable
	}
	if sourceEndpointID <= 0 {
		return sourceapplication.SourceEndpointCapabilityFactsDTO{}, fmt.Errorf("%w: source endpoint identity is invalid", sharedrepository.ErrInvalidInput)
	}
	var facts sourceapplication.SourceEndpointCapabilityFactsDTO
	var deletedAt sql.NullTime
	err := repository.queryRow(ctx, `
SELECT id,source_type,enabled,health_status,deleted_at
FROM source_connections
WHERE id=$1 AND deleted_at IS NULL`, sourceEndpointID).Scan(
		&facts.SourceEndpointID, &facts.SourceType, &facts.Enabled, &facts.HealthStatus, &deletedAt,
	)
	if err != nil {
		return sourceapplication.SourceEndpointCapabilityFactsDTO{}, mapRightsManagementDatabaseError(err)
	}
	facts.Deleted = deletedAt.Valid
	return facts, nil
}

func (repository *RightsManagementRepository) ListRightsPolicies(ctx context.Context, query sourceapplication.ListRightsPoliciesRepositoryDTO) (sourceapplication.ListRightsPoliciesRepositoryResultDTO, error) {
	if !repository.available() {
		return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, sharedrepository.ErrUnavailable
	}
	limit, cursorID, fingerprint, err := rightsProjectionPageParameters("policies", query.SourceEndpointID, query.Cursor, query.Limit)
	if err != nil {
		return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, err
	}
	if err := repository.ensureRightsSourceEndpoint(ctx, query.SourceEndpointID); err != nil {
		return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, err
	}
	rows, err := repository.queryRightsRows(ctx, `
SELECT `+rightsPolicyReadColumns+`
FROM source_rights_policies
WHERE (source_connection_id=$1 OR scope_type='organization_default')
  AND ($2::bigint=0 OR id < $2)
ORDER BY id DESC
LIMIT $3`, query.SourceEndpointID, cursorID, limit+1)
	if err != nil {
		return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
	}
	defer rows.Close()
	items := make([]sourceapplication.RightsPolicyReadDTO, 0, limit+1)
	for rows.Next() {
		record, err := scanRightsPolicyRead(rows)
		if err != nil {
			return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
		}
		items = append(items, record.applicationDTO())
	}
	if err := rows.Err(); err != nil {
		return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
	}
	result := sourceapplication.ListRightsPoliciesRepositoryResultDTO{Items: items}
	if len(items) > limit {
		result.Items = items[:limit]
		result.NextCursor, err = pagination.Encode("id", true, fingerprint, result.Items[len(result.Items)-1].ID)
		if err != nil {
			return sourceapplication.ListRightsPoliciesRepositoryResultDTO{}, fmt.Errorf("%w: encode rights policy cursor: %v", sharedrepository.ErrConstraint, err)
		}
	}
	return result, nil
}

func (repository *RightsManagementRepository) ListRightsDecisionBatches(ctx context.Context, query sourceapplication.ListRightsDecisionBatchesRepositoryDTO) (sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO, error) {
	if !repository.available() {
		return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, sharedrepository.ErrUnavailable
	}
	limit, cursorID, fingerprint, err := rightsProjectionPageParameters("decision-batches", query.SourceEndpointID, query.Cursor, query.Limit)
	if err != nil {
		return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, err
	}
	if err := repository.ensureRightsSourceEndpoint(ctx, query.SourceEndpointID); err != nil {
		return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, err
	}
	rows, err := repository.queryRightsRows(ctx, `
WITH selected_batches AS (
  SELECT id,version,source_connection_id,policy_id,expected_policy_version,
         subject_type,subject_key,input_digest,recorded_by_user_id,decision_count,created_at
  FROM source_rights_decision_batches
  WHERE source_connection_id=$1 AND ($2::bigint=0 OR id < $2)
  ORDER BY id DESC
  LIMIT $3
)
SELECT batch.id,batch.version,batch.source_connection_id,batch.policy_id,batch.expected_policy_version,
       batch.subject_type,batch.subject_key,batch.input_digest,batch.recorded_by_user_id,
       batch.decision_count,batch.created_at,
       `+rightsDecisionReadColumns+`
FROM selected_batches AS batch
JOIN source_rights_decisions AS decision ON decision.decision_batch_id=batch.id
ORDER BY batch.id DESC,decision.action,decision.id`, query.SourceEndpointID, cursorID, limit+1)
	if err != nil {
		return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
	}
	defer rows.Close()

	type accumulatedBatch struct {
		record    rightsDecisionBatchReadRecord
		decisions []sourceapplication.RightsDecisionReadDTO
	}
	accumulated := make([]accumulatedBatch, 0, limit+1)
	for rows.Next() {
		var batchRecord rightsDecisionBatchReadRecord
		var decisionRecord rightsDecisionReadRecord
		targets := append(rightsDecisionBatchReadScanTargets(&batchRecord), rightsDecisionReadScanTargets(&decisionRecord)...)
		if err := rows.Scan(targets...); err != nil {
			return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
		}
		decision, err := decisionRecord.applicationDTO()
		if err != nil {
			return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		if len(accumulated) == 0 || accumulated[len(accumulated)-1].record.ID != batchRecord.ID {
			accumulated = append(accumulated, accumulatedBatch{record: batchRecord, decisions: []sourceapplication.RightsDecisionReadDTO{decision}})
		} else {
			accumulated[len(accumulated)-1].decisions = append(accumulated[len(accumulated)-1].decisions, decision)
		}
	}
	if err := rows.Err(); err != nil {
		return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, mapRightsManagementDatabaseError(err)
	}

	result := sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{Items: make([]sourceapplication.RightsDecisionBatchDTO, 0, minInt(len(accumulated), limit))}
	visible := accumulated
	if len(visible) > limit {
		visible = visible[:limit]
	}
	for _, batch := range visible {
		if len(batch.decisions) != batch.record.DecisionCount {
			return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, fmt.Errorf("%w: rights decision batch projection is incomplete", sharedrepository.ErrConstraint)
		}
		result.Items = append(result.Items, batch.record.applicationDTO(batch.decisions))
	}
	if len(accumulated) > limit {
		result.NextCursor, err = pagination.Encode("id", true, fingerprint, result.Items[len(result.Items)-1].ID)
		if err != nil {
			return sourceapplication.ListRightsDecisionBatchesRepositoryResultDTO{}, fmt.Errorf("%w: encode rights decision batch cursor: %v", sharedrepository.ErrConstraint, err)
		}
	}
	return result, nil
}

func (repository *RightsManagementRepository) FindRightsDecisionRead(ctx context.Context, query sourceapplication.FindRightsDecisionReadRepositoryDTO) (sourceapplication.RightsDecisionReadDTO, error) {
	if !repository.available() {
		return sourceapplication.RightsDecisionReadDTO{}, sharedrepository.ErrUnavailable
	}
	if query.SourceEndpointID <= 0 || query.DecisionID <= 0 {
		return sourceapplication.RightsDecisionReadDTO{}, fmt.Errorf("%w: rights decision identity is invalid", sharedrepository.ErrInvalidInput)
	}
	var record rightsDecisionReadRecord
	err := repository.queryRow(ctx, `
SELECT `+rightsDecisionReadColumns+`
FROM source_rights_decisions AS decision
JOIN source_rights_decision_batches AS batch ON batch.id=decision.decision_batch_id
WHERE decision.id=$1 AND decision.source_connection_id=$2`, query.DecisionID, query.SourceEndpointID).Scan(rightsDecisionReadScanTargets(&record)...)
	if err != nil {
		return sourceapplication.RightsDecisionReadDTO{}, mapRightsManagementDatabaseError(err)
	}
	result, err := record.applicationDTO()
	if err != nil {
		return sourceapplication.RightsDecisionReadDTO{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return result, nil
}

func (repository *RightsManagementRepository) EvaluateRightsActionMatrix(ctx context.Context, query sourceapplication.EvaluateRightsActionMatrixRepositoryDTO) (sourceapplication.RightsActionMatrixDTO, error) {
	if !repository.available() {
		return sourceapplication.RightsActionMatrixDTO{}, sharedrepository.ErrUnavailable
	}
	if !validRightsActionEvaluationQuery(query) {
		return sourceapplication.RightsActionMatrixDTO{}, fmt.Errorf("%w: exact rights evaluation input is invalid", sharedrepository.ErrInvalidInput)
	}
	if err := repository.ensureRightsSourceEndpoint(ctx, query.SourceEndpointID); err != nil {
		return sourceapplication.RightsActionMatrixDTO{}, err
	}
	rows, err := repository.queryRightsRows(ctx, `
WITH terminal AS (
  SELECT decision.*
  FROM source_rights_decisions AS decision
  WHERE decision.source_connection_id=$1
    AND decision.subject_type=$2
    AND decision.subject_key=$3
    AND decision.input_digest=$4
    AND decision.effective_from <= $5
    AND (decision.expires_at IS NULL OR $5 < decision.expires_at)
    AND NOT EXISTS (
      SELECT 1 FROM source_rights_decisions AS superseding
      WHERE superseding.supersedes_decision_id=decision.id
        AND superseding.effective_from <= $5
    )
), highest AS (
  SELECT action,max(priority_rank) AS priority_rank
  FROM terminal
  GROUP BY action
)
SELECT terminal.action,terminal.id,terminal.policy_id,terminal.priority_rank,
       terminal.decision,terminal.retention_days
FROM terminal
JOIN highest USING (action,priority_rank)
ORDER BY CASE terminal.action
  WHEN 'fetch' THEN 1 WHEN 'store_raw' THEN 2 WHEN 'store_derived' THEN 3
  WHEN 'display_private' THEN 4 WHEN 'redistribute' THEN 5 WHEN 'quote' THEN 6
  WHEN 'embed_local' THEN 7 WHEN 'send_external_model' THEN 8 WHEN 'retain' THEN 9
  ELSE 10 END,terminal.id`, query.SourceEndpointID, query.SubjectType, query.SubjectKey, query.InputDigest, query.At.UTC())
	if err != nil {
		return sourceapplication.RightsActionMatrixDTO{}, mapRightsManagementDatabaseError(err)
	}
	defer rows.Close()
	records := make(map[string][]rightsActionEvaluationRecord, len(rightsActionProjectionOrder))
	for rows.Next() {
		var record rightsActionEvaluationRecord
		if err := rows.Scan(&record.Action, &record.DecisionID, &record.PolicyID, &record.Priority, &record.Decision, &record.RetentionDays); err != nil {
			return sourceapplication.RightsActionMatrixDTO{}, mapRightsManagementDatabaseError(err)
		}
		records[record.Action] = append(records[record.Action], record)
	}
	if err := rows.Err(); err != nil {
		return sourceapplication.RightsActionMatrixDTO{}, mapRightsManagementDatabaseError(err)
	}

	result := sourceapplication.RightsActionMatrixDTO{
		SourceEndpointID: query.SourceEndpointID, EvaluatedAt: query.At.UTC(),
		Actions: make([]sourceapplication.RightsActionCapabilityDTO, 0, len(rightsActionProjectionOrder)),
	}
	for _, action := range rightsActionProjectionOrder {
		item, err := projectRightsActionEvaluation(action, records[action])
		if err != nil {
			return sourceapplication.RightsActionMatrixDTO{}, err
		}
		result.Actions = append(result.Actions, item)
	}
	return result, nil
}

func validRightsActionEvaluationQuery(query sourceapplication.EvaluateRightsActionMatrixRepositoryDTO) bool {
	if query.SourceEndpointID <= 0 || query.SubjectKey == "" || len(query.SubjectKey) > 512 ||
		query.SubjectKey != strings.TrimSpace(query.SubjectKey) || strings.ContainsAny(query.SubjectKey, "\x00\r\n") ||
		len(query.InputDigest) != 64 || query.InputDigest != strings.ToLower(query.InputDigest) || query.At.IsZero() {
		return false
	}
	switch query.SubjectType {
	case "raw_response", "source_observation", "document_version":
	default:
		return false
	}
	decoded, err := hex.DecodeString(query.InputDigest)
	return err == nil && len(decoded) == 32
}

func projectRightsActionEvaluation(action string, records []rightsActionEvaluationRecord) (sourceapplication.RightsActionCapabilityDTO, error) {
	result := sourceapplication.RightsActionCapabilityDTO{
		Action: action, Decision: "unknown", DecisionIDs: []int64{}, PolicyIDs: []int64{},
	}
	if len(records) == 0 {
		return result, nil
	}
	priority := records[0].Priority
	states := make(map[string]struct{}, 3)
	policies := make(map[int64]struct{}, len(records))
	retentionDays := 0
	for _, record := range records {
		if record.Action != action || record.DecisionID <= 0 || record.PolicyID <= 0 || record.Priority != priority {
			return sourceapplication.RightsActionCapabilityDTO{}, fmt.Errorf("%w: exact rights evaluation record is invalid", sharedrepository.ErrConstraint)
		}
		switch record.Decision {
		case "allow", "deny", "unknown":
		default:
			return sourceapplication.RightsActionCapabilityDTO{}, fmt.Errorf("%w: exact rights evaluation state is invalid", sharedrepository.ErrConstraint)
		}
		states[record.Decision] = struct{}{}
		result.DecisionIDs = append(result.DecisionIDs, record.DecisionID)
		policies[record.PolicyID] = struct{}{}
		if action == "retain" && record.Decision == "allow" && record.RetentionDays.Valid &&
			(retentionDays == 0 || int(record.RetentionDays.Int64) < retentionDays) {
			retentionDays = int(record.RetentionDays.Int64)
		}
	}
	result.Priority = &priority
	for policyID := range policies {
		result.PolicyIDs = append(result.PolicyIDs, policyID)
	}
	sort.Slice(result.PolicyIDs, func(left, right int) bool { return result.PolicyIDs[left] < result.PolicyIDs[right] })
	_, hasDeny := states["deny"]
	_, hasAllow := states["allow"]
	_, hasUnknown := states["unknown"]
	switch {
	case hasDeny || hasAllow && hasUnknown:
		result.Decision = "deny"
	case hasAllow:
		result.Decision = "allow"
	default:
		result.Decision = "unknown"
	}
	if action == "retain" && result.Decision == "allow" {
		if retentionDays < 1 || retentionDays > 3650 {
			return sourceapplication.RightsActionCapabilityDTO{}, fmt.Errorf("%w: exact retain evaluation is invalid", sharedrepository.ErrConstraint)
		}
		result.RetentionDays = &retentionDays
	}
	return result, nil
}

func rightsProjectionPageParameters(kind string, sourceEndpointID int64, encodedCursor string, limit int) (int, int64, string, error) {
	if sourceEndpointID <= 0 || limit < 1 || limit > 100 || len(encodedCursor) > rightsProjectionMaximumCursorSize || strings.TrimSpace(encodedCursor) != encodedCursor {
		return 0, 0, "", fmt.Errorf("%w: rights projection page is invalid", sharedrepository.ErrInvalidInput)
	}
	fingerprint := fmt.Sprintf("source-endpoint:%d:rights:%s", sourceEndpointID, kind)
	cursor, err := pagination.Decode(encodedCursor, "id", true, fingerprint)
	if err != nil {
		return 0, 0, "", fmt.Errorf("%w: rights projection cursor: %v", sharedrepository.ErrInvalidInput, err)
	}
	return limit, cursor.ID, fingerprint, nil
}

func (repository *RightsManagementRepository) ensureRightsSourceEndpoint(ctx context.Context, sourceEndpointID int64) error {
	var exists bool
	if err := repository.queryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_connections WHERE id=$1)`, sourceEndpointID).Scan(&exists); err != nil {
		return mapRightsManagementDatabaseError(err)
	}
	if !exists {
		return fmt.Errorf("%w: source endpoint does not exist", sharedrepository.ErrNotFound)
	}
	return nil
}

func (repository *RightsManagementRepository) queryRightsRows(ctx context.Context, query string, arguments ...any) (*sql.Rows, error) {
	if transaction, found := database.TransactionFromContext(ctx); found {
		return transaction.SQL.QueryContext(ctx, query, arguments...)
	}
	return repository.runtime.SQL.QueryContext(ctx, query, arguments...)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
