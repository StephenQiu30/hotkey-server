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
  SELECT request.position,
         request.subject_key AS requested_subject_key,
         request.input_digest AS requested_input_digest,
         decision.*
  FROM requested AS request
  JOIN source_rights_decisions AS decision
    ON decision.source_connection_id=$1
   AND (
        (decision.subject_type='raw_response'
         AND decision.subject_key=request.subject_key
         AND decision.input_digest=request.input_digest)
        OR
        (decision.subject_type='source_endpoint'
         AND decision.subject_key=($1::bigint)::text
         AND EXISTS (
           SELECT 1 FROM source_rights_policies AS policy
           WHERE policy.id=decision.policy_id
             AND policy.policy_hash=decision.input_digest
         ))
   )
   AND decision.action IN ('store_raw','retain')
  WHERE decision.effective_from <= $4
    AND (decision.expires_at IS NULL OR $4 < decision.expires_at)
    AND NOT EXISTS (
      SELECT 1 FROM source_rights_decisions AS superseding
      WHERE superseding.supersedes_decision_id=decision.id
        AND superseding.effective_from <= $4
    )
), highest_priority AS (
  SELECT position,requested_subject_key,requested_input_digest,action,max(priority_rank) AS priority_rank
  FROM terminal
  GROUP BY position,requested_subject_key,requested_input_digest,action
), highest_terminal AS (
  SELECT terminal.*
  FROM terminal
  JOIN highest_priority AS highest
    ON highest.position=terminal.position
   AND highest.requested_subject_key=terminal.requested_subject_key
   AND highest.requested_input_digest=terminal.requested_input_digest
   AND highest.action=terminal.action
   AND highest.priority_rank=terminal.priority_rank
), allowed_groups AS (
  SELECT position,requested_subject_key,requested_input_digest,action
  FROM highest_terminal
  GROUP BY position,requested_subject_key,requested_input_digest,action
  HAVING bool_and(decision='allow')
), selected AS (
  SELECT DISTINCT ON (terminal.position,terminal.requested_subject_key,terminal.requested_input_digest,terminal.action)
    terminal.*
  FROM highest_terminal AS terminal
  JOIN allowed_groups AS allowed
    ON allowed.position=terminal.position
   AND allowed.requested_subject_key=terminal.requested_subject_key
   AND allowed.requested_input_digest=terminal.requested_input_digest
   AND allowed.action=terminal.action
  WHERE terminal.decision='allow'
  ORDER BY terminal.position,terminal.requested_subject_key,terminal.requested_input_digest,terminal.action,
           CASE WHEN terminal.action='retain' THEN terminal.retention_days END ASC NULLS LAST,
           terminal.id DESC
)
SELECT requested_subject_key,requested_input_digest,
       id,source_connection_id,policy_id,policy_revision,policy_scope_type,policy_scope_subject,
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
		requestedKey, requestedDigest, record, err := scanRequestedRightsDecisionRecord(rows)
		if err != nil {
			return sourceapplication.CurrentRawEvidenceRightsResult{}, databaserepository.MapError(err)
		}
		entity, err := record.entity()
		if err != nil {
			return sourceapplication.CurrentRawEvidenceRightsResult{}, fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
		}
		dto := rawEvidenceRightsDecisionDTO(entity)
		dto.AuthorizedEvidenceKey = requestedKey
		dto.AuthorizedPayloadSHA256 = requestedDigest
		switch entity.Action {
		case domain.RightsActionStoreRaw:
			result.StoreRawDecisions[requestedKey] = dto
		case domain.RightsActionRetain:
			result.RetainDecisions[requestedKey] = dto
		default:
			return sourceapplication.CurrentRawEvidenceRightsResult{}, fmt.Errorf("%w: unexpected raw evidence rights action", sharedrepository.ErrConstraint)
		}
	}
	if err := rows.Err(); err != nil {
		return sourceapplication.CurrentRawEvidenceRightsResult{}, databaserepository.MapError(err)
	}
	return result, nil
}

func scanRequestedRightsDecisionRecord(scanner rightsDecisionScanner) (string, string, rightsDecisionRecord, error) {
	var requestedKey string
	var requestedDigest string
	var record rightsDecisionRecord
	err := scanner.Scan(
		&requestedKey, &requestedDigest,
		&record.ID, &record.SourceConnectionID, &record.PolicyID, &record.PolicyRevision,
		&record.PolicyScopeType, &record.PolicyScopeSubject, &record.PriorityRank,
		&record.BasisSummary, &record.TermsURL, &record.LicenseURI,
		&record.SubjectType, &record.SubjectKey, &record.InputDigest,
		&record.Action, &record.Decision, &record.ReasonCodesJSON, &record.Evaluator,
		&record.EvaluatedAt, &record.EffectiveFrom, &record.ExpiresAt,
		&record.RetentionDays, &record.SupersedesDecisionID,
	)
	return requestedKey, requestedDigest, record, err
}
