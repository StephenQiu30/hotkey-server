package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

const (
	defaultRightsManagementPageLimit  = 50
	maximumRightsManagementPageLimit  = 100
	maximumRightsManagementCursorSize = 4096

	SourceEndpointCapabilityAvailable   = "available"
	SourceEndpointCapabilityUnavailable = "unavailable"
	SourceEndpointRightsPolicyRequired  = "policy_required"
	SourceEndpointRightsUnavailable     = "unavailable"
)

var orderedRightsActions = []string{
	"fetch", "store_raw", "store_derived", "display_private", "redistribute",
	"quote", "embed_local", "send_external_model", "retain",
}

type RightsReadOperation string

const (
	RightsReadSourceEndpointCapability RightsReadOperation = "rights_capability.read_public"
	RightsReadPolicyHistory            RightsReadOperation = "rights_policy.read_history"
	RightsReadDecisionHistory          RightsReadOperation = "rights_decision.read_history"
	RightsReadExactActionMatrix        RightsReadOperation = "rights_evaluation.read_exact"
)

type RightsReadAuthorizationDTO struct {
	ActorID          int64
	Operation        RightsReadOperation
	SourceEndpointID int64
}

// RightsReadAuthorizer resolves current durable identity state. Application
// reads never trust a role string received through Transport.
type RightsReadAuthorizer interface {
	AuthorizeRightsRead(context.Context, RightsReadAuthorizationDTO) error
}

// GetSourceEndpointCapabilityQuery deliberately has no subject or digest.
// The public projection reports connector mechanics, never an authorization.
type GetSourceEndpointCapabilityQuery struct {
	ActorID          int64
	SourceEndpointID int64
}

// SourceEndpointCapabilityFactsDTO contains the fixed, credential-free Source
// facts needed to build the public connector projection.
type SourceEndpointCapabilityFactsDTO struct {
	SourceEndpointID int64
	SourceType       string
	Enabled          bool
	HealthStatus     string
	Deleted          bool
}

// SourceEndpointCapabilityDTO cannot express allow/deny. Every content action
// still requires an exact immutable RightsDecision evaluation.
type SourceEndpointCapabilityDTO struct {
	SourceEndpointID    int64
	SourceType          string
	CollectionInterface string
	ContentScope        string
	FollowsCanonicalURL bool
	Availability        string
	RightsStatus        string
}

type ListRightsPoliciesQuery struct {
	ActorID          int64
	SourceEndpointID int64
	Cursor           string
	Limit            int
}

type ListRightsPoliciesRepositoryDTO struct {
	SourceEndpointID int64
	Cursor           string
	Limit            int
}

type RightsPolicyReadDTO struct {
	ID               int64
	Version          int64
	SourceEndpointID *int64
	ScopeType        string
	ScopeSubject     string
	Revision         int64
	Priority         int
	BasisSummary     string
	TermsURL         string
	LicenseURI       string
	PolicyHash       string
	EffectiveFrom    time.Time
	ExpiresAt        *time.Time
	ParentPolicyID   *int64
	RecordedByUserID int64
	ApprovedByUserID *int64
	CreatedAt        time.Time
}

type ListRightsPoliciesRepositoryResultDTO struct {
	Items      []RightsPolicyReadDTO
	NextCursor string
}

type ListRightsPoliciesResult struct {
	Items      []RightsPolicyReadDTO
	NextCursor string
}

type ListRightsDecisionBatchesQuery struct {
	ActorID          int64
	SourceEndpointID int64
	Cursor           string
	Limit            int
}

type ListRightsDecisionBatchesRepositoryDTO struct {
	SourceEndpointID int64
	Cursor           string
	Limit            int
}

