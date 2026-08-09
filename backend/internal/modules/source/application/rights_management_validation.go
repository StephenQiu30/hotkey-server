package application

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func prepareCreateRightsPolicy(command CreateRightsPolicyCommand) (CreateRightsPolicyRepositoryDTO, error) {
	if command.ActorID <= 0 || !validRightsManagementIdempotencyKey(command.IdempotencyKey) {
		return CreateRightsPolicyRepositoryDTO{}, rightsManagementInvalidInput("policy actor or idempotency key is invalid")
	}
	request := CreateRightsPolicyRepositoryDTO{
		ActorID: command.ActorID, IdempotencyKey: command.IdempotencyKey,
		SourceConnectionID: rightsManagementInt64Pointer(command.SourceConnectionID),
		ScopeType:          command.ScopeType, ScopeSubject: command.ScopeSubject,
		Revision: command.Revision, Priority: command.Priority,
		BasisSummary: command.BasisSummary, TermsURL: command.TermsURL, LicenseURI: command.LicenseURI,
		EffectiveFrom: rightsManagementPersistenceTime(command.EffectiveFrom), ExpiresAt: rightsManagementPersistenceTimePointer(command.ExpiresAt),
		ParentPolicyID:   rightsManagementInt64Pointer(command.ParentPolicyID),
		ApprovedByUserID: rightsManagementInt64Pointer(command.ApprovedByUserID),
	}
	request.PolicyHash = rightsPolicyHash(request)
	if err := validateCreateRightsPolicyRequest(request); err != nil {
		return CreateRightsPolicyRepositoryDTO{}, err
	}
	request.CommandFingerprint = rightsCreatePolicyCommandFingerprint(request)
	return request, nil
}

func validateCreateRightsPolicyRequest(request CreateRightsPolicyRepositoryDTO) error {
	scope := domain.RightsScope{Type: domain.RightsScopeType(request.ScopeType), SubjectID: request.ScopeSubject}
	if err := scope.Validate(); err != nil {
		return rightsManagementInvalidInput("policy scope is invalid")
	}
	priority := domain.RightsPriority(request.Priority)
	if !priority.Valid() || !rightsPriorityMatchesScope(priority, scope.Type) {
		return rightsManagementInvalidInput("policy priority is invalid for scope")
	}
	if scope.Type == domain.RightsScopeOrganizationDefault {
		if request.SourceConnectionID != nil {
			return rightsManagementInvalidInput("organization policy source is invalid")
		}
	} else if request.SourceConnectionID == nil || *request.SourceConnectionID <= 0 {
		return rightsManagementInvalidInput("scoped policy source is invalid")
	}
	if request.Revision <= 0 || !validRightsManagementSHA256(request.PolicyHash) {
		return rightsManagementInvalidInput("policy identity is invalid")
	}
	basis := domain.RightsBasis{Summary: request.BasisSummary, TermsURL: request.TermsURL, LicenseURI: request.LicenseURI}
	if err := basis.Validate(); err != nil {
		return rightsManagementInvalidInput("policy basis is invalid")
	}
	if request.EffectiveFrom.IsZero() || request.ExpiresAt != nil && !request.ExpiresAt.After(request.EffectiveFrom) {
		return rightsManagementInvalidInput("policy lifetime is invalid")
	}
	if request.ParentPolicyID != nil && *request.ParentPolicyID <= 0 {
		return rightsManagementInvalidInput("parent policy identity is invalid")
	}
	if request.ApprovedByUserID != nil && *request.ApprovedByUserID <= 0 {
		return rightsManagementInvalidInput("policy approver identity is invalid")
	}
	return nil
}

