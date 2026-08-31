package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type RightsManagementOperation string

const (
	RightsManagementCreatePolicy    RightsManagementOperation = "rights_policy.create"
	RightsManagementRecordDecisions RightsManagementOperation = "rights_decision.record_batch"
)

// CreateRightsPolicyCommand is an immutable Application input. PolicyHash and
// command fingerprint are derived internally; callers cannot substitute either
// identity with legacy allow_body_storage or another mutable setting.
type CreateRightsPolicyCommand struct {
	ActorID            int64
	IdempotencyKey     string
	SourceConnectionID *int64
	ScopeType          string
	ScopeSubject       string
	Revision           int64
	Priority           int
	BasisSummary       string
	TermsURL           string
	LicenseURI         string
	EffectiveFrom      time.Time
	ExpiresAt          *time.Time
	ParentPolicyID     *int64
	ApprovedByUserID   *int64
}

type RightsPolicyDTO struct {
	ID                 int64
	Version            int64
	SourceConnectionID *int64
	ScopeType          string
	ScopeSubject       string
	Revision           int64
	Priority           int
	BasisSummary       string
	TermsURL           string
	LicenseURI         string
	PolicyHash         string
	EffectiveFrom      time.Time
	ExpiresAt          *time.Time
	ParentPolicyID     *int64
	ApprovedByUserID   *int64
}

type CreateRightsPolicyResult struct {
	Policy           RightsPolicyDTO
	IdempotentReplay bool
}

// RightsActionDecisionDTO is exactly one action fact in an atomic batch. The
// subject, input digest, source, and policy are shared by the enclosing command.
type RightsActionDecisionDTO struct {
	Action               string
	Decision             string
	ReasonCodes          []string
	Evaluator            string
	EvaluatedAt          time.Time
	EffectiveFrom        time.Time
	ExpiresAt            *time.Time
	RetentionDays        *int
	SupersedesDecisionID *int64
}

type RecordRightsDecisionCommand struct {
	ActorID               int64
	IdempotencyKey        string
	SourceConnectionID    int64
	PolicyID              int64
	ExpectedPolicyVersion int64
	SubjectType           string
	SubjectKey            string
	InputDigest           string
	Decisions             []RightsActionDecisionDTO
}

// RightsDecisionDTO is the safe Application projection of one persisted,
// single-action decision and its immutable policy snapshot.
type RightsDecisionDTO struct {
	ID                   int64
	DecisionBatchID      int64
	SourceConnectionID   int64
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
}

type RecordRightsDecisionResult struct {
	DecisionBatchID  int64
	Decisions        []RightsDecisionDTO
	IdempotentReplay bool
}

// CreateRightsPolicyRepositoryDTO explicitly carries durable idempotency
// facts. PostgreSQL must persist both values rather than deriving them.
type CreateRightsPolicyRepositoryDTO struct {
	ActorID            int64
	IdempotencyKey     string
	CommandFingerprint string
	SourceConnectionID *int64
	ScopeType          string
	ScopeSubject       string
	Revision           int64
	Priority           int
	BasisSummary       string
	TermsURL           string
	LicenseURI         string
	PolicyHash         string
	EffectiveFrom      time.Time
	ExpiresAt          *time.Time
	ParentPolicyID     *int64
	ApprovedByUserID   *int64
}

type CreateRightsPolicyRepositoryResultDTO struct {
	Policy           RightsPolicyDTO
	IdempotentReplay bool
}

type RecordRightsDecisionRepositoryDTO struct {
	ActorID               int64
	IdempotencyKey        string
	CommandFingerprint    string
	SourceConnectionID    int64
	ExpectedPolicyVersion int64
	Policy                RightsPolicyDTO
	SubjectType           string
	SubjectKey            string
	InputDigest           string
	Decisions             []RightsActionDecisionDTO
}

type RecordRightsDecisionRepositoryResultDTO struct {
	DecisionBatchID  int64
	Decisions        []RightsDecisionDTO
	IdempotentReplay bool
}

// FindRightsPolicyQueryDTO separates the optimistic row contract from the
// policy revision copied into every immutable decision fact.
type FindRightsPolicyQueryDTO struct {
	PolicyID        int64
	ExpectedVersion int64
}

// RightsManagementRepository is an Application port. Implementations must
// atomically record every decision in one batch and implement same-key,
// same-fingerprint replay; a different fingerprint for the same key conflicts.
type RightsManagementRepository interface {
	CreateRightsPolicy(context.Context, CreateRightsPolicyRepositoryDTO) (CreateRightsPolicyRepositoryResultDTO, error)
	FindRightsPolicy(context.Context, FindRightsPolicyQueryDTO) (RightsPolicyDTO, error)
	FindRightsDecision(context.Context, int64) (RightsDecisionDTO, error)
	RecordRightsDecisions(context.Context, RecordRightsDecisionRepositoryDTO) (RecordRightsDecisionRepositoryResultDTO, error)
}