// RightsDecisionReadDTO is a safe administrator projection. It has no raw
// response body, object storage address, command fingerprint, or source secret.
type RightsDecisionReadDTO struct {
	ID                   int64
	DecisionBatchID      int64
	SourceEndpointID     int64
	PolicyID             int64
	PolicyRevision       int64
	PolicyScopeType      string
	PolicyScopeSubject   string
	Priority             int
	BasisSummary         string
	TermsURL             string
	LicenseURI           string
	SubjectType          string
	SubjectKey           string
	InputDigest          string
	Action               string
	Decision             string
	ReasonCodes          []string
	Evaluator            string
	EvaluatedAt          time.Time
	EffectiveFrom        time.Time
	ExpiresAt            *time.Time
	RetentionDays        *int
	SupersedesDecisionID *int64
	RecordedByUserID     int64
	CreatedAt            time.Time
}

type RightsDecisionBatchDTO struct {
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
	Decisions             []RightsDecisionReadDTO
}

type ListRightsDecisionBatchesRepositoryResultDTO struct {
	Items      []RightsDecisionBatchDTO
	NextCursor string
}

type ListRightsDecisionBatchesResult struct {
	Items      []RightsDecisionBatchDTO
	NextCursor string
}

type GetRightsDecisionQuery struct {
	ActorID          int64
	SourceEndpointID int64
	DecisionID       int64
}

type FindRightsDecisionReadRepositoryDTO struct {
	SourceEndpointID int64
	DecisionID       int64
}

// EvaluateRightsActionMatrixQuery is accepted only by the administrator-only
// POST transport. SubjectKey and InputDigest must never be placed in a URL.
type EvaluateRightsActionMatrixQuery struct {
	ActorID          int64
	SourceEndpointID int64
	SubjectType      string
	SubjectKey       string
	InputDigest      string
	At               time.Time
}

type EvaluateRightsActionMatrixRepositoryDTO struct {
	SourceEndpointID int64
	SubjectType      string
	SubjectKey       string
	InputDigest      string
	At               time.Time
}

type RightsActionCapabilityDTO struct {
	Action        string
	Decision      string
	DecisionIDs   []int64
	PolicyIDs     []int64
	Priority      *int
	RetentionDays *int
}

// RightsActionMatrixDTO intentionally omits SubjectKey and InputDigest so the
// response cannot echo exact asset identities through a broad management UI.
type RightsActionMatrixDTO struct {
	SourceEndpointID int64
	EvaluatedAt      time.Time
	Actions          []RightsActionCapabilityDTO
}

type RightsManagementProjectionRepository interface {
	FindSourceEndpointCapabilityFacts(context.Context, int64) (SourceEndpointCapabilityFactsDTO, error)
	ListRightsPolicies(context.Context, ListRightsPoliciesRepositoryDTO) (ListRightsPoliciesRepositoryResultDTO, error)
	ListRightsDecisionBatches(context.Context, ListRightsDecisionBatchesRepositoryDTO) (ListRightsDecisionBatchesRepositoryResultDTO, error)
	FindRightsDecisionRead(context.Context, FindRightsDecisionReadRepositoryDTO) (RightsDecisionReadDTO, error)
	EvaluateRightsActionMatrix(context.Context, EvaluateRightsActionMatrixRepositoryDTO) (RightsActionMatrixDTO, error)
}

func (service *RightsManagementService) GetSourceEndpointCapability(ctx context.Context, query GetSourceEndpointCapabilityQuery) (SourceEndpointCapabilityDTO, error) {
	if err := service.authorizeRightsRead(ctx, query.ActorID, query.SourceEndpointID, RightsReadSourceEndpointCapability); err != nil {
		return SourceEndpointCapabilityDTO{}, err
	}
	facts, err := service.projection.FindSourceEndpointCapabilityFacts(ctx, query.SourceEndpointID)
	if err != nil {
		return SourceEndpointCapabilityDTO{}, rightsManagementProjectionError(err)
	}
	if facts.SourceEndpointID != query.SourceEndpointID {
		return SourceEndpointCapabilityDTO{}, fmt.Errorf("%w: source endpoint capability projection is misbound", sharedrepository.ErrConstraint)
	}
	result, err := sourceEndpointCapability(facts)
	if err != nil {
		return SourceEndpointCapabilityDTO{}, err
	}
	return result, nil
}

