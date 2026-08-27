package application

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type rightsManagementTransactionContextKey struct{}

type rightsManagementTransactionFake struct{ calls int }

func (runner *rightsManagementTransactionFake) WithinRightsManagementTransaction(ctx context.Context, operation func(context.Context) error) error {
	runner.calls++
	return operation(context.WithValue(ctx, rightsManagementTransactionContextKey{}, true))
}

type rightsActorAuthorizerFake struct {
	calls int
	last  RightsActorAuthorizationDTO
	err   error
}

func (authorizer *rightsActorAuthorizerFake) AuthorizeRightsMutation(_ context.Context, authorization RightsActorAuthorizationDTO) error {
	authorizer.calls++
	authorizer.last = authorization
	return authorizer.err
}

type rightsManagementAuditFake struct {
	events     []RightsManagementAuditDTO
	attempts   []RightsManagementAttemptAuditDTO
	err        error
	attemptErr error
}

func (audit *rightsManagementAuditFake) WriteRightsMutation(ctx context.Context, event RightsManagementAuditDTO) error {
	if allowed, _ := ctx.Value(rightsManagementTransactionContextKey{}).(bool); !allowed {
		return errors.New("rights audit ran outside transaction")
	}
	audit.events = append(audit.events, cloneRightsManagementAudit(event))
	return audit.err
}

func (audit *rightsManagementAuditFake) WriteRightsMutationAttempt(ctx context.Context, event RightsManagementAttemptAuditDTO) error {
	if insideTransaction, _ := ctx.Value(rightsManagementTransactionContextKey{}).(bool); insideTransaction {
		return errors.New("rights attempt audit ran inside rolled-back business transaction")
	}
	audit.attempts = append(audit.attempts, event)
	return audit.attemptErr
}

type rightsIdempotencyFixture struct {
	fingerprint string
	batchID     int64
	policy      RightsPolicyDTO
	decisions   []RightsDecisionDTO
}

type rightsManagementRepositoryFake struct {
	nextPolicyID   int64
	nextBatchID    int64
	nextDecisionID int64
	policies       map[int64]RightsPolicyDTO
	decisions      map[int64]RightsDecisionDTO
	idempotency    map[string]rightsIdempotencyFixture
	createCalls    int
	recordCalls    int
	lastCreate     CreateRightsPolicyRepositoryDTO
	lastRecord     RecordRightsDecisionRepositoryDTO
}

func newRightsManagementRepositoryFake() *rightsManagementRepositoryFake {
	return &rightsManagementRepositoryFake{
		nextPolicyID: 100, nextBatchID: 900, nextDecisionID: 1000,
		policies: make(map[int64]RightsPolicyDTO), decisions: make(map[int64]RightsDecisionDTO),
		idempotency: make(map[string]rightsIdempotencyFixture),
	}
}

func (repository *rightsManagementRepositoryFake) CreateRightsPolicy(ctx context.Context, request CreateRightsPolicyRepositoryDTO) (CreateRightsPolicyRepositoryResultDTO, error) {
	if allowed, _ := ctx.Value(rightsManagementTransactionContextKey{}).(bool); !allowed {
		return CreateRightsPolicyRepositoryResultDTO{}, errors.New("policy write ran outside transaction")
	}
	repository.createCalls++
	repository.lastCreate = cloneCreateRightsPolicyRepositoryDTO(request)
	key := "policy:" + request.IdempotencyKey
	if existing, found := repository.idempotency[key]; found {
		if existing.fingerprint != request.CommandFingerprint {
			return CreateRightsPolicyRepositoryResultDTO{}, sharedrepository.ErrConflict
		}
		return CreateRightsPolicyRepositoryResultDTO{Policy: existing.policy, IdempotentReplay: true}, nil
	}
	repository.nextPolicyID++
	policy := RightsPolicyDTO{
		ID: repository.nextPolicyID, Version: 1, SourceConnectionID: copyRightsInt64(request.SourceConnectionID),
		ScopeType: request.ScopeType, ScopeSubject: request.ScopeSubject, Revision: request.Revision,
		Priority: request.Priority, BasisSummary: request.BasisSummary, TermsURL: request.TermsURL,
		LicenseURI: request.LicenseURI, PolicyHash: request.PolicyHash,
		EffectiveFrom: request.EffectiveFrom, ExpiresAt: copyRightsTime(request.ExpiresAt),
		ParentPolicyID: copyRightsInt64(request.ParentPolicyID), ApprovedByUserID: copyRightsInt64(request.ApprovedByUserID),
	}
	repository.policies[policy.ID] = cloneRightsPolicyDTO(policy)
	repository.idempotency[key] = rightsIdempotencyFixture{fingerprint: request.CommandFingerprint, policy: cloneRightsPolicyDTO(policy)}
	return CreateRightsPolicyRepositoryResultDTO{Policy: policy}, nil
}

