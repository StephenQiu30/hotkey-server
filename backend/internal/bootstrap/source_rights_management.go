package bootstrap

import (
	"context"
	"errors"
	"fmt"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	identitypostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/infrastructure/postgres"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type rightsActorReader interface {
	FindByID(context.Context, int64) (*identitydomain.User, error)
}

// rightsActorAuthorizer resolves durable user state for every mutation. Caller
// supplied role strings never participate in a rights decision.
type rightsActorAuthorizer struct {
	users rightsActorReader
}

var _ sourceapplication.RightsActorAuthorizer = (*rightsActorAuthorizer)(nil)
var _ sourceapplication.RightsReadAuthorizer = (*rightsActorAuthorizer)(nil)

func newRightsActorAuthorizer(users *identitypostgres.UserRepository) sourceapplication.RightsActorAuthorizer {
	return &rightsActorAuthorizer{users: users}
}

func (authorizer *rightsActorAuthorizer) AuthorizeRightsMutation(ctx context.Context, authorization sourceapplication.RightsActorAuthorizationDTO) error {
	if authorizer == nil || authorizer.users == nil {
		return sharedrepository.ErrUnavailable
	}
	if authorization.ActorID <= 0 {
		return fmt.Errorf("%w: rights actor identity is invalid", sharedrepository.ErrInvalidInput)
	}
	switch authorization.Operation {
	case sourceapplication.RightsManagementCreatePolicy:
		if authorization.SourceConnectionID != nil && *authorization.SourceConnectionID <= 0 {
			return fmt.Errorf("%w: rights policy source identity is invalid", sharedrepository.ErrInvalidInput)
		}
	case sourceapplication.RightsManagementRecordDecisions:
		if authorization.SourceConnectionID == nil || *authorization.SourceConnectionID <= 0 || authorization.ApprovedByUserID != nil {
			return fmt.Errorf("%w: rights decision authorization is invalid", sharedrepository.ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: rights management operation is invalid", sharedrepository.ErrInvalidInput)
	}

	actor, err := authorizer.users.FindByID(ctx, authorization.ActorID)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return rightsManagementForbidden()
		}
		return err
	}
	if !activeRightsAdministrator(actor) {
		return rightsManagementForbidden()
	}
	if authorization.ApprovedByUserID == nil || *authorization.ApprovedByUserID == authorization.ActorID {
		return nil
	}
	approver, err := authorizer.users.FindByID(ctx, *authorization.ApprovedByUserID)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return rightsManagementForbidden()
		}
		return err
	}
	if !activeRightsAdministrator(approver) {
		return rightsManagementForbidden()
	}
	return nil
}

// AuthorizeRightsRead re-resolves durable user state for every Application
// query. Transport role middleware is defense in depth, never the source of
// truth for rights administration reads.
func (authorizer *rightsActorAuthorizer) AuthorizeRightsRead(ctx context.Context, authorization sourceapplication.RightsReadAuthorizationDTO) error {
	if authorizer == nil || authorizer.users == nil {
		return sharedrepository.ErrUnavailable
	}
	if authorization.ActorID <= 0 || authorization.SourceEndpointID <= 0 {
		return fmt.Errorf("%w: rights read identity is invalid", sharedrepository.ErrInvalidInput)
	}
	switch authorization.Operation {
	case sourceapplication.RightsReadSourceEndpointCapability,
		sourceapplication.RightsReadPolicyHistory,
		sourceapplication.RightsReadDecisionHistory,
		sourceapplication.RightsReadExactActionMatrix:
	default:
		return fmt.Errorf("%w: rights read operation is invalid", sharedrepository.ErrInvalidInput)
	}
	actor, err := authorizer.users.FindByID(ctx, authorization.ActorID)
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return rightsManagementForbidden()
		}
		return err
	}
	if actor == nil || actor.ID <= 0 || !actor.Active() || !actor.Role.Valid() {
		return rightsManagementForbidden()
	}
	if authorization.Operation == sourceapplication.RightsReadSourceEndpointCapability || actor.Role == identitydomain.RoleAdmin {
		return nil
	}
	return rightsManagementForbidden()
}