type RightsActorAuthorizationDTO struct {
	ActorID            int64
	Operation          RightsManagementOperation
	SourceConnectionID *int64
	ApprovedByUserID   *int64
}

// RightsActorAuthorizer keeps RBAC outside caller-controlled role strings.
// Every mutation, including an idempotent replay, is authorized first.
type RightsActorAuthorizer interface {
	AuthorizeRightsMutation(context.Context, RightsActorAuthorizationDTO) error
}

type RightsManagementAuditDTO struct {
	ActorID            int64
	Operation          RightsManagementOperation
	SourceConnectionID *int64
	PolicyID           int64
	DecisionBatchID    int64
	DecisionIDs        []int64
	Actions            []string
	IdempotencyKey     string
	CommandFingerprint string
}

type RightsManagementAttemptResult string

const (
	RightsManagementAttemptFailure RightsManagementAttemptResult = "failure"
	RightsManagementAttemptDenied  RightsManagementAttemptResult = "denied"

	RightsManagementReasonInvalidInput          = "invalid_input"
	RightsManagementReasonAuthorizationDenied   = "authorization_denied"
	RightsManagementReasonIdempotencyConflict   = "idempotency_conflict"
	RightsManagementReasonVersionConflict       = "version_conflict"
	RightsManagementReasonDependencyUnavailable = "dependency_unavailable"
)

// RightsManagementAttemptAuditDTO contains only bounded correlation facts.
// Failure attempts deliberately omit command receipts because the same
// idempotency key may already belong to a successful immutable audit row.
type RightsManagementAttemptAuditDTO struct {
	ActorID            int64
	Operation          RightsManagementOperation
	SourceConnectionID *int64
	PolicyID           int64
	IdempotencyKey     string
	CommandFingerprint string
	Result             RightsManagementAttemptResult
	ReasonCode         string
}

type RightsManagementAuditWriter interface {
	WriteRightsMutation(context.Context, RightsManagementAuditDTO) error
	WriteRightsMutationAttempt(context.Context, RightsManagementAttemptAuditDTO) error
}

// RightsManagementTransactionRunner must place Repository and Audit calls in
// one transaction context. Audit failure therefore rolls back the business fact.
type RightsManagementTransactionRunner interface {
	WithinRightsManagementTransaction(context.Context, func(context.Context) error) error
}

type RightsManagementDependencies struct {
	Repository RightsManagementRepository
	// Projection is a read-only, credential-free port. When omitted, the
	// service reuses Repository only when that adapter explicitly implements
	// RightsManagementProjectionRepository as a second interface.
	Projection     RightsManagementProjectionRepository
	Authorizer     RightsActorAuthorizer
	ReadAuthorizer RightsReadAuthorizer
	Audit          RightsManagementAuditWriter
	Transactions   RightsManagementTransactionRunner
}

type RightsManagementService struct {
	repository     RightsManagementRepository
	projection     RightsManagementProjectionRepository
	authorizer     RightsActorAuthorizer
	readAuthorizer RightsReadAuthorizer
	audit          RightsManagementAuditWriter
	transactions   RightsManagementTransactionRunner
}

func NewRightsManagementService(dependencies RightsManagementDependencies) (*RightsManagementService, error) {
	if dependencies.Repository == nil || dependencies.Authorizer == nil || dependencies.Audit == nil || dependencies.Transactions == nil {
		return nil, errors.New("rights management repository, authorizer, audit, and transaction runner are required")
	}
	projection := dependencies.Projection
	if projection == nil {
		projection, _ = dependencies.Repository.(RightsManagementProjectionRepository)
	}
	readAuthorizer := dependencies.ReadAuthorizer
	if readAuthorizer == nil {
		readAuthorizer, _ = dependencies.Authorizer.(RightsReadAuthorizer)
	}
	return &RightsManagementService{
		repository: dependencies.Repository, authorizer: dependencies.Authorizer,
		projection: projection, readAuthorizer: readAuthorizer, audit: dependencies.Audit, transactions: dependencies.Transactions,
	}, nil
}