func (repository *rightsManagementRepositoryFake) FindRightsPolicy(_ context.Context, query FindRightsPolicyQueryDTO) (RightsPolicyDTO, error) {
	policy, found := repository.policies[query.PolicyID]
	if !found || policy.Version != query.ExpectedVersion {
		return RightsPolicyDTO{}, sharedrepository.ErrNotFound
	}
	return cloneRightsPolicyDTO(policy), nil
}

func (repository *rightsManagementRepositoryFake) FindRightsDecision(_ context.Context, decisionID int64) (RightsDecisionDTO, error) {
	decision, found := repository.decisions[decisionID]
	if !found {
		return RightsDecisionDTO{}, sharedrepository.ErrNotFound
	}
	return cloneRightsDecisionDTO(decision), nil
}

func (repository *rightsManagementRepositoryFake) RecordRightsDecisions(ctx context.Context, request RecordRightsDecisionRepositoryDTO) (RecordRightsDecisionRepositoryResultDTO, error) {
	if allowed, _ := ctx.Value(rightsManagementTransactionContextKey{}).(bool); !allowed {
		return RecordRightsDecisionRepositoryResultDTO{}, errors.New("decision write ran outside transaction")
	}
	repository.recordCalls++
	repository.lastRecord = cloneRecordRightsDecisionRepositoryDTO(request)
	key := "decision:" + request.IdempotencyKey
	if existing, found := repository.idempotency[key]; found {
		if existing.fingerprint != request.CommandFingerprint {
			return RecordRightsDecisionRepositoryResultDTO{}, sharedrepository.ErrConflict
		}
		return RecordRightsDecisionRepositoryResultDTO{
			DecisionBatchID: existing.batchID,
			Decisions:       cloneRightsDecisionDTOs(existing.decisions), IdempotentReplay: true,
		}, nil
	}
	repository.nextBatchID++
	batchID := repository.nextBatchID
	decisions := make([]RightsDecisionDTO, 0, len(request.Decisions))
	for _, candidate := range request.Decisions {
		repository.nextDecisionID++
		decision := RightsDecisionDTO{
			ID: repository.nextDecisionID, DecisionBatchID: batchID, SourceConnectionID: request.SourceConnectionID,
			PolicyID: request.Policy.ID, PolicyRevision: request.Policy.Revision,
			PolicyScopeType: request.Policy.ScopeType, PolicyScopeSubject: request.Policy.ScopeSubject,
			Priority: request.Policy.Priority, BasisSummary: request.Policy.BasisSummary,
			TermsURL: request.Policy.TermsURL, LicenseURI: request.Policy.LicenseURI,
			SubjectType: request.SubjectType, SubjectKey: request.SubjectKey, InputDigest: request.InputDigest,
			Action: candidate.Action, Decision: candidate.Decision, ReasonCodes: append([]string(nil), candidate.ReasonCodes...),
			Evaluator: candidate.Evaluator, EvaluatedAt: candidate.EvaluatedAt, EffectiveFrom: candidate.EffectiveFrom,
			ExpiresAt: copyRightsTime(candidate.ExpiresAt), RetentionDays: copyRightsInt(candidate.RetentionDays),
			SupersedesDecisionID: copyRightsInt64(candidate.SupersedesDecisionID),
		}
		repository.decisions[decision.ID] = cloneRightsDecisionDTO(decision)
		decisions = append(decisions, decision)
	}
	repository.idempotency[key] = rightsIdempotencyFixture{
		fingerprint: request.CommandFingerprint, batchID: batchID, decisions: cloneRightsDecisionDTOs(decisions),
	}
	return RecordRightsDecisionRepositoryResultDTO{DecisionBatchID: batchID, Decisions: decisions}, nil
}