func activeRightsAdministrator(user *identitydomain.User) bool {
	return user != nil && user.ID > 0 && user.Active() && user.Role == identitydomain.RoleAdmin
}

func rightsManagementForbidden() error {
	return sharederrors.New(sharederrors.CodeForbidden, 403, "")
}

type rightsManagementAuditAdapter struct {
	writer operationsapplication.AuditWriter
}

type independentOperationsAuditWriter interface {
	WriteIndependent(context.Context, operationsdomain.AuditEntry) error
}

var _ sourceapplication.RightsManagementAuditWriter = (*rightsManagementAuditAdapter)(nil)

func newRightsManagementAuditWriter(writer *operationspostgres.AuditWriter) sourceapplication.RightsManagementAuditWriter {
	return &rightsManagementAuditAdapter{writer: writer}
}

func (adapter *rightsManagementAuditAdapter) WriteRightsMutation(ctx context.Context, event sourceapplication.RightsManagementAuditDTO) error {
	if adapter == nil || adapter.writer == nil {
		return sharedrepository.ErrUnavailable
	}
	entry, err := rightsManagementAuditEntry(ctx, event)
	if err != nil {
		return err
	}
	return adapter.writer.Write(ctx, entry)
}

func (adapter *rightsManagementAuditAdapter) WriteRightsMutationAttempt(ctx context.Context, event sourceapplication.RightsManagementAttemptAuditDTO) error {
	if adapter == nil || adapter.writer == nil {
		return sharedrepository.ErrUnavailable
	}
	independent, ok := adapter.writer.(independentOperationsAuditWriter)
	if !ok {
		return sharedrepository.ErrUnavailable
	}
	entry, err := rightsManagementAttemptAuditEntry(ctx, event)
	if err != nil {
		return err
	}
	return independent.WriteIndependent(ctx, entry)
}