func prepareRecordRightsDecision(command RecordRightsDecisionCommand) (RecordRightsDecisionRepositoryDTO, error) {
	if command.ActorID <= 0 || !validRightsManagementIdempotencyKey(command.IdempotencyKey) || command.SourceConnectionID <= 0 ||
		command.PolicyID <= 0 || command.ExpectedPolicyVersion <= 0 {
		return RecordRightsDecisionRepositoryDTO{}, rightsManagementInvalidInput("decision actor, idempotency, source, or policy identity is invalid")
	}
	subjectType := domain.RightsSubjectType(command.SubjectType)
	if !subjectType.Valid() || !validRightsManagementText(command.SubjectKey, 512) || !validRightsManagementSHA256(command.InputDigest) {
		return RecordRightsDecisionRepositoryDTO{}, rightsManagementInvalidInput("decision subject is invalid")
	}
	if len(command.Decisions) == 0 || len(command.Decisions) > 9 {
		return RecordRightsDecisionRepositoryDTO{}, rightsManagementInvalidInput("decision batch size is invalid")
	}

	decisions := make([]RightsActionDecisionDTO, len(command.Decisions))
	seenActions := make(map[domain.RightsAction]struct{}, len(command.Decisions))
	for index, candidate := range command.Decisions {
		canonical, err := canonicalRightsActionDecision(candidate)
		if err != nil {
			return RecordRightsDecisionRepositoryDTO{}, err
		}
		action := domain.RightsAction(canonical.Action)
		if _, duplicate := seenActions[action]; duplicate {
			return RecordRightsDecisionRepositoryDTO{}, rightsManagementInvalidInput("decision batch contains duplicate actions")
		}
		seenActions[action] = struct{}{}
		decisions[index] = canonical
	}
	sort.Slice(decisions, func(left, right int) bool { return decisions[left].Action < decisions[right].Action })
	request := RecordRightsDecisionRepositoryDTO{
		ActorID: command.ActorID, IdempotencyKey: command.IdempotencyKey,
		SourceConnectionID: command.SourceConnectionID, ExpectedPolicyVersion: command.ExpectedPolicyVersion,
		SubjectType: command.SubjectType, SubjectKey: command.SubjectKey, InputDigest: command.InputDigest,
		Decisions: decisions,
	}
	request.Policy = RightsPolicyDTO{ID: command.PolicyID, Version: command.ExpectedPolicyVersion}
	return request, nil
}

func canonicalRightsActionDecision(candidate RightsActionDecisionDTO) (RightsActionDecisionDTO, error) {
	action := domain.RightsAction(candidate.Action)
	state := domain.RightsState(candidate.Decision)
	if !action.Valid() || !state.Valid() {
		return RightsActionDecisionDTO{}, rightsManagementInvalidInput("decision action or state is invalid")
	}
	if !validRightsManagementReasonCodes(candidate.ReasonCodes) || !validRightsManagementText(candidate.Evaluator, 64) {
		return RightsActionDecisionDTO{}, rightsManagementInvalidInput("decision evaluation metadata is invalid")
	}
	if candidate.EvaluatedAt.IsZero() || candidate.EffectiveFrom.IsZero() ||
		candidate.ExpiresAt != nil && !candidate.ExpiresAt.After(candidate.EffectiveFrom) {
		return RightsActionDecisionDTO{}, rightsManagementInvalidInput("decision lifetime is invalid")
	}
	if action == domain.RightsActionRetain {
		if candidate.RetentionDays == nil || *candidate.RetentionDays < 1 || *candidate.RetentionDays > 3650 {
			return RightsActionDecisionDTO{}, rightsManagementInvalidInput("retain decision duration is invalid")
		}
	} else if candidate.RetentionDays != nil {
		return RightsActionDecisionDTO{}, rightsManagementInvalidInput("retention duration belongs only to retain")
	}
	if candidate.SupersedesDecisionID != nil && *candidate.SupersedesDecisionID <= 0 {
		return RightsActionDecisionDTO{}, rightsManagementInvalidInput("superseded decision identity is invalid")
	}
	reasonCodes := append([]string(nil), candidate.ReasonCodes...)
	sort.Strings(reasonCodes)
	return RightsActionDecisionDTO{
		Action: candidate.Action, Decision: candidate.Decision, ReasonCodes: reasonCodes,
		Evaluator: candidate.Evaluator, EvaluatedAt: rightsManagementPersistenceTime(candidate.EvaluatedAt), EffectiveFrom: rightsManagementPersistenceTime(candidate.EffectiveFrom),
		ExpiresAt: rightsManagementPersistenceTimePointer(candidate.ExpiresAt), RetentionDays: rightsManagementIntPointer(candidate.RetentionDays),
		SupersedesDecisionID: rightsManagementInt64Pointer(candidate.SupersedesDecisionID),
	}, nil
}

