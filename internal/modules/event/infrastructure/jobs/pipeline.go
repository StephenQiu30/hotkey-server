package jobs

import (
	"context"
	"fmt"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
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

type HeatHandler struct {
	service *eventapplication.HeatService
	jobs    *queue.Store
}

func NewHeatHandler(service *eventapplication.HeatService, jobs *queue.Store) (*HeatHandler, error) {
	if service == nil || jobs == nil {
		return nil, fmt.Errorf("heat handler dependencies are required")
	}
	return &HeatHandler{service: service, jobs: jobs}, nil
}

func (handler *HeatHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindRecomputeEventHeat); err != nil {
		return queue.NewPermanentError(err)
	}
	windowEnd := job.Payload.WindowEnd
	if windowEnd.IsZero() {
		windowEnd = time.Now().UTC()
	}
	if _, err := handler.service.RecomputeEventMetrics(ctx, eventapplication.MetricRecomputeCommand{EventID: job.Payload.EntityID, WindowEnd: windowEnd, HeatVersion: domain.HeatAlgorithmVersionV1}); err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	summaryHash := queue.StableJobHash(queue.KindGenerateEventSummary, fmt.Sprint(job.Payload.EntityID), fmt.Sprint(job.Payload.EntityVersion), job.Payload.InputHash)
	_, _, err := handler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindGenerateEventSummary,
		UniqueKey:   queue.StableJobKey(queue.KindGenerateEventSummary, job.Payload.EntityID, job.Payload.EntityVersion, summaryHash),
		Payload:     queue.Payload{EntityID: job.Payload.EntityID, EntityVersion: job.Payload.EntityVersion, WindowStart: job.Payload.WindowStart, WindowEnd: windowEnd, InputHash: summaryHash},
		ScheduledAt: job.ScheduledAt, MaxAttempts: 3, Priority: 6,
	})
	return queue.ClassifyHandlerError(ctx, err)
}