func TestRightsManagementServiceCreatesImmutablePolicyIdempotentlyAndAuditsFirstWrite(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	authorizer := &rightsActorAuthorizerFake{}
	audit := &rightsManagementAuditFake{}
	transactions := &rightsManagementTransactionFake{}
	service := newRightsManagementServiceForTest(t, repository, authorizer, audit, transactions)
	command := validCreateRightsPolicyCommand()

	first, err := service.CreatePolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("CreatePolicy(first): %v", err)
	}
	second, err := service.CreatePolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("CreatePolicy(replay): %v", err)
	}
	if first.IdempotentReplay || !second.IdempotentReplay || first.Policy.ID != second.Policy.ID ||
		first.Policy.PolicyHash == "" || first.Policy.PolicyHash != repository.lastCreate.PolicyHash {
		t.Fatalf("idempotent policy results = first:%#v second:%#v", first, second)
	}
	if repository.lastCreate.EffectiveFrom.Nanosecond()%1000 != 0 {
		t.Fatalf("policy time was not canonicalized to PostgreSQL precision: %s", repository.lastCreate.EffectiveFrom)
	}
	if repository.createCalls != 2 || authorizer.calls != 2 || transactions.calls != 2 || len(audit.events) != 1 {
		t.Fatalf("calls = repository:%d authorizer:%d transactions:%d audits:%d", repository.createCalls, authorizer.calls, transactions.calls, len(audit.events))
	}
	if audit.events[0].Operation != RightsManagementCreatePolicy || audit.events[0].PolicyID != first.Policy.ID ||
		audit.events[0].ActorID != command.ActorID || audit.events[0].CommandFingerprint != repository.lastCreate.CommandFingerprint {
		t.Fatalf("policy audit = %#v", audit.events[0])
	}

	conflict := command
	conflict.BasisSummary = "different immutable policy input"
	if _, err := service.CreatePolicy(context.Background(), conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same idempotency key with different input error = %v", err)
	}
	if len(audit.attempts) != 1 || audit.attempts[0].Result != RightsManagementAttemptFailure ||
		audit.attempts[0].ReasonCode != RightsManagementReasonIdempotencyConflict || audit.attempts[0].PolicyID != 0 {
		t.Fatalf("policy conflict attempt audit = %#v", audit.attempts)
	}

	originalSourceID := *command.SourceConnectionID
	*command.SourceConnectionID = originalSourceID + 100
	if repository.lastCreate.SourceConnectionID == nil || *repository.lastCreate.SourceConnectionID != originalSourceID {
		t.Fatal("repository request retained a mutable caller pointer")
	}
}

