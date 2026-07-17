package application

import (
	"context"
	"errors"
	"fmt"

	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type JobStore interface {
	ListJobs(context.Context, operationsdomain.JobListQuery) (operationsdomain.JobPage, error)
	CancelJob(context.Context, int64) (operationsdomain.JobSummary, error)
	RetryJob(context.Context, int64) (operationsdomain.JobSummary, error)
}

type JobStoreWithHook interface {
	JobStore
	CancelJobWithHook(context.Context, int64, func(context.Context, operationsdomain.JobSummary) error) (operationsdomain.JobSummary, error)
	RetryJobWithHook(context.Context, int64, func(context.Context, operationsdomain.JobSummary) error) (operationsdomain.JobSummary, error)
}

type JobAuditWriter interface {
	Write(context.Context, operationsdomain.AuditEntry) error
}

type JobService struct {
	store JobStore
	audit JobAuditWriter
}

func NewJobService(store JobStore, audit JobAuditWriter) (*JobService, error) {
	if store == nil {
		return nil, fmt.Errorf("job store is required")
	}
	return &JobService{store: store, audit: audit}, nil
}

func (service *JobService) List(ctx context.Context, query operationsdomain.JobListQuery) (operationsdomain.JobPage, error) {
	if service == nil || service.store == nil {
		return operationsdomain.JobPage{}, sharedrepository.ErrUnavailable
	}
	if err := query.Validate(); err != nil {
		return operationsdomain.JobPage{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	return service.store.ListJobs(ctx, query)
}

func (service *JobService) Cancel(ctx context.Context, input operationsdomain.JobMutationInput) (operationsdomain.JobSummary, error) {
	return service.mutate(ctx, input, operationsdomain.ActionJobCancelled, false)
}

func (service *JobService) Retry(ctx context.Context, input operationsdomain.JobMutationInput) (operationsdomain.JobSummary, error) {
	return service.mutate(ctx, input, operationsdomain.ActionJobRetried, true)
}

func (service *JobService) mutate(ctx context.Context, input operationsdomain.JobMutationInput, action operationsdomain.AuditAction, retry bool) (operationsdomain.JobSummary, error) {
	if service == nil || service.store == nil {
		return operationsdomain.JobSummary{}, sharedrepository.ErrUnavailable
	}
	if err := input.Validate(); err != nil {
		return operationsdomain.JobSummary{}, fmt.Errorf("%w: %v", sharedrepository.ErrInvalidInput, err)
	}
	hook := func(hookCtx context.Context, job operationsdomain.JobSummary) error {
		if service.audit == nil {
			return nil
		}
		return service.audit.Write(hookCtx, operationsdomain.AuditEntry{ActorType: "user", ActorID: input.ActorID, Action: action, ResourceType: "river_job", ResourceID: job.ID, Result: operationsdomain.AuditResultSuccess})
	}
	if store, ok := service.store.(JobStoreWithHook); ok {
		if retry {
			return store.RetryJobWithHook(ctx, input.JobID, hook)
		}
		return store.CancelJobWithHook(ctx, input.JobID, hook)
	}
	var (
		job operationsdomain.JobSummary
		err error
	)
	if retry {
		job, err = service.store.RetryJob(ctx, input.JobID)
	} else {
		job, err = service.store.CancelJob(ctx, input.JobID)
	}
	if err != nil {
		return operationsdomain.JobSummary{}, err
	}
	if err := hook(ctx, job); err != nil {
		return operationsdomain.JobSummary{}, err
	}
	return job, nil
}

func JobHTTPError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) {
		return sharederrors.Wrap(sharederrors.CodeInvalidRequest, 400, "invalid job request", err)
	}
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return sharederrors.Wrap(sharederrors.CodeNotFound, 404, "job not found", err)
	}
	if errors.Is(err, sharedrepository.ErrConflict) {
		return sharederrors.Wrap(sharederrors.CodeConflict, 409, "job state conflict", err)
	}
	if errors.Is(err, sharedrepository.ErrUnavailable) {
		return sharederrors.Wrap(sharederrors.CodeUnavailable, 503, "job service unavailable", err)
	}
	return err
}