func (service *RightsManagementService) CreatePolicy(ctx context.Context, command CreateRightsPolicyCommand) (CreateRightsPolicyResult, error) {
	if service == nil || service.repository == nil || service.authorizer == nil || service.audit == nil || service.transactions == nil {
		return CreateRightsPolicyResult{}, errors.New("rights management service is not initialized")
	}
	request, err := prepareCreateRightsPolicy(command)
	if err != nil {
		return CreateRightsPolicyResult{}, service.writeAttemptAudit(ctx, createPolicyAttempt(command, RightsManagementAttemptFailure, RightsManagementReasonInvalidInput), err)
	}
	if err := service.authorizer.AuthorizeRightsMutation(ctx, RightsActorAuthorizationDTO{
		ActorID: command.ActorID, Operation: RightsManagementCreatePolicy,
		SourceConnectionID: rightsManagementInt64Pointer(request.SourceConnectionID),
		ApprovedByUserID:   rightsManagementInt64Pointer(request.ApprovedByUserID),
	}); err != nil {
		result, reason := rightsManagementAuthorizationAttempt(err)
		return CreateRightsPolicyResult{}, service.writeAttemptAudit(ctx, createPolicyAttempt(command, result, reason), err)
	}

	var result CreateRightsPolicyResult
	err = service.transactions.WithinRightsManagementTransaction(ctx, func(transactionCtx context.Context) error {
		stored, err := service.repository.CreateRightsPolicy(transactionCtx, request)
		if err != nil {
			return err
		}
		policy, err := validateCreatedRightsPolicy(request, stored.Policy)
		if err != nil {
			return err
		}
		if !stored.IdempotentReplay {
			if err := service.audit.WriteRightsMutation(transactionCtx, RightsManagementAuditDTO{
				ActorID: command.ActorID, Operation: RightsManagementCreatePolicy,
				SourceConnectionID: rightsManagementInt64Pointer(policy.SourceConnectionID),
				PolicyID:           policy.ID, IdempotencyKey: command.IdempotencyKey,
				CommandFingerprint: request.CommandFingerprint,
			}); err != nil {
				return err
			}
		}
		result = CreateRightsPolicyResult{Policy: cloneRightsManagementPolicy(policy), IdempotentReplay: stored.IdempotentReplay}
		return nil
	})
	if err != nil {
		result, reason := rightsManagementMutationAttempt(err, false)
		return CreateRightsPolicyResult{}, service.writeAttemptAudit(ctx, createPolicyAttempt(command, result, reason), err)
	}
	return result, nil
}

func (service *RightsManagementService) RecordDecisions(ctx context.Context, command RecordRightsDecisionCommand) (RecordRightsDecisionResult, error) {
	if service == nil || service.repository == nil || service.authorizer == nil || service.audit == nil || service.transactions == nil {
		return RecordRightsDecisionResult{}, errors.New("rights management service is not initialized")
	}
	prepared, err := prepareRecordRightsDecision(command)
	if err != nil {
		return RecordRightsDecisionResult{}, service.writeAttemptAudit(ctx, recordDecisionAttempt(command, RightsManagementAttemptFailure, RightsManagementReasonInvalidInput), err)
	}
	sourceConnectionID := command.SourceConnectionID
	if err := service.authorizer.AuthorizeRightsMutation(ctx, RightsActorAuthorizationDTO{
		ActorID: command.ActorID, Operation: RightsManagementRecordDecisions, SourceConnectionID: &sourceConnectionID,
	}); err != nil {
		result, reason := rightsManagementAuthorizationAttempt(err)
		return RecordRightsDecisionResult{}, service.writeAttemptAudit(ctx, recordDecisionAttempt(command, result, reason), err)
	}

	var result RecordRightsDecisionResult
	err = service.transactions.WithinRightsManagementTransaction(ctx, func(transactionCtx context.Context) error {
		policy, err := service.repository.FindRightsPolicy(transactionCtx, FindRightsPolicyQueryDTO{
			PolicyID: command.PolicyID, ExpectedVersion: command.ExpectedPolicyVersion,
		})
		if err != nil {
			return err
		}
		if err := validateRightsPolicyDTO(policy); err != nil {
			return fmt.Errorf("rights policy projection is invalid: %w", err)
		}
		request, err := service.prepareDecisionRepositoryRequest(transactionCtx, prepared, policy)
		if err != nil {
			return err
		}
		stored, err := service.repository.RecordRightsDecisions(transactionCtx, request)
		if err != nil {
			return err
		}
		decisions, err := validateRecordedRightsDecisions(request, stored.DecisionBatchID, stored.Decisions)
		if err != nil {
			return err
		}
		if !stored.IdempotentReplay {
			ids, actions := rightsDecisionAuditFacts(decisions)
			if err := service.audit.WriteRightsMutation(transactionCtx, RightsManagementAuditDTO{
				ActorID: command.ActorID, Operation: RightsManagementRecordDecisions,
				SourceConnectionID: &sourceConnectionID, PolicyID: policy.ID,
				DecisionBatchID: stored.DecisionBatchID,
				DecisionIDs:     ids, Actions: actions, IdempotencyKey: command.IdempotencyKey,
				CommandFingerprint: request.CommandFingerprint,
			}); err != nil {
				return err
			}
		}
		result = RecordRightsDecisionResult{
			DecisionBatchID: stored.DecisionBatchID,
			Decisions:       cloneRightsManagementDecisions(decisions), IdempotentReplay: stored.IdempotentReplay,
		}
		return nil
	})
	if err != nil {
		result, reason := rightsManagementMutationAttempt(err, true)
		return RecordRightsDecisionResult{}, service.writeAttemptAudit(ctx, recordDecisionAttempt(command, result, reason), err)
	}
	return result, nil
}

