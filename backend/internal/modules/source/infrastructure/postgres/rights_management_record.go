package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
)

const rightsManagementPolicyColumns = `
id,version,recorded_by_user_id,idempotency_key,command_fingerprint,
source_connection_id,scope_type,scope_subject,policy_revision,priority,basis_summary,
terms_url,license_uri,policy_hash,parent_policy_id,approved_by_user_id,effective_at,expires_at`

const rightsManagementDecisionBatchColumns = `
id,version,source_connection_id,policy_id,expected_policy_version,
subject_type,subject_key,input_digest,recorded_by_user_id,idempotency_key,
command_fingerprint,decision_count,created_at`

const rightsManagementDecisionColumns = `
id,decision_batch_id,source_connection_id,policy_id,policy_revision,
policy_scope_type,policy_scope_subject,priority_rank,basis_summary,terms_url,license_uri,
subject_type,subject_key,input_digest,action,decision,array_to_json(reason_codes)::text,
evaluator,evaluated_at,effective_from,expires_at,retention_days,supersedes_decision_id`

type rightsManagementPolicyRecord struct {
	ID                 int64
	Version            int64
	RecordedByUserID   int64
	IdempotencyKey     string
	CommandFingerprint string
	SourceConnectionID sql.NullInt64
	ScopeType          string
	ScopeSubject       string
	PolicyRevision     int64
	Priority           int
	BasisSummary       string
	TermsURL           sql.NullString
	LicenseURI         sql.NullString
	PolicyHash         string
	ParentPolicyID     sql.NullInt64
	ApprovedByUserID   sql.NullInt64
	EffectiveAt        time.Time
	ExpiresAt          sql.NullTime
}

type rightsManagementDecisionBatchRecord struct {
	ID                    int64
	Version               int64
	SourceConnectionID    int64
	PolicyID              int64
	ExpectedPolicyVersion int64
	SubjectType           string
	SubjectKey            string
	InputDigest           string
	RecordedByUserID      int64
	IdempotencyKey        string
	CommandFingerprint    string
	DecisionCount         int
	CreatedAt             time.Time
}

type rightsManagementDecisionRecord struct {
	ID                   int64
	DecisionBatchID      int64
	SourceConnectionID   int64
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
}

type rightsManagementScanner interface {
	Scan(...any) error
}

func scanRightsManagementPolicy(scanner rightsManagementScanner) (rightsManagementPolicyRecord, error) {
	var record rightsManagementPolicyRecord
	err := scanner.Scan(
		&record.ID, &record.Version, &record.RecordedByUserID, &record.IdempotencyKey, &record.CommandFingerprint,
		&record.SourceConnectionID, &record.ScopeType, &record.ScopeSubject, &record.PolicyRevision, &record.Priority,
		&record.BasisSummary, &record.TermsURL, &record.LicenseURI, &record.PolicyHash, &record.ParentPolicyID,
		&record.ApprovedByUserID, &record.EffectiveAt, &record.ExpiresAt,
	)
	return record, err
}

func scanRightsManagementDecisionBatch(scanner rightsManagementScanner) (rightsManagementDecisionBatchRecord, error) {
	var record rightsManagementDecisionBatchRecord
	err := scanner.Scan(
		&record.ID, &record.Version, &record.SourceConnectionID, &record.PolicyID, &record.ExpectedPolicyVersion,
		&record.SubjectType, &record.SubjectKey, &record.InputDigest, &record.RecordedByUserID, &record.IdempotencyKey,
		&record.CommandFingerprint, &record.DecisionCount, &record.CreatedAt,
	)
	return record, err
}

func scanRightsManagementDecision(scanner rightsManagementScanner) (rightsManagementDecisionRecord, error) {
	var record rightsManagementDecisionRecord
	err := scanner.Scan(
		&record.ID, &record.DecisionBatchID, &record.SourceConnectionID, &record.PolicyID, &record.PolicyRevision,
		&record.PolicyScopeType, &record.PolicyScopeSubject, &record.Priority, &record.BasisSummary,
		&record.TermsURL, &record.LicenseURI, &record.SubjectType, &record.SubjectKey, &record.InputDigest,
		&record.Action, &record.Decision, &record.ReasonCodesJSON, &record.Evaluator, &record.EvaluatedAt,
		&record.EffectiveFrom, &record.ExpiresAt, &record.RetentionDays, &record.SupersedesDecisionID,
	)
	return record, err
}

func (record rightsManagementPolicyRecord) applicationDTO() sourceapplication.RightsPolicyDTO {
	return sourceapplication.RightsPolicyDTO{
		ID: record.ID, Version: record.Version, SourceConnectionID: nullableRightsManagementInt64(record.SourceConnectionID),
		ScopeType: record.ScopeType, ScopeSubject: record.ScopeSubject, Revision: record.PolicyRevision,
		Priority: record.Priority, BasisSummary: record.BasisSummary, TermsURL: record.TermsURL.String,
		LicenseURI: record.LicenseURI.String, PolicyHash: record.PolicyHash, EffectiveFrom: record.EffectiveAt.UTC(),
		ExpiresAt: nullableRightsManagementTime(record.ExpiresAt), ParentPolicyID: nullableRightsManagementInt64(record.ParentPolicyID),
		ApprovedByUserID: nullableRightsManagementInt64(record.ApprovedByUserID),
	}
}

func (record rightsManagementDecisionRecord) applicationDTO() (sourceapplication.RightsDecisionDTO, error) {
	reasonCodes := make([]string, 0)
	if err := json.Unmarshal(record.ReasonCodesJSON, &reasonCodes); err != nil {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("decode rights decision reason codes: %w", err)
	}
	if record.RetentionDays.Valid && (record.RetentionDays.Int64 < 1 || record.RetentionDays.Int64 > 3650) {
		return sourceapplication.RightsDecisionDTO{}, fmt.Errorf("rights decision retention duration is invalid")
	}
	var retentionDays *int
	if record.RetentionDays.Valid {
		value := int(record.RetentionDays.Int64)
		retentionDays = &value
	}
	return sourceapplication.RightsDecisionDTO{
		ID: record.ID, DecisionBatchID: record.DecisionBatchID, SourceConnectionID: record.SourceConnectionID,
		PolicyID: record.PolicyID, PolicyRevision: record.PolicyRevision,
		PolicyScopeType: record.PolicyScopeType, PolicyScopeSubject: record.PolicyScopeSubject,
		Priority: record.Priority, BasisSummary: record.BasisSummary, TermsURL: record.TermsURL.String, LicenseURI: record.LicenseURI.String,
		SubjectType: record.SubjectType, SubjectKey: record.SubjectKey, InputDigest: record.InputDigest,
		Action: record.Action, Decision: record.Decision, ReasonCodes: reasonCodes, Evaluator: record.Evaluator,
		EvaluatedAt: record.EvaluatedAt.UTC(), EffectiveFrom: record.EffectiveFrom.UTC(),
		ExpiresAt: nullableRightsManagementTime(record.ExpiresAt), RetentionDays: retentionDays,
		SupersedesDecisionID: nullableRightsManagementInt64(record.SupersedesDecisionID),
	}, nil
}

func nullableRightsManagementInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullableRightsManagementTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}
