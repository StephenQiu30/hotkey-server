package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// RightsState is deliberately fail-closed: only an explicit allow permits an
// action. Unknown is a first-class decision, not an unset value.
type RightsState string

const (
	RightsAllow   RightsState = "allow"
	RightsDeny    RightsState = "deny"
	RightsUnknown RightsState = "unknown"
)

func (state RightsState) Valid() bool {
	return state == RightsAllow || state == RightsDeny || state == RightsUnknown
}

type RightsAction string

const (
	RightsActionFetch             RightsAction = "fetch"
	RightsActionStoreRaw          RightsAction = "store_raw"
	RightsActionStoreDerived      RightsAction = "store_derived"
	RightsActionDisplayPrivate    RightsAction = "display_private"
	RightsActionRedistribute      RightsAction = "redistribute"
	RightsActionQuote             RightsAction = "quote"
	RightsActionEmbedLocal        RightsAction = "embed_local"
	RightsActionSendExternalModel RightsAction = "send_external_model"
	RightsActionRetain            RightsAction = "retain"
)

func (action RightsAction) Valid() bool {
	switch action {
	case RightsActionFetch, RightsActionStoreRaw, RightsActionStoreDerived, RightsActionDisplayPrivate,
		RightsActionRedistribute, RightsActionQuote, RightsActionEmbedLocal, RightsActionSendExternalModel, RightsActionRetain:
		return true
	default:
		return false
	}
}

type RightsScopeType string

const (
	RightsScopeOrganizationDefault RightsScopeType = "organization_default"
	RightsScopeSourceEndpoint      RightsScopeType = "source_endpoint"
	RightsScopePublisher           RightsScopeType = "publisher"
	RightsScopeFeedOrAccount       RightsScopeType = "feed_or_account"
	RightsScopeObservation         RightsScopeType = "observation"
)

type RightsScope struct {
	Type      RightsScopeType
	SubjectID string
}

func (scope RightsScope) Validate() error {
	switch scope.Type {
	case RightsScopeOrganizationDefault:
		if scope.SubjectID != "" {
			return fmt.Errorf("organization rights scope cannot identify a subject")
		}
		return nil
	case RightsScopeSourceEndpoint, RightsScopePublisher, RightsScopeFeedOrAccount, RightsScopeObservation:
		if !validRightsText(scope.SubjectID, 512) {
			return fmt.Errorf("rights scope subject is invalid")
		}
		return nil
	default:
		return fmt.Errorf("rights scope type is invalid")
	}
}

// RightsPriority encodes the fixed precedence ladder. Values between these
// levels are rejected so callers cannot invent a permission-expanding tier.
type RightsPriority int

const (
	RightsPriorityOrganizationDefault  RightsPriority = 100
	RightsPriorityConnectorRestriction RightsPriority = 200
	RightsPriorityEndpointContract     RightsPriority = 300
	RightsPriorityObservationExplicit  RightsPriority = 400
)

func (priority RightsPriority) Valid() bool {
	return priority == RightsPriorityOrganizationDefault || priority == RightsPriorityConnectorRestriction || priority == RightsPriorityEndpointContract || priority == RightsPriorityObservationExplicit
}

func (priority RightsPriority) validFor(scope RightsScopeType) bool {
	switch scope {
	case RightsScopeOrganizationDefault:
		return priority == RightsPriorityOrganizationDefault
	case RightsScopeSourceEndpoint, RightsScopePublisher, RightsScopeFeedOrAccount:
		return priority == RightsPriorityConnectorRestriction || priority == RightsPriorityEndpointContract
	case RightsScopeObservation:
		return priority == RightsPriorityObservationExplicit
	default:
		return false
	}
}

type RightsBasis struct {
	Summary    string
	TermsURL   string
	LicenseURI string
}

func (basis RightsBasis) Validate() error {
	if !validRightsText(basis.Summary, 1024) {
		return fmt.Errorf("rights basis is invalid")
	}
	if err := validateRightsURI(basis.TermsURL, false); err != nil {
		return fmt.Errorf("rights terms URI is invalid")
	}
	if err := validateRightsURI(basis.LicenseURI, true); err != nil {
		return fmt.Errorf("rights license URI is invalid")
	}
	return nil
}

// RightsPolicy is the immutable domain entity that supplies policy metadata.
// Persistence records and Application DTOs map explicitly into this entity.
type RightsPolicy struct {
	ID                 int64
	Version            int64
	SourceConnectionID *int64
	Scope              RightsScope
	Revision           int64
	Priority           RightsPriority
	Basis              RightsBasis
	PolicyHash         string
	EffectiveFrom      time.Time
	ExpiresAt          *time.Time
	ParentPolicyID     *int64
	ApprovedByUserID   *int64
}

func (policy RightsPolicy) Validate() error {
	if policy.ID <= 0 || policy.Version <= 0 || policy.Revision <= 0 || !validSHA256(policy.PolicyHash) {
		return fmt.Errorf("rights policy identity is invalid")
	}
	if err := policy.Scope.Validate(); err != nil {
		return err
	}
	if !policy.Priority.Valid() || !policy.Priority.validFor(policy.Scope.Type) {
		return fmt.Errorf("rights policy priority is invalid for scope")
	}
	if policy.Scope.Type == RightsScopeOrganizationDefault {
		if policy.SourceConnectionID != nil {
			return fmt.Errorf("organization rights policy cannot identify a source")
		}
	} else if policy.SourceConnectionID == nil || *policy.SourceConnectionID <= 0 {
		return fmt.Errorf("scoped rights policy source is invalid")
	}
	if err := policy.Basis.Validate(); err != nil {
		return err
	}
	if policy.EffectiveFrom.IsZero() || policy.ExpiresAt != nil && !policy.ExpiresAt.After(policy.EffectiveFrom) {
		return fmt.Errorf("rights policy lifetime is invalid")
	}
	if policy.ParentPolicyID != nil && (*policy.ParentPolicyID <= 0 || *policy.ParentPolicyID == policy.ID) {
		return fmt.Errorf("rights parent policy is invalid")
	}
	if policy.ApprovedByUserID != nil && *policy.ApprovedByUserID <= 0 {
		return fmt.Errorf("rights policy approver is invalid")
	}
	return nil
}