func validateDecisionPolicyCompatibility(request RecordRightsDecisionRepositoryDTO) error {
	policy := request.Policy
	if policy.ID <= 0 || policy.Version <= 0 || policy.Revision <= 0 || policy.Version != request.ExpectedPolicyVersion {
		return fmt.Errorf("%w: decision policy identity is invalid", sharedrepository.ErrConstraint)
	}
	if policy.SourceConnectionID != nil && *policy.SourceConnectionID != request.SourceConnectionID {
		return fmt.Errorf("%w: decision source does not match policy source", sharedrepository.ErrConstraint)
	}
	for _, candidate := range request.Decisions {
		if candidate.Decision == string(domain.RightsAllow) && policy.ApprovedByUserID == nil {
			return fmt.Errorf("%w: allow decision requires an approved policy", sharedrepository.ErrConstraint)
		}
		if candidate.EffectiveFrom.Before(policy.EffectiveFrom) {
			return fmt.Errorf("%w: decision lifetime starts before policy", sharedrepository.ErrConstraint)
		}
		if policy.ExpiresAt != nil && (candidate.ExpiresAt == nil || candidate.ExpiresAt.After(*policy.ExpiresAt)) {
			return fmt.Errorf("%w: decision lifetime exceeds policy", sharedrepository.ErrConstraint)
		}
	}
	return nil
}

func rightsSupersessionMatches(request RecordRightsDecisionRepositoryDTO, candidate RightsActionDecisionDTO, previous RightsDecisionDTO) bool {
	return candidate.SupersedesDecisionID != nil && previous.ID == *candidate.SupersedesDecisionID &&
		previous.SourceConnectionID == request.SourceConnectionID && previous.SubjectType == request.SubjectType &&
		previous.SubjectKey == request.SubjectKey && previous.InputDigest == request.InputDigest &&
		previous.Action == candidate.Action && !candidate.EffectiveFrom.Before(previous.EffectiveFrom)
}

func validateCreatedRightsPolicy(request CreateRightsPolicyRepositoryDTO, policy RightsPolicyDTO) (RightsPolicyDTO, error) {
	if err := validateRightsPolicyDTO(policy); err != nil {
		return RightsPolicyDTO{}, fmt.Errorf("persisted rights policy is invalid: %w", err)
	}
	if policy.Version != 1 || !rightsManagementOptionalInt64Equal(policy.SourceConnectionID, request.SourceConnectionID) ||
		policy.ScopeType != request.ScopeType || policy.ScopeSubject != request.ScopeSubject || policy.Revision != request.Revision ||
		policy.Priority != request.Priority || policy.BasisSummary != request.BasisSummary || policy.TermsURL != request.TermsURL ||
		policy.LicenseURI != request.LicenseURI || policy.PolicyHash != request.PolicyHash || !policy.EffectiveFrom.Equal(request.EffectiveFrom) ||
		!rightsManagementOptionalTimeEqual(policy.ExpiresAt, request.ExpiresAt) ||
		!rightsManagementOptionalInt64Equal(policy.ParentPolicyID, request.ParentPolicyID) ||
		!rightsManagementOptionalInt64Equal(policy.ApprovedByUserID, request.ApprovedByUserID) {
		return RightsPolicyDTO{}, fmt.Errorf("%w: persisted policy differs from immutable command", sharedrepository.ErrConstraint)
	}
	return cloneRightsManagementPolicy(policy), nil
}

func validateRecordedRightsDecisions(request RecordRightsDecisionRepositoryDTO, decisionBatchID int64, decisions []RightsDecisionDTO) ([]RightsDecisionDTO, error) {
	if decisionBatchID <= 0 {
		return nil, fmt.Errorf("%w: persisted decision batch identity is invalid", sharedrepository.ErrConstraint)
	}
	if len(decisions) != len(request.Decisions) {
		return nil, fmt.Errorf("%w: persisted decision batch size differs", sharedrepository.ErrConstraint)
	}
	expected := make(map[string]RightsActionDecisionDTO, len(request.Decisions))
	for _, candidate := range request.Decisions {
		expected[candidate.Action] = candidate
	}
	ids := make(map[int64]struct{}, len(decisions))
	result := make([]RightsDecisionDTO, len(decisions))
	for index, decision := range decisions {
		if decision.DecisionBatchID != decisionBatchID {
			return nil, fmt.Errorf("%w: persisted decision is bound to another batch", sharedrepository.ErrConstraint)
		}
		if err := validateRightsDecisionDTO(decision); err != nil {
			return nil, fmt.Errorf("persisted rights decision is invalid: %w", err)
		}
		candidate, found := expected[decision.Action]
		if !found || !rightsDecisionMatchesRequest(request, candidate, decision) {
			return nil, fmt.Errorf("%w: persisted decision differs from immutable command", sharedrepository.ErrConstraint)
		}
		if _, duplicate := ids[decision.ID]; duplicate {
			return nil, fmt.Errorf("%w: persisted decision batch contains duplicate identity", sharedrepository.ErrConstraint)
		}
		ids[decision.ID] = struct{}{}
		result[index] = cloneRightsManagementDecision(decision)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Action < result[right].Action })
	return result, nil
}