func TestRightsManagementServiceRecordsExactSingleActionBatchAtomicallyAndIdempotently(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	policy := approvedRightsPolicyDTO()
	repository.policies[policy.ID] = policy
	previous := priorRightsDecisionDTO(policy)
	repository.decisions[previous.ID] = previous
	authorizer := &rightsActorAuthorizerFake{}
	audit := &rightsManagementAuditFake{}
	transactions := &rightsManagementTransactionFake{}
	service := newRightsManagementServiceForTest(t, repository, authorizer, audit, transactions)
	command := validRecordRightsDecisionCommand(policy, previous)

	first, err := service.RecordDecisions(context.Background(), command)
	if err != nil {
		t.Fatalf("RecordDecisions(first): %v", err)
	}
	if first.IdempotentReplay || first.DecisionBatchID <= 0 || len(first.Decisions) != 2 || repository.recordCalls != 1 || len(repository.lastRecord.Decisions) != 2 {
		t.Fatalf("atomic decision result = %#v calls=%d", first, repository.recordCalls)
	}
	if repository.lastRecord.ExpectedPolicyVersion != policy.Version || repository.lastRecord.Policy.Version != policy.Version ||
		repository.lastRecord.Policy.Revision != policy.Revision || policy.Version == policy.Revision {
		t.Fatalf("row version and policy revision were conflated: request=%#v policy=%#v", repository.lastRecord, policy)
	}
	for _, decision := range repository.lastRecord.Decisions {
		if decision.EvaluatedAt.Nanosecond()%1000 != 0 || decision.EffectiveFrom.Nanosecond()%1000 != 0 {
			t.Fatalf("decision time was not canonicalized to PostgreSQL precision: %#v", decision)
		}
	}
	for _, decision := range first.Decisions {
		if decision.DecisionBatchID != first.DecisionBatchID || decision.PolicyID != policy.ID ||
			decision.PolicyRevision != policy.Revision || decision.PolicyRevision == policy.Version ||
			decision.PolicyScopeType != policy.ScopeType || decision.PolicyScopeSubject != policy.ScopeSubject ||
			decision.Priority != policy.Priority || decision.BasisSummary != policy.BasisSummary ||
			decision.SourceConnectionID != command.SourceConnectionID || decision.SubjectKey != command.SubjectKey ||
			decision.InputDigest != command.InputDigest {
			t.Fatalf("decision did not use the immutable policy/subject snapshot: %#v", decision)
		}
	}
	if len(audit.events) != 1 || audit.events[0].DecisionBatchID != first.DecisionBatchID ||
		len(audit.events[0].DecisionIDs) != 2 || len(audit.events[0].Actions) != 2 {
		t.Fatalf("decision audit = %#v", audit.events)
	}

	reordered := command
	reordered.Decisions = []RightsActionDecisionDTO{command.Decisions[1], command.Decisions[0]}
	reordered.Decisions[1].ReasonCodes = []string{"terms_confirmed", "manual_review"}
	replayed, err := service.RecordDecisions(context.Background(), reordered)
	if err != nil {
		t.Fatalf("RecordDecisions(reordered replay): %v", err)
	}
	if !replayed.IdempotentReplay || replayed.DecisionBatchID != first.DecisionBatchID ||
		!reflect.DeepEqual(decisionIDs(first.Decisions), decisionIDs(replayed.Decisions)) || len(audit.events) != 1 {
		t.Fatalf("semantic replay diverged: first=%#v replay=%#v audits=%d", first, replayed, len(audit.events))
	}
	conflict := command
	conflict.Decisions = append([]RightsActionDecisionDTO(nil), command.Decisions...)
	conflict.Decisions[0].Evaluator = "different-evaluator"
	if _, err := service.RecordDecisions(context.Background(), conflict); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("same decision idempotency key with different input error = %v", err)
	}
	if len(audit.attempts) != 1 || audit.attempts[0].Result != RightsManagementAttemptFailure ||
		audit.attempts[0].ReasonCode != RightsManagementReasonIdempotencyConflict || audit.attempts[0].PolicyID != policy.ID {
		t.Fatalf("decision conflict attempt audit = %#v", audit.attempts)
	}

	command.Decisions[0].ReasonCodes[0] = "caller_mutation"
	for _, decision := range repository.lastRecord.Decisions {
		if len(decision.ReasonCodes) > 0 && decision.ReasonCodes[0] == "caller_mutation" {
			t.Fatal("repository request retained a mutable caller slice")
		}
	}
}

