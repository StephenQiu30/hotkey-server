package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
)

const rightsPolicyReadColumns = `
id,version,source_connection_id,scope_type,scope_subject,policy_revision,priority,
basis_summary,terms_url,license_uri,policy_hash,effective_at,expires_at,parent_policy_id,
recorded_by_user_id,approved_by_user_id,created_at`

const rightsDecisionReadColumns = `
decision.id,decision.decision_batch_id,decision.source_connection_id,decision.policy_id,
decision.policy_revision,decision.policy_scope_type,decision.policy_scope_subject,
decision.priority_rank,decision.basis_summary,decision.terms_url,decision.license_uri,
decision.subject_type,decision.subject_key,decision.input_digest,decision.action,
decision.decision,array_to_json(decision.reason_codes)::text,decision.evaluator,
decision.evaluated_at,decision.effective_from,decision.expires_at,decision.retention_days,
decision.supersedes_decision_id,batch.recorded_by_user_id,decision.created_at`

type rightsPolicyReadRecord struct {
	ID               int64
	Version          int64
	SourceEndpointID sql.NullInt64
	ScopeType        string
	ScopeSubject     string
	PolicyRevision   int64
	Priority         int
	BasisSummary     string
	TermsURL         sql.NullString
	LicenseURI       sql.NullString
	PolicyHash       string
	EffectiveAt      time.Time
	ExpiresAt        sql.NullTime
	ParentPolicyID   sql.NullInt64
	RecordedByUserID int64
	ApprovedByUserID sql.NullInt64
	CreatedAt        time.Time
}

type rightsDecisionBatchReadRecord struct {
	ID                    int64
	Version               int64
	SourceEndpointID      int64
	PolicyID              int64
	ExpectedPolicyVersion int64
	SubjectType           string
	SubjectKey            string
	InputDigest           string
	RecordedByUserID      int64
	DecisionCount         int
	CreatedAt             time.Time
}

type rightsDecisionReadRecord struct {
	ID                   int64
	DecisionBatchID      int64
	SourceEndpointID     int64
	PolicyID             int64
	PolicyRevision       int64
	PolicyScopeType      string
	PolicyScopeSubject   string
	Priority             int
	BasisSummary         string
	TermsURL             sql.NullString
	LicenseURI           sql.NullString
	SubjectType          string
	SubjectKey           string
	InputDigest          string
	Action               string
	Decision             string
	ReasonCodesJSON      []byte
	Evaluator            string
	EvaluatedAt          time.Time
	EffectiveFrom        time.Time
	ExpiresAt            sql.NullTime
	RetentionDays        sql.NullInt64
	SupersedesDecisionID sql.NullInt64
	RecordedByUserID     int64
	CreatedAt            time.Time
}

type rightsActionEvaluationRecord struct {
	Action        string
	DecisionID    int64
	PolicyID      int64
	Priority      int
	Decision      string
	RetentionDays sql.NullInt64
}

func scanRightsPolicyRead(scanner rightsManagementScanner) (rightsPolicyReadRecord, error) {
	var record rightsPolicyReadRecord
	err := scanner.Scan(
		&record.ID, &record.Version, &record.SourceEndpointID, &record.ScopeType, &record.ScopeSubject,
		&record.PolicyRevision, &record.Priority, &record.BasisSummary, &record.TermsURL, &record.LicenseURI,
		&record.PolicyHash, &record.EffectiveAt, &record.ExpiresAt, &record.ParentPolicyID,
		&record.RecordedByUserID, &record.ApprovedByUserID, &record.CreatedAt,
	)
	return record, err
}

func rightsDecisionBatchReadScanTargets(record *rightsDecisionBatchReadRecord) []any {
	return []any{
		&record.ID, &record.Version, &record.SourceEndpointID, &record.PolicyID, &record.ExpectedPolicyVersion,
		&record.SubjectType, &record.SubjectKey, &record.InputDigest, &record.RecordedByUserID,
		&record.DecisionCount, &record.CreatedAt,
	}
}