func (service *RightsManagementService) ListRightsPolicies(ctx context.Context, query ListRightsPoliciesQuery) (ListRightsPoliciesResult, error) {
	if err := service.authorizeRightsRead(ctx, query.ActorID, query.SourceEndpointID, RightsReadPolicyHistory); err != nil {
		return ListRightsPoliciesResult{}, err
	}
	request, err := rightsManagementPolicyListRequest(query)
	if err != nil {
		return ListRightsPoliciesResult{}, err
	}
	page, err := service.projection.ListRightsPolicies(ctx, request)
	if err != nil {
		return ListRightsPoliciesResult{}, rightsManagementProjectionError(err)
	}
	items := make([]RightsPolicyReadDTO, len(page.Items))
	for index, policy := range page.Items {
		if err := validateRightsPolicyReadDTO(policy); err != nil ||
			policy.SourceEndpointID != nil && *policy.SourceEndpointID != query.SourceEndpointID {
			return ListRightsPoliciesResult{}, fmt.Errorf("%w: invalid rights policy read projection", sharedrepository.ErrConstraint)
		}
		items[index] = cloneRightsPolicyReadDTO(policy)
	}
	if !validRightsManagementCursor(page.NextCursor) {
		return ListRightsPoliciesResult{}, fmt.Errorf("%w: invalid rights policy next cursor", sharedrepository.ErrConstraint)
	}
	return ListRightsPoliciesResult{Items: items, NextCursor: page.NextCursor}, nil
}

func (service *RightsManagementService) ListRightsDecisionBatches(ctx context.Context, query ListRightsDecisionBatchesQuery) (ListRightsDecisionBatchesResult, error) {
	if err := service.authorizeRightsRead(ctx, query.ActorID, query.SourceEndpointID, RightsReadDecisionHistory); err != nil {
		return ListRightsDecisionBatchesResult{}, err
	}
	request, err := rightsManagementDecisionBatchListRequest(query)
	if err != nil {
		return ListRightsDecisionBatchesResult{}, err
	}
	page, err := service.projection.ListRightsDecisionBatches(ctx, request)
	if err != nil {
		return ListRightsDecisionBatchesResult{}, rightsManagementProjectionError(err)
	}
	items := make([]RightsDecisionBatchDTO, len(page.Items))
	for index, batch := range page.Items {
		if err := validateRightsDecisionBatchDTO(batch); err != nil || batch.SourceEndpointID != query.SourceEndpointID {
			return ListRightsDecisionBatchesResult{}, fmt.Errorf("%w: invalid rights decision batch read projection", sharedrepository.ErrConstraint)
		}
		items[index] = cloneRightsDecisionBatchDTO(batch)
	}
	if !validRightsManagementCursor(page.NextCursor) {
		return ListRightsDecisionBatchesResult{}, fmt.Errorf("%w: invalid rights decision batch next cursor", sharedrepository.ErrConstraint)
	}
	return ListRightsDecisionBatchesResult{Items: items, NextCursor: page.NextCursor}, nil
}

func (service *RightsManagementService) GetRightsDecision(ctx context.Context, query GetRightsDecisionQuery) (RightsDecisionReadDTO, error) {
	if err := service.authorizeRightsRead(ctx, query.ActorID, query.SourceEndpointID, RightsReadDecisionHistory); err != nil {
		return RightsDecisionReadDTO{}, err
	}
	if query.DecisionID <= 0 {
		return RightsDecisionReadDTO{}, rightsManagementInvalidInput("rights decision identity is invalid")
	}
	decision, err := service.projection.FindRightsDecisionRead(ctx, FindRightsDecisionReadRepositoryDTO{
		SourceEndpointID: query.SourceEndpointID, DecisionID: query.DecisionID,
	})
	if err != nil {
		return RightsDecisionReadDTO{}, rightsManagementProjectionError(err)
	}
	if decision.SourceEndpointID != query.SourceEndpointID || validateRightsDecisionReadDTO(decision) != nil {
		return RightsDecisionReadDTO{}, rightsManagementProjectionError(sharedrepository.ErrNotFound)
	}
	return cloneRightsDecisionReadDTO(decision), nil
}