func TestRightsManagementServiceFailsClosedForUnapprovedAllowInvalidRetentionAndSupersedesMismatch(t *testing.T) {
	t.Parallel()
	t.Run("unapproved allow", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		policy.ApprovedByUserID = nil
		repository.policies[policy.ID] = policy
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
		command.Decisions = command.Decisions[:1]
		command.Decisions[0].SupersedesDecisionID = nil
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrConstraint) {
			t.Fatalf("unapproved allow error = %v", err)
		}
		if repository.recordCalls != 0 {
			t.Fatal("unapproved allow reached decision writer")
		}
	})

	t.Run("retention belongs only to retain", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		repository.policies[policy.ID] = policy
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
		command.Decisions = command.Decisions[:1]
		command.Decisions[0].SupersedesDecisionID = nil
		days := 30
		command.Decisions[0].RetentionDays = &days
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("non-retain duration error = %v", err)
		}
	})

	t.Run("retain requires a duration", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		repository.policies[policy.ID] = policy
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
		command.Decisions = command.Decisions[1:]
		command.Decisions[0].RetentionDays = nil
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("retain without duration error = %v", err)
		}
	})

	t.Run("source must match scoped policy", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		repository.policies[policy.ID] = policy
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
		command.SourceConnectionID++
		command.Decisions = command.Decisions[:1]
		command.Decisions[0].SupersedesDecisionID = nil
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrConstraint) {
			t.Fatalf("policy source mismatch error = %v", err)
		}
		if repository.recordCalls != 0 {
			t.Fatal("policy source mismatch reached decision writer")
		}
	})

	t.Run("endpoint subject binds concrete source and policy hash", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		repository.policies[policy.ID] = policy
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
		command.SubjectType = string(domain.RightsSubjectSourceEndpoint)
		command.SubjectKey = "42"
		command.InputDigest = strings.Repeat("f", 64)
		command.Decisions = command.Decisions[:1]
		command.Decisions[0].SupersedesDecisionID = nil
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrConstraint) {
			t.Fatalf("endpoint policy hash mismatch error = %v", err)
		}
		if repository.recordCalls != 0 {
			t.Fatal("invalid endpoint binding reached decision writer")
		}
	})

	t.Run("supersedes exact identity", func(t *testing.T) {
		repository := newRightsManagementRepositoryFake()
		policy := approvedRightsPolicyDTO()
		repository.policies[policy.ID] = policy
		previous := priorRightsDecisionDTO(policy)
		previous.SubjectKey = "different-subject"
		repository.decisions[previous.ID] = previous
		service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
		command := validRecordRightsDecisionCommand(policy, previous)
		command.Decisions = command.Decisions[:1]
		if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrConstraint) {
			t.Fatalf("supersedes mismatch error = %v", err)
		}
		if repository.recordCalls != 0 {
			t.Fatal("invalid supersedes chain reached decision writer")
		}
	})
}

func TestRightsManagementServiceRecordsUnknownWithoutUpgradingItToAllow(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	policy := approvedRightsPolicyDTO()
	policy.ApprovedByUserID = nil
	repository.policies[policy.ID] = policy
	service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, &rightsManagementAuditFake{}, &rightsManagementTransactionFake{})
	command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
	command.Decisions = command.Decisions[:1]
	command.Decisions[0].SupersedesDecisionID = nil
	command.Decisions[0].Decision = string(domain.RightsUnknown)

	result, err := service.RecordDecisions(context.Background(), command)
	if err != nil {
		t.Fatalf("RecordDecisions(unknown): %v", err)
	}
	if len(result.Decisions) != 1 || result.Decisions[0].Decision != string(domain.RightsUnknown) {
		t.Fatalf("unknown decision changed semantics: %#v", result)
	}
	entity, err := rightsDecisionEntity(result.Decisions[0])
	if err != nil {
		t.Fatalf("map unknown decision: %v", err)
	}
	if entity.Allows(command.Decisions[0].EffectiveFrom) {
		t.Fatal("unknown decision authorized an action")
	}
}