func rightsDecisionReadScanTargets(record *rightsDecisionReadRecord) []any {
	return []any{
		&record.ID, &record.DecisionBatchID, &record.SourceEndpointID, &record.PolicyID,
		&record.PolicyRevision, &record.PolicyScopeType, &record.PolicyScopeSubject, &record.Priority,
		&record.BasisSummary, &record.TermsURL, &record.LicenseURI, &record.SubjectType,
		&record.SubjectKey, &record.InputDigest, &record.Action, &record.Decision,
		&record.ReasonCodesJSON, &record.Evaluator, &record.EvaluatedAt, &record.EffectiveFrom,
		&record.ExpiresAt, &record.RetentionDays, &record.SupersedesDecisionID,
		&record.RecordedByUserID, &record.CreatedAt,
	}
}

func (record rightsPolicyReadRecord) applicationDTO() sourceapplication.RightsPolicyReadDTO {
	return sourceapplication.RightsPolicyReadDTO{
		ID: record.ID, Version: record.Version, SourceEndpointID: nullableRightsManagementInt64(record.SourceEndpointID),
		ScopeType: record.ScopeType, ScopeSubject: record.ScopeSubject, Revision: record.PolicyRevision,
		Priority: record.Priority, BasisSummary: record.BasisSummary, TermsURL: record.TermsURL.String,
		LicenseURI: record.LicenseURI.String, PolicyHash: record.PolicyHash, EffectiveFrom: record.EffectiveAt.UTC(),
		ExpiresAt: nullableRightsManagementTime(record.ExpiresAt), ParentPolicyID: nullableRightsManagementInt64(record.ParentPolicyID),
		RecordedByUserID: record.RecordedByUserID, ApprovedByUserID: nullableRightsManagementInt64(record.ApprovedByUserID),
		CreatedAt: record.CreatedAt.UTC(),
	}
}

func (record rightsDecisionReadRecord) applicationDTO() (sourceapplication.RightsDecisionReadDTO, error) {
	reasonCodes := make([]string, 0)
	if err := json.Unmarshal(record.ReasonCodesJSON, &reasonCodes); err != nil {
		return sourceapplication.RightsDecisionReadDTO{}, fmt.Errorf("decode rights decision reason codes: %w", err)
	}
	var retentionDays *int
	if record.RetentionDays.Valid {
		if record.RetentionDays.Int64 < 1 || record.RetentionDays.Int64 > 3650 {
			return sourceapplication.RightsDecisionReadDTO{}, fmt.Errorf("rights decision retention duration is invalid")
		}
		value := int(record.RetentionDays.Int64)
		retentionDays = &value
	}
	return sourceapplication.RightsDecisionReadDTO{
		ID: record.ID, DecisionBatchID: record.DecisionBatchID, SourceEndpointID: record.SourceEndpointID,
		PolicyID: record.PolicyID, PolicyRevision: record.PolicyRevision,
		PolicyScopeType: record.PolicyScopeType, PolicyScopeSubject: record.PolicyScopeSubject,
		Priority: record.Priority, BasisSummary: record.BasisSummary, TermsURL: record.TermsURL.String,
		LicenseURI: record.LicenseURI.String, SubjectType: record.SubjectType, SubjectKey: record.SubjectKey,
		InputDigest: record.InputDigest, Action: record.Action, Decision: record.Decision,
		ReasonCodes: reasonCodes, Evaluator: record.Evaluator, EvaluatedAt: record.EvaluatedAt.UTC(),
		EffectiveFrom: record.EffectiveFrom.UTC(), ExpiresAt: nullableRightsManagementTime(record.ExpiresAt),
		RetentionDays: retentionDays, SupersedesDecisionID: nullableRightsManagementInt64(record.SupersedesDecisionID),
		RecordedByUserID: record.RecordedByUserID, CreatedAt: record.CreatedAt.UTC(),
	}, nil
}

func (record rightsDecisionBatchReadRecord) applicationDTO(decisions []sourceapplication.RightsDecisionReadDTO) sourceapplication.RightsDecisionBatchDTO {
	return sourceapplication.RightsDecisionBatchDTO{
		ID: record.ID, Version: record.Version, SourceEndpointID: record.SourceEndpointID,
		PolicyID: record.PolicyID, ExpectedPolicyVersion: record.ExpectedPolicyVersion,
		SubjectType: record.SubjectType, SubjectKey: record.SubjectKey, InputDigest: record.InputDigest,
		RecordedByUserID: record.RecordedByUserID, DecisionCount: record.DecisionCount,
		CreatedAt: record.CreatedAt.UTC(), Decisions: decisions,
	}
}