func (service *RightsManagementService) EvaluateRightsActionMatrix(ctx context.Context, query EvaluateRightsActionMatrixQuery) (RightsActionMatrixDTO, error) {
	if err := service.authorizeRightsRead(ctx, query.ActorID, query.SourceEndpointID, RightsReadExactActionMatrix); err != nil {
		return RightsActionMatrixDTO{}, err
	}
	request, err := rightsActionMatrixRequest(query)
	if err != nil {
		return RightsActionMatrixDTO{}, err
	}
	result, err := service.projection.EvaluateRightsActionMatrix(ctx, request)
	if err != nil {
		return RightsActionMatrixDTO{}, rightsManagementProjectionError(err)
	}
	if err := validateRightsActionMatrixDTO(request, result); err != nil {
		return RightsActionMatrixDTO{}, err
	}
	return cloneRightsActionMatrixDTO(result), nil
}

func (service *RightsManagementService) authorizeRightsRead(ctx context.Context, actorID, sourceEndpointID int64, operation RightsReadOperation) error {
	if service == nil || service.projection == nil || service.readAuthorizer == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	if actorID <= 0 || sourceEndpointID <= 0 {
		return rightsManagementInvalidInput("rights read actor or source endpoint identity is invalid")
	}
	if err := service.readAuthorizer.AuthorizeRightsRead(ctx, RightsReadAuthorizationDTO{
		ActorID: actorID, Operation: operation, SourceEndpointID: sourceEndpointID,
	}); err != nil {
		return rightsManagementProjectionError(err)
	}
	return nil
}

func sourceEndpointCapability(facts SourceEndpointCapabilityFactsDTO) (SourceEndpointCapabilityDTO, error) {
	if facts.SourceEndpointID <= 0 || !domain.SourceType(facts.SourceType).Valid() {
		return SourceEndpointCapabilityDTO{}, fmt.Errorf("%w: invalid source endpoint capability facts", sharedrepository.ErrConstraint)
	}
	collectionInterface, contentScope := "", ""
	switch domain.SourceType(facts.SourceType) {
	case domain.SourceTypeRSS:
		collectionInterface, contentScope = "rss_atom_feed", "feed_payload"
	case domain.SourceTypeHackerNews:
		collectionInterface, contentScope = "official_api", "platform_post"
	case domain.SourceTypeX, domain.SourceTypeBilibili, domain.SourceTypeWeibo:
		collectionInterface, contentScope = "authorized_official_api", "platform_post"
	case domain.SourceTypeBingGrounding, domain.SourceTypeGoogleAgentSearch:
		collectionInterface, contentScope = "authorized_official_api", "discovery_snippet"
	default:
		return SourceEndpointCapabilityDTO{}, fmt.Errorf("%w: unsupported source endpoint capability", sharedrepository.ErrConstraint)
	}
	availability, rightsStatus := SourceEndpointCapabilityAvailable, SourceEndpointRightsPolicyRequired
	if facts.Deleted || !facts.Enabled || facts.HealthStatus == string(domain.HealthStatusUnavailable) {
		availability, rightsStatus = SourceEndpointCapabilityUnavailable, SourceEndpointRightsUnavailable
	}
	return SourceEndpointCapabilityDTO{
		SourceEndpointID: facts.SourceEndpointID, SourceType: facts.SourceType,
		CollectionInterface: collectionInterface, ContentScope: contentScope, FollowsCanonicalURL: false,
		Availability: availability, RightsStatus: rightsStatus,
	}, nil
}