func rightsDecisionMatchesRequest(request RecordRightsDecisionRepositoryDTO, candidate RightsActionDecisionDTO, decision RightsDecisionDTO) bool {
	policy := request.Policy
	return decision.SourceConnectionID == request.SourceConnectionID && decision.PolicyID == policy.ID && decision.PolicyRevision == policy.Revision &&
		decision.PolicyScopeType == policy.ScopeType && decision.PolicyScopeSubject == policy.ScopeSubject && decision.Priority == policy.Priority &&
		decision.BasisSummary == policy.BasisSummary && decision.TermsURL == policy.TermsURL && decision.LicenseURI == policy.LicenseURI &&
		decision.SubjectType == request.SubjectType && decision.SubjectKey == request.SubjectKey && decision.InputDigest == request.InputDigest &&
		decision.Action == candidate.Action && decision.Decision == candidate.Decision &&
		rightsManagementStringsEqual(decision.ReasonCodes, candidate.ReasonCodes) && decision.Evaluator == candidate.Evaluator &&
		decision.EvaluatedAt.Equal(candidate.EvaluatedAt) && decision.EffectiveFrom.Equal(candidate.EffectiveFrom) &&
		rightsManagementOptionalTimeEqual(decision.ExpiresAt, candidate.ExpiresAt) &&
		rightsManagementOptionalIntEqual(decision.RetentionDays, candidate.RetentionDays) &&
		rightsManagementOptionalInt64Equal(decision.SupersedesDecisionID, candidate.SupersedesDecisionID)
}

func validateRightsPolicyDTO(policy RightsPolicyDTO) error {
	_, err := rightsPolicyEntity(policy)
	if err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return nil
}

func rightsPolicyEntity(policy RightsPolicyDTO) (domain.RightsPolicy, error) {
	entity := domain.RightsPolicy{
		ID: policy.ID, Version: policy.Version, SourceConnectionID: rightsManagementInt64Pointer(policy.SourceConnectionID),
		Scope:    domain.RightsScope{Type: domain.RightsScopeType(policy.ScopeType), SubjectID: policy.ScopeSubject},
		Revision: policy.Revision, Priority: domain.RightsPriority(policy.Priority),
		Basis:      domain.RightsBasis{Summary: policy.BasisSummary, TermsURL: policy.TermsURL, LicenseURI: policy.LicenseURI},
		PolicyHash: policy.PolicyHash, EffectiveFrom: policy.EffectiveFrom.UTC(),
		ExpiresAt: rightsManagementTimePointer(policy.ExpiresAt), ParentPolicyID: rightsManagementInt64Pointer(policy.ParentPolicyID),
		ApprovedByUserID: rightsManagementInt64Pointer(policy.ApprovedByUserID),
	}
	if err := entity.Validate(); err != nil {
		return domain.RightsPolicy{}, err
	}
	return entity, nil
}

func validateRightsDecisionDTO(decision RightsDecisionDTO) error {
	if decision.DecisionBatchID <= 0 {
		return fmt.Errorf("%w: rights decision batch identity is invalid", sharedrepository.ErrConstraint)
	}
	_, err := rightsDecisionEntity(decision)
	if err != nil {
		return fmt.Errorf("%w: %v", sharedrepository.ErrConstraint, err)
	}
	return nil
}

func rightsDecisionEntity(decision RightsDecisionDTO) (domain.RightsDecision, error) {
	entity := domain.RightsDecision{
		ID: decision.ID, SourceConnectionID: decision.SourceConnectionID,
		Scope:    domain.RightsScope{Type: domain.RightsScopeType(decision.PolicyScopeType), SubjectID: decision.PolicyScopeSubject},
		PolicyID: decision.PolicyID, PolicyRevision: decision.PolicyRevision, Priority: domain.RightsPriority(decision.Priority),
		Basis:       domain.RightsBasis{Summary: decision.BasisSummary, TermsURL: decision.TermsURL, LicenseURI: decision.LicenseURI},
		SubjectType: domain.RightsSubjectType(decision.SubjectType), SubjectKey: decision.SubjectKey, InputDigest: decision.InputDigest,
		Action: domain.RightsAction(decision.Action), Decision: domain.RightsState(decision.Decision),
		ReasonCodes: append([]string(nil), decision.ReasonCodes...), Evaluator: decision.Evaluator,
		EvaluatedAt: decision.EvaluatedAt.UTC(), EffectiveFrom: decision.EffectiveFrom.UTC(),
		ExpiresAt: rightsManagementTimePointer(decision.ExpiresAt), RetentionDays: rightsManagementIntPointer(decision.RetentionDays),
		SupersedesDecisionID: rightsManagementInt64Pointer(decision.SupersedesDecisionID),
	}
	if err := entity.Validate(); err != nil {
		return domain.RightsDecision{}, err
	}
	return entity, nil
}

