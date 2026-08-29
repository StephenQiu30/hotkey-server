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
	CreateRun(context.Context, operationsdomain.RetentionPolicy, time.Time, int, int64, time.Time) (operationsdomain.RetentionRun, error)
	FindRun(context.Context, int64) (operationsdomain.RetentionRun, error)
	ApproveRun(context.Context, int64, string, int64, time.Time) (operationsdomain.RetentionRun, error)
	ExecuteApprovedRun(context.Context, int64, string, int64, time.Time) (operationsdomain.RetentionRun, error)
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
	var result operationsdomain.CleanupResult
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		policy, cutoff, err := service.retentionBoundary(transactionCtx, input)
		if err != nil {
			return err
		}
		run, err := service.retention.CreateRun(transactionCtx, policy, cutoff, input.BatchSize, input.Subject.UserID, service.now().UTC())
		if err != nil {
			return err
		}
		if err := service.writeRetentionAudit(transactionCtx, input.Subject.UserID, run, operationsdomain.ActionRetentionPreviewed); err != nil {
			return err
		}
		result = cleanupResult(run, true)
		return nil
	})
	return result, err
}

type RetentionRunInput struct {
	Subject       identitydomain.Subject
	RunID         int64
	CandidateHash string
}

func (service *GovernanceService) ApproveRetention(ctx context.Context, input RetentionRunInput) (operationsdomain.CleanupResult, error) {
	if err := requireGovernanceAdmin(input.Subject); err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	var result operationsdomain.CleanupResult
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		run, err := service.retention.ApproveRun(transactionCtx, input.RunID, input.CandidateHash, input.Subject.UserID, service.now().UTC())
		if err != nil {
			return err
		}
		if err := service.writeRetentionAudit(transactionCtx, input.Subject.UserID, run, operationsdomain.ActionRetentionApproved); err != nil {
			return err
		}
		result = cleanupResult(run, true)
		return nil
	})
	return result, err
}

func (service *GovernanceService) ExecuteRetention(ctx context.Context, input RetentionRunInput) (operationsdomain.CleanupResult, error) {
	if err := requireGovernanceAdmin(input.Subject); err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	var result operationsdomain.CleanupResult
	blocked := false
	err := service.runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		run, err := service.retention.ExecuteApprovedRun(transactionCtx, input.RunID, input.CandidateHash, input.Subject.UserID, service.now().UTC())
		if err != nil {
			return err
		}
		blocked = run.Status == operationsdomain.RetentionRunBlocked
		action := operationsdomain.ActionRetentionExecuted
		if blocked {
			action = operationsdomain.ActionRetentionBlocked
		}
		if err := service.writeRetentionAudit(transactionCtx, input.Subject.UserID, run, action); err != nil {
			return err
		}
		result = cleanupResult(run, false)
		return nil
	})
	if err != nil {
		return operationsdomain.CleanupResult{}, err
	}
	if blocked {
		return operationsdomain.CleanupResult{}, sharedrepository.ErrConflict
	}
	return result, nil
}

func (service *GovernanceService) writeRetentionAudit(ctx context.Context, actorID int64, run operationsdomain.RetentionRun, action operationsdomain.AuditAction) error {
	after := map[string]any{"batch_size": int64(run.BatchSize), "candidate_count": run.CandidateCount, "policy_version": run.PolicyVersion, "candidate_hash": run.CandidateHash}
	switch action {
	case operationsdomain.ActionRetentionPreviewed:
		after["approval_status"] = "pending"
	case operationsdomain.ActionRetentionApproved:
		after["approval_status"] = "approved"
	case operationsdomain.ActionRetentionBlocked:
		after["reason_code"] = run.FailureCode
	case operationsdomain.ActionRetentionExecuted:
		after["affected"] = run.Affected
	}
	return service.audit.Write(ctx, operationsdomain.AuditEntry{
		ActorType: "user", ActorID: actorID, Action: action,
		ResourceType: "retention_run", ResourceID: run.ID, Result: operationsdomain.AuditResultSuccess,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), After: after,
	})
}

func cleanupResult(run operationsdomain.RetentionRun, dryRun bool) operationsdomain.CleanupResult {
	affected := run.Affected
	if dryRun {
		affected = run.CandidateCount
	}
	return operationsdomain.CleanupResult{
		RunID: run.ID, PolicyVersion: run.PolicyVersion, DataClass: run.DataClass, Cutoff: run.Cutoff,
		Affected: affected, BatchSize: run.BatchSize, HasMore: run.HasMore, CandidateHash: run.CandidateHash,
		Status: run.Status, FailureCode: run.FailureCode, DryRun: dryRun,
	}
}

func (service *GovernanceService) Audit(ctx context.Context, subject identitydomain.Subject, query operationsdomain.AuditQuery) (operationsdomain.AuditPage, error) {
	if err := requireGovernanceAdmin(subject); err != nil {
		return operationsdomain.AuditPage{}, err
	}
	query.SubjectUserID = subject.UserID
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