func rightsManagementPolicyListRequest(query ListRightsPoliciesQuery) (ListRightsPoliciesRepositoryDTO, error) {
	limit, err := normalizeRightsManagementPage(query.SourceEndpointID, query.Cursor, query.Limit)
	if err != nil {
		return ListRightsPoliciesRepositoryDTO{}, err
	}
	return ListRightsPoliciesRepositoryDTO{SourceEndpointID: query.SourceEndpointID, Cursor: query.Cursor, Limit: limit}, nil
}

func rightsManagementDecisionBatchListRequest(query ListRightsDecisionBatchesQuery) (ListRightsDecisionBatchesRepositoryDTO, error) {
	limit, err := normalizeRightsManagementPage(query.SourceEndpointID, query.Cursor, query.Limit)
	if err != nil {
		return ListRightsDecisionBatchesRepositoryDTO{}, err
	}
	return ListRightsDecisionBatchesRepositoryDTO{SourceEndpointID: query.SourceEndpointID, Cursor: query.Cursor, Limit: limit}, nil
}

func normalizeRightsManagementPage(sourceEndpointID int64, cursor string, limit int) (int, error) {
	if sourceEndpointID <= 0 || !validRightsManagementCursor(cursor) {
		return 0, rightsManagementInvalidInput("rights management page is invalid")
	}
	if limit == 0 {
		limit = defaultRightsManagementPageLimit
	}
	if limit < 1 || limit > maximumRightsManagementPageLimit {
		return 0, rightsManagementInvalidInput("rights management page limit is invalid")
	}
	return limit, nil
}

func rightsActionMatrixRequest(query EvaluateRightsActionMatrixQuery) (EvaluateRightsActionMatrixRepositoryDTO, error) {
	if query.SourceEndpointID <= 0 || !domain.RightsSubjectType(query.SubjectType).Valid() ||
		!validRightsManagementText(query.SubjectKey, 512) || !validRightsManagementSHA256(query.InputDigest) || query.At.IsZero() {
		return EvaluateRightsActionMatrixRepositoryDTO{}, rightsManagementInvalidInput("exact rights evaluation input is invalid")
	}
	return EvaluateRightsActionMatrixRepositoryDTO{
		SourceEndpointID: query.SourceEndpointID, SubjectType: query.SubjectType,
		SubjectKey: query.SubjectKey, InputDigest: query.InputDigest, At: rightsManagementPersistenceTime(query.At),
	}, nil
}

func validateRightsPolicyReadDTO(policy RightsPolicyReadDTO) error {
	applicationPolicy := RightsPolicyDTO{
		ID: policy.ID, Version: policy.Version, SourceConnectionID: rightsManagementInt64Pointer(policy.SourceEndpointID),
		ScopeType: policy.ScopeType, ScopeSubject: policy.ScopeSubject, Revision: policy.Revision, Priority: policy.Priority,
		BasisSummary: policy.BasisSummary, TermsURL: policy.TermsURL, LicenseURI: policy.LicenseURI,
		PolicyHash: policy.PolicyHash, EffectiveFrom: policy.EffectiveFrom, ExpiresAt: rightsManagementTimePointer(policy.ExpiresAt),
		ParentPolicyID: rightsManagementInt64Pointer(policy.ParentPolicyID), ApprovedByUserID: rightsManagementInt64Pointer(policy.ApprovedByUserID),
	}
	if err := validateRightsPolicyDTO(applicationPolicy); err != nil || policy.RecordedByUserID <= 0 || policy.CreatedAt.IsZero() {
		return fmt.Errorf("invalid rights policy history")
	}
	return nil
}