func rightsDecisionAuditFacts(decisions []RightsDecisionDTO) ([]int64, []string) {
	ids := make([]int64, len(decisions))
	actions := make([]string, len(decisions))
	for index, decision := range decisions {
		ids[index], actions[index] = decision.ID, decision.Action
	}
	return ids, actions
}

func rightsPolicyHash(request CreateRightsPolicyRepositoryDTO) string {
	digest := sha256.New()
	rightsFingerprintString(digest, "rights-policy-v1")
	rightsFingerprintOptionalInt64(digest, request.SourceConnectionID)
	rightsFingerprintString(digest, request.ScopeType)
	rightsFingerprintString(digest, request.ScopeSubject)
	rightsFingerprintInt64(digest, request.Revision)
	rightsFingerprintInt64(digest, int64(request.Priority))
	rightsFingerprintString(digest, request.BasisSummary)
	rightsFingerprintString(digest, request.TermsURL)
	rightsFingerprintString(digest, request.LicenseURI)
	rightsFingerprintTime(digest, request.EffectiveFrom)
	rightsFingerprintOptionalTime(digest, request.ExpiresAt)
	rightsFingerprintOptionalInt64(digest, request.ParentPolicyID)
	rightsFingerprintOptionalInt64(digest, request.ApprovedByUserID)
	return hex.EncodeToString(digest.Sum(nil))
}

func rightsCreatePolicyCommandFingerprint(request CreateRightsPolicyRepositoryDTO) string {
	digest := sha256.New()
	rightsFingerprintString(digest, "rights-policy-create-command-v1")
	rightsFingerprintInt64(digest, request.ActorID)
	rightsFingerprintString(digest, request.PolicyHash)
	return hex.EncodeToString(digest.Sum(nil))
}