type RightsSubjectType string

const (
	RightsSubjectSourceEndpoint    RightsSubjectType = "source_endpoint"
	RightsSubjectRawResponse       RightsSubjectType = "raw_response"
	RightsSubjectSourceObservation RightsSubjectType = "source_observation"
	RightsSubjectDocumentVersion   RightsSubjectType = "document_version"
)

func (subjectType RightsSubjectType) Valid() bool {
	return subjectType == RightsSubjectSourceEndpoint || subjectType == RightsSubjectRawResponse ||
		subjectType == RightsSubjectSourceObservation || subjectType == RightsSubjectDocumentVersion
}

// RightsDecision is one immutable, exact action evaluation. Policy metadata is
// copied as an immutable snapshot so historical authorization remains
// explainable without turning a policy into a mutable action matrix.
type RightsDecision struct {
	ID                   int64
	SourceConnectionID   int64
	Scope                RightsScope
	PolicyID             int64
	PolicyRevision       int64
	Priority             RightsPriority
	Basis                RightsBasis
	SubjectType          RightsSubjectType
	SubjectKey           string
	InputDigest          string
	Action               RightsAction
	Decision             RightsState
	ReasonCodes          []string
	Evaluator            string
	EvaluatedAt          time.Time
	EffectiveFrom        time.Time
	ExpiresAt            *time.Time
	RetentionDays        *int
	SupersedesDecisionID *int64
}

func (decision RightsDecision) Validate() error {
	if decision.ID <= 0 || decision.SourceConnectionID <= 0 {
		return fmt.Errorf("rights decision identity is invalid")
	}
	if err := decision.Scope.Validate(); err != nil {
		return err
	}
	if decision.PolicyID <= 0 || decision.PolicyRevision <= 0 {
		return fmt.Errorf("rights policy identity is invalid")
	}
	if !decision.Priority.Valid() || !decision.Priority.validFor(decision.Scope.Type) {
		return fmt.Errorf("rights policy priority is invalid for scope")
	}
	if err := decision.Basis.Validate(); err != nil {
		return err
	}
	if !decision.SubjectType.Valid() || !validRightsText(decision.SubjectKey, 512) || !validSHA256(decision.InputDigest) {
		return fmt.Errorf("rights decision subject is invalid")
	}
	if !decision.Action.Valid() || !decision.Decision.Valid() {
		return fmt.Errorf("rights decision action or result is invalid")
	}
	if !validRightsText(decision.Evaluator, 64) || !validRightsReasonCodes(decision.ReasonCodes) {
		return fmt.Errorf("rights decision evaluation metadata is invalid")
	}
	if decision.EvaluatedAt.IsZero() || decision.EffectiveFrom.IsZero() {
		return fmt.Errorf("rights decision evaluation and effective time are required")
	}
	if decision.ExpiresAt != nil && !decision.ExpiresAt.After(decision.EffectiveFrom) {
		return fmt.Errorf("rights decision expiry is invalid")
	}
	if decision.Action == RightsActionRetain {
		if decision.RetentionDays == nil || *decision.RetentionDays <= 0 || *decision.RetentionDays > 3650 {
			return fmt.Errorf("retain decision duration is invalid")
		}
	} else if decision.RetentionDays != nil {
		return fmt.Errorf("retention duration belongs only to retain decisions")
	}
	if decision.SupersedesDecisionID != nil && (*decision.SupersedesDecisionID <= 0 || *decision.SupersedesDecisionID == decision.ID) {
		return fmt.Errorf("rights decision supersession is invalid")
	}
	return nil
}

func validRightsReasonCodes(values []string) bool {
	if len(values) > 32 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validRightsText(value, 64) || value != strings.ToLower(value) {
			return false
		}
		for _, character := range value {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '_' || character == '-' || character == ':') {
				return false
			}
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func (decision RightsDecision) Allows(at time.Time) bool {
	if at.IsZero() || decision.Validate() != nil || at.Before(decision.EffectiveFrom) {
		return false
	}
	if decision.ExpiresAt != nil && !at.Before(*decision.ExpiresAt) {
		return false
	}
	return decision.Decision == RightsAllow
}

func validRightsText(value string, maxBytes int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maxBytes && !strings.ContainsAny(value, "\x00\r\n")
}

func validateRightsURI(value string, allowURN bool) error {
	if value == "" {
		return nil
	}
	if value != strings.TrimSpace(value) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("invalid URI")
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || parsed.Fragment != "" || parsed.User != nil {
		return fmt.Errorf("invalid URI")
	}
	if allowURN && parsed.Scheme == "urn" && parsed.Opaque != "" {
		return nil
	}
	if parsed.Scheme != "https" || parsed.Hostname() == "" {
		return fmt.Errorf("invalid URI")
	}
	return nil
}