func TestRightsManagementServiceDoesNotBypassActorAuthorization(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	authorizer := &rightsActorAuthorizerFake{err: sharederrors.New(sharederrors.CodeForbidden, 403, "")}
	audit := &rightsManagementAuditFake{}
	transactions := &rightsManagementTransactionFake{}
	service := newRightsManagementServiceForTest(t, repository, authorizer, audit, transactions)

	if _, err := service.CreatePolicy(context.Background(), validCreateRightsPolicyCommand()); err == nil {
		t.Fatalf("authorization error = %v", err)
	}
	if repository.createCalls != 0 || transactions.calls != 0 || len(audit.events) != 0 || len(audit.attempts) != 1 {
		t.Fatalf("denied actor side effects = repository:%d transactions:%d success-audits:%d attempt-audits:%d", repository.createCalls, transactions.calls, len(audit.events), len(audit.attempts))
	}
	if attempt := audit.attempts[0]; attempt.Result != RightsManagementAttemptDenied ||
		attempt.ReasonCode != RightsManagementReasonAuthorizationDenied || attempt.ActorID != 7 || attempt.SourceConnectionID == nil || *attempt.SourceConnectionID != 42 {
		t.Fatalf("denied attempt audit = %#v", attempt)
	}
}

func TestRightsManagementServiceAuditsInvalidInputBeforeAnySideEffect(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	authorizer := &rightsActorAuthorizerFake{}
	audit := &rightsManagementAuditFake{}
	transactions := &rightsManagementTransactionFake{}
	service := newRightsManagementServiceForTest(t, repository, authorizer, audit, transactions)
	command := validCreateRightsPolicyCommand()
	command.Priority = 999

	if _, err := service.CreatePolicy(context.Background(), command); err == nil {
		t.Fatal("invalid policy input succeeded")
	}
	if repository.createCalls != 0 || authorizer.calls != 0 || transactions.calls != 0 || len(audit.events) != 0 || len(audit.attempts) != 1 {
		t.Fatalf("invalid input side effects = repository:%d authorization:%d transactions:%d success-audits:%d attempt-audits:%d", repository.createCalls, authorizer.calls, transactions.calls, len(audit.events), len(audit.attempts))
	}
	if attempt := audit.attempts[0]; attempt.Result != RightsManagementAttemptFailure ||
		attempt.ReasonCode != RightsManagementReasonInvalidInput || attempt.IdempotencyKey != "" || attempt.CommandFingerprint != "" {
		t.Fatalf("invalid input attempt audit = %#v", attempt)
	}
}

func TestRightsManagementServiceAuditsStalePolicyVersionWithoutDecisionWrite(t *testing.T) {
	t.Parallel()
	repository := newRightsManagementRepositoryFake()
	policy := approvedRightsPolicyDTO()
	repository.policies[policy.ID] = policy
	audit := &rightsManagementAuditFake{}
	service := newRightsManagementServiceForTest(t, repository, &rightsActorAuthorizerFake{}, audit, &rightsManagementTransactionFake{})
	command := validRecordRightsDecisionCommand(policy, RightsDecisionDTO{})
	command.ExpectedPolicyVersion = policy.Version + 1

	if _, err := service.RecordDecisions(context.Background(), command); !errors.Is(err, sharedrepository.ErrNotFound) {
		t.Fatalf("stale policy version error = %v", err)
	}
	if repository.recordCalls != 0 || len(audit.events) != 0 || len(audit.attempts) != 1 {
		t.Fatalf("stale version side effects = writes:%d success-audits:%d attempt-audits:%d", repository.recordCalls, len(audit.events), len(audit.attempts))
	}
	if attempt := audit.attempts[0]; attempt.Result != RightsManagementAttemptFailure ||
		attempt.ReasonCode != RightsManagementReasonVersionConflict || attempt.PolicyID != policy.ID {
		t.Fatalf("stale version attempt audit = %#v", attempt)
	}
}