func rightsRecordDecisionCommandFingerprint(request RecordRightsDecisionRepositoryDTO) string {
	digest := sha256.New()
	rightsFingerprintString(digest, "rights-decision-record-command-v1")
	rightsFingerprintInt64(digest, request.ActorID)
	rightsFingerprintInt64(digest, request.SourceConnectionID)
	rightsFingerprintInt64(digest, request.Policy.ID)
	rightsFingerprintInt64(digest, request.ExpectedPolicyVersion)
	rightsFingerprintInt64(digest, request.Policy.Revision)
	rightsFingerprintString(digest, request.SubjectType)
	rightsFingerprintString(digest, request.SubjectKey)
	rightsFingerprintString(digest, request.InputDigest)
	rightsFingerprintInt64(digest, int64(len(request.Decisions)))
	for _, decision := range request.Decisions {
		rightsFingerprintString(digest, decision.Action)
		rightsFingerprintString(digest, decision.Decision)
		rightsFingerprintInt64(digest, int64(len(decision.ReasonCodes)))
		for _, reason := range decision.ReasonCodes {
			rightsFingerprintString(digest, reason)
		}
		rightsFingerprintString(digest, decision.Evaluator)
		rightsFingerprintTime(digest, decision.EvaluatedAt)
		rightsFingerprintTime(digest, decision.EffectiveFrom)
		rightsFingerprintOptionalTime(digest, decision.ExpiresAt)
		rightsFingerprintOptionalInt(digest, decision.RetentionDays)
		rightsFingerprintOptionalInt64(digest, decision.SupersedesDecisionID)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func rightsFingerprintString(digest hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write([]byte(value))
}

func rightsFingerprintInt64(digest hash.Hash, value int64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	_, _ = digest.Write(encoded[:])
}

func rightsFingerprintTime(digest hash.Hash, value time.Time) {
	rightsFingerprintString(digest, value.UTC().Format(time.RFC3339Nano))
}

func rightsFingerprintOptionalTime(digest hash.Hash, value *time.Time) {
	if value == nil {
		rightsFingerprintString(digest, "nil")
		return
	}
	rightsFingerprintString(digest, "time")
	rightsFingerprintTime(digest, *value)
}

func rightsFingerprintOptionalInt64(digest hash.Hash, value *int64) {
	if value == nil {
		rightsFingerprintString(digest, "nil")
		return
	}
	rightsFingerprintString(digest, "int64")
	rightsFingerprintInt64(digest, *value)
}

func rightsFingerprintOptionalInt(digest hash.Hash, value *int) {
	if value == nil {
		rightsFingerprintString(digest, "nil")
		return
	}
	rightsFingerprintString(digest, "int")
	rightsFingerprintInt64(digest, int64(*value))
}

func rightsPriorityMatchesScope(priority domain.RightsPriority, scope domain.RightsScopeType) bool {
	switch scope {
	case domain.RightsScopeOrganizationDefault:
		return priority == domain.RightsPriorityOrganizationDefault
	case domain.RightsScopeSourceEndpoint, domain.RightsScopePublisher, domain.RightsScopeFeedOrAccount:
		return priority == domain.RightsPriorityConnectorRestriction || priority == domain.RightsPriorityEndpointContract
	case domain.RightsScopeObservation:
		return priority == domain.RightsPriorityObservationExplicit
	default:
		return false
	}
}

func validRightsManagementIdempotencyKey(value string) bool {
	if value == "" || len(value) > 128 || value != strings.TrimSpace(value) {
		return false
	}
	for index, character := range value {
		alphanumeric := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
		if alphanumeric || index > 0 && (character == '.' || character == '_' || character == '-' || character == ':') {
			continue
		}
		return false
	}
	return true
}

func validRightsManagementText(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func validRightsManagementSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validRightsManagementReasonCodes(values []string) bool {
	if len(values) > 32 {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validRightsManagementText(value, 64) || value != strings.ToLower(value) {
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

func rightsManagementInvalidInput(message string) error {
	return fmt.Errorf("%w: %s", sharedrepository.ErrInvalidInput, message)
}

func cloneRightsManagementPolicy(value RightsPolicyDTO) RightsPolicyDTO {
	result := value
	result.SourceConnectionID = rightsManagementInt64Pointer(value.SourceConnectionID)
	result.ExpiresAt = rightsManagementTimePointer(value.ExpiresAt)
	result.ParentPolicyID = rightsManagementInt64Pointer(value.ParentPolicyID)
	result.ApprovedByUserID = rightsManagementInt64Pointer(value.ApprovedByUserID)
	return result
}

func cloneRightsManagementDecision(value RightsDecisionDTO) RightsDecisionDTO {
	result := value
	result.ReasonCodes = append([]string(nil), value.ReasonCodes...)
	result.ExpiresAt = rightsManagementTimePointer(value.ExpiresAt)
	result.RetentionDays = rightsManagementIntPointer(value.RetentionDays)
	result.SupersedesDecisionID = rightsManagementInt64Pointer(value.SupersedesDecisionID)
	return result
}

func cloneRightsManagementDecisions(values []RightsDecisionDTO) []RightsDecisionDTO {
	result := make([]RightsDecisionDTO, len(values))
	for index, value := range values {
		result[index] = cloneRightsManagementDecision(value)
	}
	return result
}

func cloneRightsManagementRecordRequest(value RecordRightsDecisionRepositoryDTO) RecordRightsDecisionRepositoryDTO {
	result := value
	result.Policy = cloneRightsManagementPolicy(value.Policy)
	result.Decisions = make([]RightsActionDecisionDTO, len(value.Decisions))
	for index, decision := range value.Decisions {
		result.Decisions[index] = RightsActionDecisionDTO{
			Action: decision.Action, Decision: decision.Decision, ReasonCodes: append([]string(nil), decision.ReasonCodes...),
			Evaluator: decision.Evaluator, EvaluatedAt: decision.EvaluatedAt, EffectiveFrom: decision.EffectiveFrom,
			ExpiresAt: rightsManagementTimePointer(decision.ExpiresAt), RetentionDays: rightsManagementIntPointer(decision.RetentionDays),
			SupersedesDecisionID: rightsManagementInt64Pointer(decision.SupersedesDecisionID),
		}
	}
	return result
}

func rightsManagementIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func rightsManagementInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func rightsManagementTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func rightsManagementPersistenceTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Microsecond)
}

func rightsManagementPersistenceTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := rightsManagementPersistenceTime(*value)
	return &result
}

func rightsManagementOptionalIntEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func rightsManagementOptionalInt64Equal(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func rightsManagementOptionalTimeEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func rightsManagementStringsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