func rightsManagementAttemptAuditEntry(ctx context.Context, event sourceapplication.RightsManagementAttemptAuditDTO) (operationsdomain.AuditEntry, error) {
	if event.ActorID <= 0 || event.SourceConnectionID != nil && *event.SourceConnectionID <= 0 ||
		event.IdempotencyKey != "" || event.CommandFingerprint != "" {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights attempt audit identity or receipt is invalid", sharedrepository.ErrInvalidInput)
	}
	result := operationsdomain.AuditResult(event.Result)
	if result != operationsdomain.AuditResultFailure && result != operationsdomain.AuditResultDenied {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights attempt audit result is invalid", sharedrepository.ErrInvalidInput)
	}
	allowedReasons := map[string]struct{}{
		sourceapplication.RightsManagementReasonInvalidInput:          {},
		sourceapplication.RightsManagementReasonAuthorizationDenied:   {},
		sourceapplication.RightsManagementReasonIdempotencyConflict:   {},
		sourceapplication.RightsManagementReasonVersionConflict:       {},
		sourceapplication.RightsManagementReasonDependencyUnavailable: {},
	}
	if _, allowed := allowedReasons[event.ReasonCode]; !allowed {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights attempt audit reason is invalid", sharedrepository.ErrInvalidInput)
	}
	entry := operationsdomain.AuditEntry{
		ActorType: "user", ActorID: event.ActorID,
		RequestID: sharedrequestcontext.RequestID(ctx), TraceID: sharedrequestcontext.TraceID(ctx),
		Result: result, After: map[string]any{"reason_code": event.ReasonCode},
	}
	switch event.Operation {
	case sourceapplication.RightsManagementCreatePolicy:
		if event.PolicyID != 0 {
			return operationsdomain.AuditEntry{}, fmt.Errorf("%w: failed policy creation cannot claim a policy identity", sharedrepository.ErrInvalidInput)
		}
		entry.Action = operationsdomain.ActionRightsPolicyCreated
		entry.ResourceType = "rights_policy"
	case sourceapplication.RightsManagementRecordDecisions:
		entry.Action = operationsdomain.ActionRightsDecisionBatchRecorded
		entry.ResourceType = "rights_decision_batch"
	default:
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights attempt audit operation is invalid", sharedrepository.ErrInvalidInput)
	}
	if err := entry.Validate(); err != nil {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	return entry, nil
}

func rightsManagementAuditEntry(ctx context.Context, event sourceapplication.RightsManagementAuditDTO) (operationsdomain.AuditEntry, error) {
	if event.ActorID <= 0 || event.PolicyID <= 0 || event.SourceConnectionID != nil && *event.SourceConnectionID <= 0 {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights audit identity is invalid", sharedrepository.ErrInvalidInput)
	}
	entry := operationsdomain.AuditEntry{
		ActorType: "user", ActorID: event.ActorID,
		RequestID: sharedrequestcontext.RequestID(ctx), TraceID: sharedrequestcontext.TraceID(ctx),
		IdempotencyKey: event.IdempotencyKey, CommandFingerprint: event.CommandFingerprint,
		Result: operationsdomain.AuditResultSuccess,
	}
	switch event.Operation {
	case sourceapplication.RightsManagementCreatePolicy:
		if event.DecisionBatchID != 0 || len(event.DecisionIDs) != 0 || len(event.Actions) != 0 {
			return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights policy audit contains decision facts", sharedrepository.ErrInvalidInput)
		}
		entry.Action = operationsdomain.ActionRightsPolicyCreated
		entry.ResourceType = "rights_policy"
		entry.ResourceID = event.PolicyID
	case sourceapplication.RightsManagementRecordDecisions:
		if event.SourceConnectionID == nil || event.DecisionBatchID <= 0 || len(event.DecisionIDs) == 0 || len(event.DecisionIDs) > 9 || len(event.DecisionIDs) != len(event.Actions) {
			return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights decision audit receipt is invalid", sharedrepository.ErrInvalidInput)
		}
		seenIDs := make(map[int64]struct{}, len(event.DecisionIDs))
		seenActions := make(map[sourcedomain.RightsAction]struct{}, len(event.Actions))
		for index, decisionID := range event.DecisionIDs {
			action := sourcedomain.RightsAction(event.Actions[index])
			if decisionID <= 0 || !action.Valid() {
				return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights decision audit fact is invalid", sharedrepository.ErrInvalidInput)
			}
			if _, duplicate := seenIDs[decisionID]; duplicate {
				return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights decision audit identity is duplicated", sharedrepository.ErrInvalidInput)
			}
			if _, duplicate := seenActions[action]; duplicate {
				return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights decision audit action is duplicated", sharedrepository.ErrInvalidInput)
			}
			seenIDs[decisionID] = struct{}{}
			seenActions[action] = struct{}{}
		}
		entry.Action = operationsdomain.ActionRightsDecisionBatchRecorded
		entry.ResourceType = "rights_decision_batch"
		entry.ResourceID = event.DecisionBatchID
		entry.After = map[string]any{"decision_count": len(event.DecisionIDs)}
	default:
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: rights audit operation is invalid", sharedrepository.ErrInvalidInput)
	}
	if err := entry.Validate(); err != nil {
		return operationsdomain.AuditEntry{}, fmt.Errorf("%w: %w", sharedrepository.ErrInvalidInput, err)
	}
	return entry, nil
}

func newRightsManagementService(
	repository *sourcepostgres.RightsManagementRepository,
	authorizer sourceapplication.RightsActorAuthorizer,
	audit sourceapplication.RightsManagementAuditWriter,
	transactions *sourcepostgres.RightsManagementTransactionAdapter,
) (*sourceapplication.RightsManagementService, error) {
	return sourceapplication.NewRightsManagementService(sourceapplication.RightsManagementDependencies{
		Repository: repository, Authorizer: authorizer, Audit: audit, Transactions: transactions,
	})
}