func newRightsManagementServiceForTest(t *testing.T, repository RightsManagementRepository, authorizer RightsActorAuthorizer, audit RightsManagementAuditWriter, transactions RightsManagementTransactionRunner) *RightsManagementService {
	t.Helper()
	service, err := NewRightsManagementService(RightsManagementDependencies{
		Repository: repository, Authorizer: authorizer, Audit: audit, Transactions: transactions,
	})
	if err != nil {
		t.Fatalf("NewRightsManagementService(): %v", err)
	}
	return service
}

func validCreateRightsPolicyCommand() CreateRightsPolicyCommand {
	actorID, sourceID := int64(7), int64(42)
	now := time.Date(2026, 8, 9, 10, 0, 0, 123456789, time.UTC)
	return CreateRightsPolicyCommand{
		ActorID: actorID, IdempotencyKey: "rights-policy-create-1", SourceConnectionID: &sourceID,
		ScopeType: string(domain.RightsScopeSourceEndpoint), ScopeSubject: "feed-main", Revision: 1,
		Priority: int(domain.RightsPriorityEndpointContract), BasisSummary: "approved feed terms",
		TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:feed-main",
		EffectiveFrom: now, ApprovedByUserID: &actorID,
	}
}

func approvedRightsPolicyDTO() RightsPolicyDTO {
	actorID, sourceID := int64(7), int64(42)
	return RightsPolicyDTO{
		ID: 71, Version: 1, SourceConnectionID: &sourceID,
		ScopeType: string(domain.RightsScopeSourceEndpoint), ScopeSubject: "feed-main", Revision: 7,
		Priority: int(domain.RightsPriorityEndpointContract), BasisSummary: "approved feed terms",
		TermsURL: "https://publisher.example.test/terms", LicenseURI: "urn:license:feed-main",
		PolicyHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EffectiveFrom: time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC), ApprovedByUserID: &actorID,
	}
}

func priorRightsDecisionDTO(policy RightsPolicyDTO) RightsDecisionDTO {
	return RightsDecisionDTO{
		ID: 501, DecisionBatchID: 90, SourceConnectionID: 42, PolicyID: policy.ID, PolicyRevision: policy.Revision,
		PolicyScopeType: policy.ScopeType, PolicyScopeSubject: policy.ScopeSubject,
		Priority: policy.Priority, BasisSummary: policy.BasisSummary, TermsURL: policy.TermsURL, LicenseURI: policy.LicenseURI,
		SubjectType: string(domain.RightsSubjectRawResponse), SubjectKey: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		InputDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Action:      string(domain.RightsActionStoreRaw), Decision: string(domain.RightsAllow),
		ReasonCodes: []string{"previous_terms"}, Evaluator: "rights-admin",
		EvaluatedAt: time.Date(2026, 8, 9, 9, 30, 0, 0, time.UTC), EffectiveFrom: time.Date(2026, 8, 9, 9, 30, 0, 0, time.UTC),
	}
}