func (service *RightsManagementService) writeAttemptAudit(ctx context.Context, attempt RightsManagementAttemptAuditDTO, operationErr error) error {
	if operationErr == nil {
		return nil
	}
	auditCtx := context.WithoutCancel(ctx)
	if auditErr := service.audit.WriteRightsMutationAttempt(auditCtx, attempt); auditErr != nil {
		return errors.Join(operationErr, fmt.Errorf("write rights mutation attempt audit: %w", auditErr))
	}
	return operationErr
}

func createPolicyAttempt(command CreateRightsPolicyCommand, result RightsManagementAttemptResult, reason string) RightsManagementAttemptAuditDTO {
	return RightsManagementAttemptAuditDTO{
		ActorID: command.ActorID, Operation: RightsManagementCreatePolicy,
		SourceConnectionID: positiveRightsManagementReference(command.SourceConnectionID),
		Result:             result, ReasonCode: reason,
	}
}

func recordDecisionAttempt(command RecordRightsDecisionCommand, result RightsManagementAttemptResult, reason string) RightsManagementAttemptAuditDTO {
	var sourceConnectionID *int64
	if command.SourceConnectionID > 0 {
		sourceID := command.SourceConnectionID
		sourceConnectionID = rightsManagementInt64Pointer(&sourceID)
	}
	policyID := command.PolicyID
	if policyID < 0 {
		policyID = 0
	}
	return RightsManagementAttemptAuditDTO{
		ActorID: command.ActorID, Operation: RightsManagementRecordDecisions,
		SourceConnectionID: sourceConnectionID, PolicyID: policyID,
		Result: result, ReasonCode: reason,
	}
}

func positiveRightsManagementReference(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	return rightsManagementInt64Pointer(value)
}

func rightsManagementAuthorizationAttempt(err error) (RightsManagementAttemptResult, string) {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) && appError.Code == sharederrors.CodeForbidden {
		return RightsManagementAttemptDenied, RightsManagementReasonAuthorizationDenied
	}
	return RightsManagementAttemptFailure, RightsManagementReasonDependencyUnavailable
}

func rightsManagementMutationAttempt(err error, versioned bool) (RightsManagementAttemptResult, string) {
	switch {
	case errors.Is(err, sharedrepository.ErrConflict):
		return RightsManagementAttemptFailure, RightsManagementReasonIdempotencyConflict
	case versioned && errors.Is(err, sharedrepository.ErrNotFound):
		return RightsManagementAttemptFailure, RightsManagementReasonVersionConflict
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return RightsManagementAttemptFailure, RightsManagementReasonInvalidInput
	default:
		return RightsManagementAttemptFailure, RightsManagementReasonDependencyUnavailable
	}
}

func (service *RightsManagementService) prepareDecisionRepositoryRequest(ctx context.Context, prepared RecordRightsDecisionRepositoryDTO, policy RightsPolicyDTO) (RecordRightsDecisionRepositoryDTO, error) {
	request := cloneRightsManagementRecordRequest(prepared)
	request.Policy = cloneRightsManagementPolicy(policy)
	if err := validateDecisionPolicyCompatibility(request); err != nil {
		return RecordRightsDecisionRepositoryDTO{}, err
	}
	request.CommandFingerprint = rightsRecordDecisionCommandFingerprint(request)
	for _, candidate := range request.Decisions {
		if candidate.SupersedesDecisionID == nil {
			continue
		}
		previous, err := service.repository.FindRightsDecision(ctx, *candidate.SupersedesDecisionID)
		if err != nil {
			return RecordRightsDecisionRepositoryDTO{}, err
		}
		if err := validateRightsDecisionDTO(previous); err != nil {
			return RecordRightsDecisionRepositoryDTO{}, fmt.Errorf("superseded rights decision projection is invalid: %w", err)
		}
		if !rightsSupersessionMatches(request, candidate, previous) {
			return RecordRightsDecisionRepositoryDTO{}, fmt.Errorf("%w: superseded rights decision identity does not match", sharedrepository.ErrConstraint)
		}
	}
	return request, nil
}