func validateRightsDecisionBatchDTO(batch RightsDecisionBatchDTO) error {
	if batch.ID <= 0 || batch.Version != 1 || batch.SourceEndpointID <= 0 || batch.PolicyID <= 0 ||
		batch.ExpectedPolicyVersion <= 0 || batch.RecordedByUserID <= 0 || batch.DecisionCount < 1 ||
		batch.DecisionCount != len(batch.Decisions) || batch.CreatedAt.IsZero() ||
		!domain.RightsSubjectType(batch.SubjectType).Valid() || !validRightsManagementText(batch.SubjectKey, 512) ||
		!validRightsManagementSHA256(batch.InputDigest) {
		return fmt.Errorf("invalid rights decision batch")
	}
	seen := make(map[string]struct{}, len(batch.Decisions))
	for _, decision := range batch.Decisions {
		if validateRightsDecisionReadDTO(decision) != nil || decision.DecisionBatchID != batch.ID ||
			decision.SourceEndpointID != batch.SourceEndpointID || decision.PolicyID != batch.PolicyID ||
			decision.SubjectType != batch.SubjectType || decision.SubjectKey != batch.SubjectKey || decision.InputDigest != batch.InputDigest {
			return fmt.Errorf("invalid rights decision batch member")
		}
		if _, duplicate := seen[decision.Action]; duplicate {
			return fmt.Errorf("duplicate rights decision batch action")
		}
		seen[decision.Action] = struct{}{}
	}
	return nil
}

func validateRightsDecisionReadDTO(decision RightsDecisionReadDTO) error {
	applicationDecision := RightsDecisionDTO{
		ID: decision.ID, DecisionBatchID: decision.DecisionBatchID, SourceConnectionID: decision.SourceEndpointID,
		PolicyID: decision.PolicyID, PolicyRevision: decision.PolicyRevision, PolicyScopeType: decision.PolicyScopeType,
		PolicyScopeSubject: decision.PolicyScopeSubject, Priority: decision.Priority, BasisSummary: decision.BasisSummary,
		TermsURL: decision.TermsURL, LicenseURI: decision.LicenseURI, SubjectType: decision.SubjectType,
		SubjectKey: decision.SubjectKey, InputDigest: decision.InputDigest, Action: decision.Action, Decision: decision.Decision,
		ReasonCodes: append([]string(nil), decision.ReasonCodes...), Evaluator: decision.Evaluator,
		EvaluatedAt: decision.EvaluatedAt, EffectiveFrom: decision.EffectiveFrom,
		ExpiresAt: rightsManagementTimePointer(decision.ExpiresAt), RetentionDays: rightsManagementIntPointer(decision.RetentionDays),
		SupersedesDecisionID: rightsManagementInt64Pointer(decision.SupersedesDecisionID),
	}
	if validateRightsDecisionDTO(applicationDecision) != nil || decision.RecordedByUserID <= 0 || decision.CreatedAt.IsZero() {
		return fmt.Errorf("invalid rights decision history")
	}
	return nil
}

func validateRightsActionMatrixDTO(query EvaluateRightsActionMatrixRepositoryDTO, matrix RightsActionMatrixDTO) error {
	if matrix.SourceEndpointID != query.SourceEndpointID || !matrix.EvaluatedAt.Equal(query.At) || len(matrix.Actions) != len(orderedRightsActions) {
		return fmt.Errorf("%w: invalid exact rights action matrix identity", sharedrepository.ErrConstraint)
	}
	seen := make(map[string]struct{}, len(matrix.Actions))
	for _, item := range matrix.Actions {
		action := domain.RightsAction(item.Action)
		decision := domain.RightsState(item.Decision)
		if !action.Valid() || !decision.Valid() {
			return fmt.Errorf("%w: invalid exact rights action result", sharedrepository.ErrConstraint)
		}
		if _, duplicate := seen[item.Action]; duplicate {
			return fmt.Errorf("%w: duplicate exact rights action result", sharedrepository.ErrConstraint)
		}
		seen[item.Action] = struct{}{}
		if !positiveDistinctInt64s(item.DecisionIDs) || !positiveDistinctInt64s(item.PolicyIDs) {
			return fmt.Errorf("%w: invalid exact rights action receipts", sharedrepository.ErrConstraint)
		}
		if len(item.DecisionIDs) == 0 {
			if item.Decision != string(domain.RightsUnknown) || len(item.PolicyIDs) != 0 || item.Priority != nil || item.RetentionDays != nil {
				return fmt.Errorf("%w: decision-free action must be unknown", sharedrepository.ErrConstraint)
			}
		} else if item.Priority == nil || !domain.RightsPriority(*item.Priority).Valid() {
			return fmt.Errorf("%w: decided action priority is invalid", sharedrepository.ErrConstraint)
		}
		if item.Action == string(domain.RightsActionRetain) && item.Decision == string(domain.RightsAllow) {
			if item.RetentionDays == nil || *item.RetentionDays < 1 || *item.RetentionDays > 3650 {
				return fmt.Errorf("%w: retain action duration is invalid", sharedrepository.ErrConstraint)
			}
		} else if item.RetentionDays != nil {
			return fmt.Errorf("%w: action unexpectedly exposes retention", sharedrepository.ErrConstraint)
		}
	}
	for _, action := range orderedRightsActions {
		if _, found := seen[action]; !found {
			return fmt.Errorf("%w: exact rights action matrix is incomplete", sharedrepository.ErrConstraint)
		}
	}
	return nil
}

