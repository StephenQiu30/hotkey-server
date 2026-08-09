package postgres

import (
	"context"
	"fmt"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	databaserepository "github.com/StephenQiu30/hotkey-server/backend/internal/platform/database/repository"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RightsDecisionReader struct {
	runtime *database.Runtime
}

var _ sourceapplication.CurrentRawEvidenceRightsReader = (*RightsDecisionReader)(nil)

func NewRightsDecisionReader(runtime *database.Runtime) *RightsDecisionReader {
	return &RightsDecisionReader{runtime: runtime}
}

// ResolveCurrent evaluates the complete bounded batch in one PostgreSQL
// statement, so store_raw and conservative retain selections share a single
// MVCC snapshot. The insert/commit triggers re-evaluate them at write time.
func (reader *RightsDecisionReader) ResolveCurrent(ctx context.Context, query sourceapplication.CurrentRawEvidenceRightsQuery) (sourceapplication.CurrentRawEvidenceRightsResult, error) {
	if reader == nil || reader.runtime == nil || reader.runtime.SQL == nil {
		return sourceapplication.CurrentRawEvidenceRightsResult{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return sourceapplication.CurrentRawEvidenceRightsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	keys := make([]string, len(query.Subjects))
	digests := make([]string, len(query.Subjects))
	for index, subject := range query.Subjects {
		keys[index], digests[index] = subject.EvidenceKey, subject.PayloadSHA256
	}
	rows, err := reader.runtime.SQL.QueryContext(ctx, `
WITH requested AS (
  SELECT subject_key,input_digest,position
  FROM unnest($2::text[],$3::text[]) WITH ORDINALITY AS request(subject_key,input_digest,position)
), terminal AS (
  SELECT request.position,decision.*
  FROM requested AS request
  JOIN source_rights_decisions AS decision
    ON decision.source_connection_id=$1
   AND decision.subject_type='raw_response'
   AND decision.subject_key=request.subject_key
   AND decision.input_digest=request.input_digest
   AND decision.action IN ('store_raw','retain')
  WHERE decision.effective_from <= $4
    AND (decision.expires_at IS NULL OR $4 < decision.expires_at)
    AND NOT EXISTS (
      SELECT 1 FROM source_rights_decisions AS superseding
      WHERE superseding.supersedes_decision_id=decision.id
        AND superseding.effective_from <= $4
    )
), highest_priority AS (
  SELECT position,subject_key,input_digest,action,max(priority_rank) AS priority_rank
  FROM terminal
  GROUP BY position,subject_key,input_digest,action
), highest_terminal AS (
  SELECT terminal.*
  FROM terminal
  JOIN highest_priority AS highest
    ON highest.position=terminal.position
   AND highest.subject_key=terminal.subject_key
   AND highest.input_digest=terminal.input_digest
   AND highest.action=terminal.action
   AND highest.priority_rank=terminal.priority_rank
), allowed_groups AS (
  SELECT position,subject_key,input_digest,action
  FROM highest_terminal
  GROUP BY position,subject_key,input_digest,action
  HAVING bool_and(decision='allow')
), selected AS (
  SELECT DISTINCT ON (terminal.position,terminal.subject_key,terminal.input_digest,terminal.action)
    terminal.*
  FROM highest_terminal AS terminal
  JOIN allowed_groups AS allowed
    ON allowed.position=terminal.position
   AND allowed.subject_key=terminal.subject_key
   AND allowed.input_digest=terminal.input_digest
   AND allowed.action=terminal.action
  WHERE terminal.decision='allow'
  ORDER BY terminal.position,terminal.subject_key,terminal.input_digest,terminal.action,
           CASE WHEN terminal.action='retain' THEN terminal.retention_days END ASC NULLS LAST,
           terminal.id DESC
)
SELECT id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
       priority_rank,basis_summary,terms_url,license_uri,subject_type,subject_key,input_digest,
       action,decision,array_to_json(reason_codes)::text,evaluator,evaluated_at,effective_from,
       expires_at,retention_days,supersedes_decision_id
FROM selected
ORDER BY position,action`, query.SourceConnectionID, keys, digests, query.DecisionAt.UTC())
	if err != nil {
		return sourceapplication.CurrentRawEvidenceRightsResult{}, databaserepository.MapError(err)
	}
	defer rows.Close()

	result := sourceapplication.CurrentRawEvidenceRightsResult{
		StoreRawDecisions: make(map[string]sourceapplication.RawEvidenceRightsDecisionDTO, len(query.Subjects)),
		RetainDecisions:   make(map[string]sourceapplication.RawEvidenceRightsDecisionDTO, len(query.Subjects)),
	}
	for rows.Next() {
		record, err := scanRightsDecisionRecord(rows)
		if err != nil {
			return sourceapplication.CurrentRawEvidenceRightsResult{}, databaserepository.MapError(err)
		}
		entity, err := record.entity()
		if err != nil {
			return sourceapplication.CurrentRawEvidenceRightsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		dto := rawEvidenceRightsDecisionDTO(entity)
		switch entity.Action {
		case domain.RightsActionStoreRaw:
			result.StoreRawDecisions[entity.SubjectKey] = dto
		case domain.RightsActionRetain:
			result.RetainDecisions[entity.SubjectKey] = dto
		default:
			return sourceapplication.CurrentRawEvidenceRightsResult{}, fmt.Errorf("%w: unexpected raw evidence rights action", sharedrepository.ErrConstraint)
		}
	}
	if err := rows.Err(); err != nil {
		return sourceapplication.CurrentRawEvidenceRightsResult{}, databaserepository.MapError(err)
	}
	return result, nil
}
