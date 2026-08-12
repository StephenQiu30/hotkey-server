package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

// QuotaGuard is reused by monitor and source application transactions. Its
// concrete PostgreSQL adapter automatically joins the caller transaction.
type QuotaGuard interface {
	CheckActiveMonitor(context.Context, int64) error
	RecordManualSearch(context.Context, int64, time.Time) error
}

type GovernanceStore interface {
	UsageOverview(context.Context, int64, time.Time) (operationsdomain.UsageOverview, error)
	ListAudit(context.Context, operationsdomain.AuditQuery) (operationsdomain.AuditPage, error)
}

type RetentionStore interface {
	List(context.Context) ([]operationsdomain.RetentionPolicy, error)
	Find(context.Context, int64) (operationsdomain.RetentionPolicy, error)
	Preview(context.Context, operationsdomain.RetentionPolicy, time.Time, int) (int64, bool, error)
	ApplyRetentionBatch(context.Context, operationsdomain.RetentionPolicy, time.Time, int) (int64, error)
}

type GovernanceService struct {
	runtime   *database.Runtime
	store     GovernanceStore
	retention RetentionStore
	audit     AuditWriter
	now       func() time.Time
}

type GovernanceDependencies struct {
	Runtime   *database.Runtime
	Store     GovernanceStore
	Retention RetentionStore
	Audit     AuditWriter
	Now       func() time.Time
}

func NewGovernanceService(dependencies GovernanceDependencies) (*GovernanceService, error) {
	if dependencies.Runtime == nil || dependencies.Store == nil || dependencies.Retention == nil || dependencies.Audit == nil {
		return nil, fmt.Errorf("governance dependencies are required")
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	return &GovernanceService{runtime: dependencies.Runtime, store: dependencies.Store, retention: dependencies.Retention, audit: dependencies.Audit, now: dependencies.Now}, nil
}

func (service *GovernanceService) Usage(ctx context.Context, subject identitydomain.Subject) (operationsdomain.UsageOverview, error) {
	if subject.UserID <= 0 || (subject.Role != identitydomain.RoleEditor && subject.Role != identitydomain.RoleAdmin) {
		return operationsdomain.UsageOverview{}, sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	return service.store.UsageOverview(ctx, subject.UserID, service.now().UTC())
}

func (service *GovernanceService) RetentionPolicies(ctx context.Context, subject identitydomain.Subject) ([]operationsdomain.RetentionPolicy, error) {
	if err := requireGovernanceAdmin(subject); err != nil {
		return nil, err
	}
	return service.retention.List(ctx)
}

type RetentionInput struct {
	Subject         identitydomain.Subject
	PolicyID        int64
	ExpectedVersion int64
	BatchSize       int
}

func (service *GovernanceService) PreviewRetention(ctx context.Context, input RetentionInput) (operationsdomain.CleanupResult, error) {
	if err := requireGovernanceAdmin(input.Subject); err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	policy, cutoff, err := service.retentionBoundary(ctx, input)
	if err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	affected, hasMore, err := service.retention.Preview(ctx, policy, cutoff, input.BatchSize)
	if err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	return operationsdomain.CleanupResult{DataClass: policy.DataClass, Cutoff: cutoff, Affected: affected, BatchSize: input.BatchSize, HasMore: hasMore, DryRun: true}, nil
}

func (service *GovernanceService) RunRetention(ctx context.Context, input RetentionInput) (operationsdomain.CleanupResult, error) {
	if err := requireGovernanceAdmin(input.Subject); err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	var result operationsdomain.CleanupResult
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		policy, cutoff, err := service.retentionBoundary(transactionCtx, input)
		if err != nil {
			return err
		}
		affected, err := service.retention.ApplyRetentionBatch(transactionCtx, policy, cutoff, input.BatchSize)
		if err != nil {
			return err
		}
		remaining, truncated, err := service.retention.Preview(transactionCtx, policy, cutoff, 1)
		if err != nil {
			return err
		}
		hasMore := remaining > 0 || truncated
		entry := operationsdomain.AuditEntry{
			ActorType: "user", ActorID: input.Subject.UserID, Action: operationsdomain.ActionRetentionExecuted,
			ResourceType: "retention_policy", ResourceID: policy.ID, Result: operationsdomain.AuditResultSuccess,
			RequestID: requestcontext.RequestID(transactionCtx), TraceID: requestcontext.TraceID(transactionCtx),
			After: map[string]any{"affected": affected, "batch_size": int64(input.BatchSize)},
		}
		if err := service.audit.Write(transactionCtx, entry); err != nil {
			return err
		}
		result = operationsdomain.CleanupResult{DataClass: policy.DataClass, Cutoff: cutoff, Affected: affected, BatchSize: input.BatchSize, HasMore: hasMore, DryRun: false}
		return nil
	})
	return result, err
}

func (service *GovernanceService) Audit(ctx context.Context, subject identitydomain.Subject, query operationsdomain.AuditQuery) (operationsdomain.AuditPage, error) {
	if err := requireGovernanceAdmin(subject); err != nil {
		return operationsdomain.AuditPage{}, err
	}
	return service.store.ListAudit(ctx, query)
}

func (service *GovernanceService) retentionBoundary(ctx context.Context, input RetentionInput) (operationsdomain.RetentionPolicy, time.Time, error) {
	if input.PolicyID <= 0 || input.ExpectedVersion <= 0 || input.BatchSize < 1 || input.BatchSize > 1000 {
		return operationsdomain.RetentionPolicy{}, time.Time{}, fmt.Errorf("%w: invalid retention input", sharedrepository.ErrInvalidInput)
	}
	policy, err := service.retention.Find(ctx, input.PolicyID)
	if err != nil {
		return operationsdomain.RetentionPolicy{}, time.Time{}, err
	}
	if policy.Version != input.ExpectedVersion {
		return operationsdomain.RetentionPolicy{}, time.Time{}, sharedrepository.ErrConflict
	}
	if policy.Protected {
		return operationsdomain.RetentionPolicy{}, time.Time{}, fmt.Errorf("%w: protected retention policy", sharedrepository.ErrInvalidInput)
	}
	return policy, service.now().UTC().AddDate(0, 0, -policy.RetentionDays), nil
}

func requireGovernanceAdmin(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || subject.Role != identitydomain.RoleAdmin {
		return sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
	}
	return nil
}

func GovernanceHTTPError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return err
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput):
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, http.StatusBadRequest, "invalid governance request", err)
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.Wrap(sharederrors.CodeNotFound, http.StatusNotFound, "retention policy not found", err)
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.Wrap(sharederrors.CodeConflict, http.StatusConflict, "governance state conflict", err)
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return sharederrors.Wrap(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "governance service unavailable", err)
	default:
		return err
	}
}