func positiveDistinctInt64s(values []int64) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validRightsManagementCursor(value string) bool {
	return len(value) <= maximumRightsManagementCursorSize && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func rightsManagementProjectionError(err error) error {
	if err == nil {
		return nil
	}
	var applicationError *sharederrors.AppError
	if errors.As(err, &applicationError) {
		return applicationError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, 400, "", err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, 404, "", err)
	case errors.Is(err, sharedrepository.ErrConflict), errors.Is(err, sharedrepository.ErrConstraint):
		return sharederrors.Wrap(sharederrors.CodeConflict, 409, "", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, 503, "", err)
	default:
		return err
	}
}

func cloneRightsPolicyReadDTO(value RightsPolicyReadDTO) RightsPolicyReadDTO {
	result := value
	result.SourceEndpointID = rightsManagementInt64Pointer(value.SourceEndpointID)
	result.ExpiresAt = rightsManagementTimePointer(value.ExpiresAt)
	result.ParentPolicyID = rightsManagementInt64Pointer(value.ParentPolicyID)
	result.ApprovedByUserID = rightsManagementInt64Pointer(value.ApprovedByUserID)
	return result
}

func cloneRightsDecisionReadDTO(value RightsDecisionReadDTO) RightsDecisionReadDTO {
	result := value
	result.ReasonCodes = append([]string(nil), value.ReasonCodes...)
	result.ExpiresAt = rightsManagementTimePointer(value.ExpiresAt)
	result.RetentionDays = rightsManagementIntPointer(value.RetentionDays)
	result.SupersedesDecisionID = rightsManagementInt64Pointer(value.SupersedesDecisionID)
	return result
}

func cloneRightsDecisionBatchDTO(value RightsDecisionBatchDTO) RightsDecisionBatchDTO {
	result := value
	result.Decisions = make([]RightsDecisionReadDTO, len(value.Decisions))
	for index, decision := range value.Decisions {
		result.Decisions[index] = cloneRightsDecisionReadDTO(decision)
	}
	return result
}

func cloneRightsActionMatrixDTO(value RightsActionMatrixDTO) RightsActionMatrixDTO {
	result := value
	result.Actions = make([]RightsActionCapabilityDTO, len(value.Actions))
	for index, item := range value.Actions {
		item.DecisionIDs = append([]int64(nil), item.DecisionIDs...)
		item.PolicyIDs = append([]int64(nil), item.PolicyIDs...)
		item.Priority = rightsManagementIntPointer(item.Priority)
		item.RetentionDays = rightsManagementIntPointer(item.RetentionDays)
		sort.Slice(item.DecisionIDs, func(left, right int) bool { return item.DecisionIDs[left] < item.DecisionIDs[right] })
		sort.Slice(item.PolicyIDs, func(left, right int) bool { return item.PolicyIDs[left] < item.PolicyIDs[right] })
		result.Actions[index] = item
	}
	return result
}
