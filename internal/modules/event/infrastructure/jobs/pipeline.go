package jobs

import (
	"context"
	"fmt"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
)

type ClusterHandler struct {
	service *eventapplication.ClusteringExecutionService
	jobs    *queue.Store
}

func NewClusterHandler(service *eventapplication.ClusteringExecutionService, jobs *queue.Store) (*ClusterHandler, error) {
	if service == nil || jobs == nil {
		return nil, fmt.Errorf("cluster handler dependencies are required")
	}
	return &ClusterHandler{service: service, jobs: jobs}, nil
}

func (handler *ClusterHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindClusterContent); err != nil {
		return queue.NewPermanentError(err)
	}
	result, err := handler.service.Execute(ctx, eventapplication.ClusteringExecutionInput{
		ContentID: job.Payload.EntityID, ClusteringVersion: "clustering-v1", FeatureInputHash: job.Payload.InputHash,
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if result.Event == nil {
		return nil
	}
	event := result.Event
	heatHash := queue.StableJobHash(queue.KindRecomputeEventHeat, fmt.Sprint(event.ID), fmt.Sprint(event.Version), job.Payload.InputHash)
	_, _, err = handler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindRecomputeEventHeat,
		UniqueKey:   queue.StableJobKey(queue.KindRecomputeEventHeat, event.ID, event.Version, heatHash),
		Payload:     queue.Payload{EntityID: event.ID, EntityVersion: event.Version, WindowStart: job.Payload.WindowStart, WindowEnd: job.Payload.WindowEnd, InputHash: heatHash},
		ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 5,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

type HeatRecomputer interface {
	RecomputeEventMetrics(context.Context, eventapplication.MetricRecomputeCommand) ([]domain.HeatResult, error)
}

type UpdateRecorder interface {
	Record(context.Context, domain.HeatResult) (*domain.EventUpdate, bool, error)
}

type JobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type HeatHandler struct {
	service HeatRecomputer
	updates UpdateRecorder
	jobs    JobEnqueuer
}

// NewHeatHandler accepts both the existing (heat, jobs) construction and the
// PLAN-025 (heat, updates, jobs) construction. The former keeps startup
// compatibility while bootstrap rolls forward to the deterministic fan-out.
func NewHeatHandler(service HeatRecomputer, dependency any, additionalJobs ...JobEnqueuer) (*HeatHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("heat handler dependencies are required")
	}
	if len(additionalJobs) == 0 {
		jobs, ok := dependency.(JobEnqueuer)
		if !ok || jobs == nil {
			return nil, fmt.Errorf("heat handler dependencies are required")
		}
		return &HeatHandler{service: service, jobs: jobs}, nil
	}
	if len(additionalJobs) != 1 || additionalJobs[0] == nil {
		return nil, fmt.Errorf("heat handler dependencies are required")
	}
	updates, ok := dependency.(UpdateRecorder)
	if !ok || updates == nil {
		return nil, fmt.Errorf("heat handler dependencies are required")
	}
	return NewHeatHandlerWithUpdates(service, updates, additionalJobs[0])
}

// NewHeatHandlerWithUpdates is the typed DI constructor for the PLAN-025
// worker graph. NewHeatHandler remains the source-compatible call boundary.
func NewHeatHandlerWithUpdates(service HeatRecomputer, updates UpdateRecorder, jobs JobEnqueuer) (*HeatHandler, error) {
	if service == nil || updates == nil || jobs == nil {
		return nil, fmt.Errorf("heat handler dependencies are required")
	}
	return &HeatHandler{service: service, updates: updates, jobs: jobs}, nil
}

func (handler *HeatHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindRecomputeEventHeat); err != nil {
		return queue.NewPermanentError(err)
	}
	windowEnd := job.Payload.WindowEnd
	if windowEnd.IsZero() {
		windowEnd = time.Now().UTC()
	}
	if handler == nil || handler.service == nil || handler.jobs == nil {
		return queue.NewRetryableError(sharedrepository.ErrUnavailable)
	}
	results, err := handler.service.RecomputeEventMetrics(ctx, eventapplication.MetricRecomputeCommand{EventID: job.Payload.EntityID, WindowEnd: windowEnd, HeatVersion: domain.HeatAlgorithmVersionV1})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if handler.updates != nil {
		current, found := current24HourHeat(results)
		if !found {
			return queue.NewPermanentError(fmt.Errorf("%w: recompute did not return a 24 hour snapshot", sharedrepository.ErrInvalidInput))
		}
		update, _, err := handler.updates.Record(ctx, current)
		if err != nil {
			return queue.ClassifyHandlerError(ctx, err)
		}
		if update != nil && update.Kind.Actionable() {
			alertInputHash := update.EvidenceSetHash
			if alertInputHash == "" {
				// Keep the narrow handler contract tolerant of older callers while
				// persisted EventUpdates always provide the evidence-set hash.
				alertInputHash = update.IdempotencyKey
			}
			_, _, err := handler.jobs.Enqueue(ctx, queue.Job{
				Kind:        queue.KindEvaluateEventAlerts,
				UniqueKey:   queue.StableJobKey(queue.KindEvaluateEventAlerts, update.ID, update.Version, alertInputHash),
				Payload:     queue.Payload{EntityID: update.ID, EntityVersion: update.Version, InputHash: alertInputHash},
				ScheduledAt: job.ScheduledAt.UTC(), MaxAttempts: 5, Priority: 3,
			})
			if err != nil {
				return queue.ClassifyHandlerError(ctx, err)
			}
		}
	}
	summaryHash := queue.StableJobHash(queue.KindGenerateEventSummary, fmt.Sprint(job.Payload.EntityID), fmt.Sprint(job.Payload.EntityVersion), job.Payload.InputHash)
	_, _, err = handler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindGenerateEventSummary,
		UniqueKey:   queue.StableJobKey(queue.KindGenerateEventSummary, job.Payload.EntityID, job.Payload.EntityVersion, summaryHash),
		Payload:     queue.Payload{EntityID: job.Payload.EntityID, EntityVersion: job.Payload.EntityVersion, WindowStart: job.Payload.WindowStart, WindowEnd: windowEnd, InputHash: summaryHash},
		ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 6,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

func current24HourHeat(results []domain.HeatResult) (domain.HeatResult, bool) {
	for _, result := range results {
		if result.WindowHours == 24 {
			return result, true
		}
	}
	return domain.HeatResult{}, false
}