func validRecordRightsDecisionCommand(policy RightsPolicyDTO, previous RightsDecisionDTO) RecordRightsDecisionCommand {
	now := time.Date(2026, 8, 9, 10, 30, 0, 987654321, time.UTC)
	days := 30
	var supersedes *int64
	if previous.ID > 0 {
		supersedes = &previous.ID
	}
	return RecordRightsDecisionCommand{
		ActorID: 7, IdempotencyKey: "rights-decision-batch-1", SourceConnectionID: 42,
		PolicyID: policy.ID, ExpectedPolicyVersion: policy.Version,
		SubjectType: string(domain.RightsSubjectRawResponse),
		SubjectKey:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		InputDigest: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Decisions: []RightsActionDecisionDTO{
			{Action: string(domain.RightsActionStoreRaw), Decision: string(domain.RightsAllow),
				ReasonCodes: []string{"manual_review", "terms_confirmed"}, Evaluator: "rights-admin",
				EvaluatedAt: now, EffectiveFrom: now, SupersedesDecisionID: supersedes},
			{Action: string(domain.RightsActionRetain), Decision: string(domain.RightsAllow),
				ReasonCodes: []string{"retention_approved"}, Evaluator: "rights-admin",
				EvaluatedAt: now, EffectiveFrom: now, RetentionDays: &days},
		},
	}
}

func decisionIDs(values []RightsDecisionDTO) []int64 {
	result := make([]int64, len(values))
	for index, value := range values {
		result[index] = value.ID
	}
	return result
}

func cloneRightsManagementAudit(value RightsManagementAuditDTO) RightsManagementAuditDTO {
	result := value
	result.SourceConnectionID = copyRightsInt64(value.SourceConnectionID)
	result.DecisionIDs = append([]int64(nil), value.DecisionIDs...)
	result.Actions = append([]string(nil), value.Actions...)
	return result
}

func cloneCreateRightsPolicyRepositoryDTO(value CreateRightsPolicyRepositoryDTO) CreateRightsPolicyRepositoryDTO {
	result := value
	result.SourceConnectionID = copyRightsInt64(value.SourceConnectionID)
	result.ExpiresAt = copyRightsTime(value.ExpiresAt)
	result.ParentPolicyID = copyRightsInt64(value.ParentPolicyID)
	result.ApprovedByUserID = copyRightsInt64(value.ApprovedByUserID)
	return result
}

func cloneRightsPolicyDTO(value RightsPolicyDTO) RightsPolicyDTO {
	result := value
	result.SourceConnectionID = copyRightsInt64(value.SourceConnectionID)
	result.ExpiresAt = copyRightsTime(value.ExpiresAt)
	result.ParentPolicyID = copyRightsInt64(value.ParentPolicyID)
	result.ApprovedByUserID = copyRightsInt64(value.ApprovedByUserID)
	return result
}

func cloneRecordRightsDecisionRepositoryDTO(value RecordRightsDecisionRepositoryDTO) RecordRightsDecisionRepositoryDTO {
	result := value
	result.Policy = cloneRightsPolicyDTO(value.Policy)
	result.Decisions = append([]RightsActionDecisionDTO(nil), value.Decisions...)
	for index := range result.Decisions {
		result.Decisions[index].ReasonCodes = append([]string(nil), value.Decisions[index].ReasonCodes...)
		result.Decisions[index].ExpiresAt = copyRightsTime(value.Decisions[index].ExpiresAt)
		result.Decisions[index].RetentionDays = copyRightsInt(value.Decisions[index].RetentionDays)
		result.Decisions[index].SupersedesDecisionID = copyRightsInt64(value.Decisions[index].SupersedesDecisionID)
	}
	return result
}

func cloneRightsDecisionDTO(value RightsDecisionDTO) RightsDecisionDTO {
	result := value
	result.ReasonCodes = append([]string(nil), value.ReasonCodes...)
	result.ExpiresAt = copyRightsTime(value.ExpiresAt)
	result.RetentionDays = copyRightsInt(value.RetentionDays)
	result.SupersedesDecisionID = copyRightsInt64(value.SupersedesDecisionID)
	return result
}

func cloneRightsDecisionDTOs(values []RightsDecisionDTO) []RightsDecisionDTO {
	result := make([]RightsDecisionDTO, len(values))
	for index, value := range values {
		result[index] = cloneRightsDecisionDTO(value)
	}
	return result
}

func copyRightsInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyRightsInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyRightsTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
