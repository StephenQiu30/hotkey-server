package http

import (
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
)

type SourceEndpointCapabilityResponseDTO struct {
	SourceEndpointID    int64    `json:"source_endpoint_id"`
	SourceType          string   `json:"source_type"`
	CollectionInterface string   `json:"collection_interface"`
	ContentScope        string   `json:"content_scope"`
	DocumentCaptureMode string   `json:"document_capture_mode"`
	DefaultAccessMode   string   `json:"default_access_mode"`
	RequiredActions     []string `json:"required_actions"`
	FollowsCanonicalURL bool     `json:"follows_canonical_url"`
	Availability        string   `json:"availability"`
	RightsStatus        string   `json:"rights_status"`
}

type CreateRightsPolicyRequestDTO struct {
	ScopeType        string     `json:"scope_type"`
	ScopeSubject     string     `json:"scope_subject"`
	Revision         int64      `json:"revision"`
	Priority         int        `json:"priority"`
	BasisSummary     string     `json:"basis_summary"`
	TermsURL         string     `json:"terms_url,omitempty"`
	LicenseURI       string     `json:"license_uri,omitempty"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	ParentPolicyID   *int64     `json:"parent_policy_id,omitempty"`
	ApprovedByUserID *int64     `json:"approved_by_user_id,omitempty"`
}

type RightsPolicyResponseDTO struct {
	ID               int64      `json:"id"`
	Version          int64      `json:"version"`
	SourceEndpointID *int64     `json:"source_endpoint_id" extensions:"x-nullable"`
	ScopeType        string     `json:"scope_type"`
	ScopeSubject     string     `json:"scope_subject"`
	Revision         int64      `json:"revision"`
	Priority         int        `json:"priority"`
	BasisSummary     string     `json:"basis_summary"`
	TermsURL         string     `json:"terms_url,omitempty"`
	LicenseURI       string     `json:"license_uri,omitempty"`
	PolicyHash       string     `json:"policy_hash"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	ExpiresAt        *time.Time `json:"expires_at" extensions:"x-nullable"`
	ParentPolicyID   *int64     `json:"parent_policy_id" extensions:"x-nullable"`
	RecordedByUserID *int64     `json:"recorded_by_user_id,omitempty" extensions:"x-nullable"`
	ApprovedByUserID *int64     `json:"approved_by_user_id" extensions:"x-nullable"`
	CreatedAt        *time.Time `json:"created_at,omitempty" extensions:"x-nullable"`
}

type CreateRightsPolicyResponseDTO struct {
	Policy           RightsPolicyResponseDTO `json:"policy"`
	IdempotentReplay bool                    `json:"idempotent_replay"`
}

type RightsPolicyPageResponseDTO struct {
	Items      []RightsPolicyResponseDTO `json:"items"`
	NextCursor string                    `json:"next_cursor,omitempty"`
}

type RightsActionDecisionRequestDTO struct {
	Action               string     `json:"action"`
	Decision             string     `json:"decision"`
	ReasonCodes          []string   `json:"reason_codes"`
	Evaluator            string     `json:"evaluator"`
	EvaluatedAt          time.Time  `json:"evaluated_at"`
	EffectiveFrom        time.Time  `json:"effective_from"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	RetentionDays        *int       `json:"retention_days,omitempty"`
	SupersedesDecisionID *int64     `json:"supersedes_decision_id,omitempty"`
}

type RecordRightsDecisionBatchRequestDTO struct {
	PolicyID              int64                            `json:"policy_id"`
	ExpectedPolicyVersion int64                            `json:"expected_policy_version"`
	SubjectType           string                           `json:"subject_type"`
	SubjectKey            string                           `json:"subject_key"`
	InputDigest           string                           `json:"input_digest"`
	Decisions             []RightsActionDecisionRequestDTO `json:"decisions"`
}

type RightsDecisionResponseDTO struct {
	ID                   int64      `json:"id"`
	DecisionBatchID      int64      `json:"decision_batch_id"`
	SourceEndpointID     int64      `json:"source_endpoint_id"`
	PolicyID             int64      `json:"policy_id"`
	PolicyRevision       int64      `json:"policy_revision"`
	PolicyScopeType      string     `json:"policy_scope_type"`
	PolicyScopeSubject   string     `json:"policy_scope_subject"`
	Priority             int        `json:"priority"`
	BasisSummary         string     `json:"basis_summary"`
	TermsURL             string     `json:"terms_url,omitempty"`
	LicenseURI           string     `json:"license_uri,omitempty"`
	SubjectType          string     `json:"subject_type"`
	SubjectKey           string     `json:"subject_key"`
	InputDigest          string     `json:"input_digest"`
	Action               string     `json:"action"`
	Decision             string     `json:"decision"`
	ReasonCodes          []string   `json:"reason_codes"`
	Evaluator            string     `json:"evaluator"`
	EvaluatedAt          time.Time  `json:"evaluated_at"`
	EffectiveFrom        time.Time  `json:"effective_from"`
	ExpiresAt            *time.Time `json:"expires_at" extensions:"x-nullable"`
	RetentionDays        *int       `json:"retention_days" extensions:"x-nullable"`
	SupersedesDecisionID *int64     `json:"supersedes_decision_id" extensions:"x-nullable"`
	RecordedByUserID     *int64     `json:"recorded_by_user_id,omitempty" extensions:"x-nullable"`
	CreatedAt            *time.Time `json:"created_at,omitempty" extensions:"x-nullable"`
}

type RecordRightsDecisionBatchResponseDTO struct {
	DecisionBatchID  int64                       `json:"decision_batch_id"`
	Decisions        []RightsDecisionResponseDTO `json:"decisions"`
	IdempotentReplay bool                        `json:"idempotent_replay"`
}

type RightsDecisionBatchResponseDTO struct {
	ID                    int64                       `json:"id"`
	Version               int64                       `json:"version"`
	SourceEndpointID      int64                       `json:"source_endpoint_id"`
	PolicyID              int64                       `json:"policy_id"`
	ExpectedPolicyVersion int64                       `json:"expected_policy_version"`
	SubjectType           string                      `json:"subject_type"`
	SubjectKey            string                      `json:"subject_key"`
	InputDigest           string                      `json:"input_digest"`
	RecordedByUserID      int64                       `json:"recorded_by_user_id"`
	DecisionCount         int                         `json:"decision_count"`
	CreatedAt             time.Time                   `json:"created_at"`
	Decisions             []RightsDecisionResponseDTO `json:"decisions"`
}

type RightsDecisionBatchPageResponseDTO struct {
	Items      []RightsDecisionBatchResponseDTO `json:"items"`
	NextCursor string                           `json:"next_cursor,omitempty"`
}

type EvaluateRightsActionsRequestDTO struct {
	SubjectType string    `json:"subject_type"`
	SubjectKey  string    `json:"subject_key"`
	InputDigest string    `json:"input_digest"`
	At          time.Time `json:"at"`
}

type RightsActionCapabilityResponseDTO struct {
	Action        string  `json:"action"`
	Decision      string  `json:"decision"`
	DecisionIDs   []int64 `json:"decision_ids"`
	PolicyIDs     []int64 `json:"policy_ids"`
	Priority      *int    `json:"priority" extensions:"x-nullable"`
	RetentionDays *int    `json:"retention_days" extensions:"x-nullable"`
}

// RightsActionMatrixResponseDTO intentionally does not echo the exact subject or
// digest accepted by EvaluateRightsActionsRequestDTO.
type RightsActionMatrixResponseDTO struct {
	SourceEndpointID int64                               `json:"source_endpoint_id"`
	EvaluatedAt      time.Time                           `json:"evaluated_at"`
	Actions          []RightsActionCapabilityResponseDTO `json:"actions"`
}

func sourceEndpointCapabilityResponse(value sourceapplication.SourceEndpointCapabilityDTO) SourceEndpointCapabilityResponseDTO {
	return SourceEndpointCapabilityResponseDTO{
		SourceEndpointID: value.SourceEndpointID, SourceType: value.SourceType,
		CollectionInterface: value.CollectionInterface, ContentScope: value.ContentScope,
		DocumentCaptureMode: value.DocumentCaptureMode, DefaultAccessMode: value.DefaultAccessMode,
		RequiredActions:     append([]string(nil), value.RequiredActions...),
		FollowsCanonicalURL: value.FollowsCanonicalURL, Availability: value.Availability, RightsStatus: value.RightsStatus,
	}
}

func rightsPolicyResponse(value sourceapplication.RightsPolicyDTO) RightsPolicyResponseDTO {
	return RightsPolicyResponseDTO{
		ID: value.ID, Version: value.Version, SourceEndpointID: copyRightsTransportInt64(value.SourceConnectionID),
		ScopeType: value.ScopeType, ScopeSubject: value.ScopeSubject, Revision: value.Revision, Priority: value.Priority,
		BasisSummary: value.BasisSummary, TermsURL: value.TermsURL, LicenseURI: value.LicenseURI, PolicyHash: value.PolicyHash,
		EffectiveFrom: value.EffectiveFrom, ExpiresAt: copyRightsTransportTime(value.ExpiresAt),
		ParentPolicyID: copyRightsTransportInt64(value.ParentPolicyID), ApprovedByUserID: copyRightsTransportInt64(value.ApprovedByUserID),
	}
}

func rightsPolicyReadResponse(value sourceapplication.RightsPolicyReadDTO) RightsPolicyResponseDTO {
	recordedBy := value.RecordedByUserID
	createdAt := value.CreatedAt
	return RightsPolicyResponseDTO{
		ID: value.ID, Version: value.Version, SourceEndpointID: copyRightsTransportInt64(value.SourceEndpointID),
		ScopeType: value.ScopeType, ScopeSubject: value.ScopeSubject, Revision: value.Revision, Priority: value.Priority,
		BasisSummary: value.BasisSummary, TermsURL: value.TermsURL, LicenseURI: value.LicenseURI, PolicyHash: value.PolicyHash,
		EffectiveFrom: value.EffectiveFrom, ExpiresAt: copyRightsTransportTime(value.ExpiresAt),
		ParentPolicyID: copyRightsTransportInt64(value.ParentPolicyID), RecordedByUserID: &recordedBy,
		ApprovedByUserID: copyRightsTransportInt64(value.ApprovedByUserID), CreatedAt: &createdAt,
	}
}

func rightsDecisionResponse(value sourceapplication.RightsDecisionDTO) RightsDecisionResponseDTO {
	return RightsDecisionResponseDTO{
		ID: value.ID, DecisionBatchID: value.DecisionBatchID, SourceEndpointID: value.SourceConnectionID,
		PolicyID: value.PolicyID, PolicyRevision: value.PolicyRevision, PolicyScopeType: value.PolicyScopeType,
		PolicyScopeSubject: value.PolicyScopeSubject, Priority: value.Priority, BasisSummary: value.BasisSummary,
		TermsURL: value.TermsURL, LicenseURI: value.LicenseURI, SubjectType: value.SubjectType, SubjectKey: value.SubjectKey,
		InputDigest: value.InputDigest, Action: value.Action, Decision: value.Decision,
		ReasonCodes: append([]string(nil), value.ReasonCodes...), Evaluator: value.Evaluator,
		EvaluatedAt: value.EvaluatedAt, EffectiveFrom: value.EffectiveFrom,
		ExpiresAt: copyRightsTransportTime(value.ExpiresAt), RetentionDays: copyRightsTransportInt(value.RetentionDays),
		SupersedesDecisionID: copyRightsTransportInt64(value.SupersedesDecisionID),
	}
}

func rightsDecisionReadResponse(value sourceapplication.RightsDecisionReadDTO) RightsDecisionResponseDTO {
	recordedBy := value.RecordedByUserID
	createdAt := value.CreatedAt
	return RightsDecisionResponseDTO{
		ID: value.ID, DecisionBatchID: value.DecisionBatchID, SourceEndpointID: value.SourceEndpointID,
		PolicyID: value.PolicyID, PolicyRevision: value.PolicyRevision, PolicyScopeType: value.PolicyScopeType,
		PolicyScopeSubject: value.PolicyScopeSubject, Priority: value.Priority, BasisSummary: value.BasisSummary,
		TermsURL: value.TermsURL, LicenseURI: value.LicenseURI, SubjectType: value.SubjectType, SubjectKey: value.SubjectKey,
		InputDigest: value.InputDigest, Action: value.Action, Decision: value.Decision,
		ReasonCodes: append([]string(nil), value.ReasonCodes...), Evaluator: value.Evaluator,
		EvaluatedAt: value.EvaluatedAt, EffectiveFrom: value.EffectiveFrom,
		ExpiresAt: copyRightsTransportTime(value.ExpiresAt), RetentionDays: copyRightsTransportInt(value.RetentionDays),
		SupersedesDecisionID: copyRightsTransportInt64(value.SupersedesDecisionID),
		RecordedByUserID:     &recordedBy, CreatedAt: &createdAt,
	}
}

func rightsDecisionBatchResponse(value sourceapplication.RightsDecisionBatchDTO) RightsDecisionBatchResponseDTO {
	decisions := make([]RightsDecisionResponseDTO, 0, len(value.Decisions))
	for _, decision := range value.Decisions {
		decisions = append(decisions, rightsDecisionReadResponse(decision))
	}
	return RightsDecisionBatchResponseDTO{
		ID: value.ID, Version: value.Version, SourceEndpointID: value.SourceEndpointID,
		PolicyID: value.PolicyID, ExpectedPolicyVersion: value.ExpectedPolicyVersion,
		SubjectType: value.SubjectType, SubjectKey: value.SubjectKey, InputDigest: value.InputDigest,
		RecordedByUserID: value.RecordedByUserID, DecisionCount: value.DecisionCount,
		CreatedAt: value.CreatedAt, Decisions: decisions,
	}
}

func rightsActionMatrixResponse(value sourceapplication.RightsActionMatrixDTO) RightsActionMatrixResponseDTO {
	actions := make([]RightsActionCapabilityResponseDTO, 0, len(value.Actions))
	for _, action := range value.Actions {
		actions = append(actions, RightsActionCapabilityResponseDTO{
			Action: action.Action, Decision: action.Decision,
			DecisionIDs: append([]int64(nil), action.DecisionIDs...), PolicyIDs: append([]int64(nil), action.PolicyIDs...),
			Priority: copyRightsTransportInt(action.Priority), RetentionDays: copyRightsTransportInt(action.RetentionDays),
		})
	}
	return RightsActionMatrixResponseDTO{SourceEndpointID: value.SourceEndpointID, EvaluatedAt: value.EvaluatedAt, Actions: actions}
}

func copyRightsTransportTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func copyRightsTransportInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyRightsTransportInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
