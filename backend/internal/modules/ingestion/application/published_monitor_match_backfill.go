package application

import (
	"context"
	"fmt"
)

type SchedulePublishedMonitorMatchBackfillCommand struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
}

type SchedulePublishedMonitorMatchBackfillResult struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
	JobID             int64
	Created           bool
}

type PublishedMonitorMatchBackfillScheduler interface {
	SchedulePublishedMonitorMatchBackfill(context.Context, SchedulePublishedMonitorMatchBackfillCommand) (SchedulePublishedMonitorMatchBackfillResult, error)
}

type ReadPublishedMonitorTriggerQuery struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
}

type ReadPublishedMonitorTriggerResult struct {
	Exists            bool
	DocumentVersionID int64
}

type PublishedMonitorDocumentReader interface {
	ReadPublishedMonitorTrigger(context.Context, ReadPublishedMonitorTriggerQuery) (ReadPublishedMonitorTriggerResult, error)
}

type BackfillPublishedMonitorMatchesCommand struct {
	MonitorID         int64
	MonitorVersionID  int64
	CompiledProfileID int64
}

type BackfillPublishedMonitorMatchesResult struct {
	MonitorID                int64
	MonitorVersionID         int64
	CompiledProfileID        int64
	TriggerDocumentVersionID int64
	Evaluated                bool
	AcceptedDecisionIDs      []int64
}

// PublishedMonitorMatchBackfillService finds one rights-safe trigger document
// for the exact immutable publication. The production evaluator then performs
// its bounded Hybrid Recall over the whole published monitor scope, so the
// backfill neither copies document text into a job nor reevaluates the same
// monitor once per historical document.
type PublishedMonitorMatchBackfillService struct {
	documents PublishedMonitorDocumentReader
	evaluator *PublishedMatchEvaluationService
}

func NewPublishedMonitorMatchBackfillService(documents PublishedMonitorDocumentReader, evaluator *PublishedMatchEvaluationService) (*PublishedMonitorMatchBackfillService, error) {
	if documents == nil || evaluator == nil {
		return nil, fmt.Errorf("%w: published monitor backfill dependencies are required", ErrInvalidDocumentMatchContract)
	}
	return &PublishedMonitorMatchBackfillService{documents: documents, evaluator: evaluator}, nil
}

func (service *PublishedMonitorMatchBackfillService) Backfill(ctx context.Context, command BackfillPublishedMonitorMatchesCommand) (BackfillPublishedMonitorMatchesResult, error) {
	result := BackfillPublishedMonitorMatchesResult{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID, AcceptedDecisionIDs: []int64{},
	}
	if service == nil || service.documents == nil || service.evaluator == nil || command.MonitorID <= 0 ||
		command.MonitorVersionID <= 0 || command.CompiledProfileID <= 0 {
		return result, ErrInvalidDocumentMatchContract
	}
	trigger, err := service.documents.ReadPublishedMonitorTrigger(ctx, ReadPublishedMonitorTriggerQuery{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID,
	})
	if err != nil {
		return result, fmt.Errorf("read published monitor backfill trigger: %w", err)
	}
	if !trigger.Exists {
		if trigger.DocumentVersionID != 0 {
			return result, ErrInvalidDocumentMatchContract
		}
		return result, nil
	}
	if trigger.DocumentVersionID <= 0 {
		return result, ErrInvalidDocumentMatchContract
	}
	evaluated, evaluateErr := service.evaluator.EvaluateForDocument(ctx, EvaluatePublishedMatchesForDocumentCommand{
		DocumentVersionID: trigger.DocumentVersionID, TriggerMonitorVersionID: command.MonitorVersionID,
	})
	if evaluateErr != nil {
		return result, fmt.Errorf("evaluate published monitor backfill: %w", evaluateErr)
	}
	if evaluated.TargetCount != 1 || evaluated.EvaluatedTargetCount != 1 {
		return result, ErrInvalidDocumentMatchContract
	}
	result.TriggerDocumentVersionID = trigger.DocumentVersionID
	result.Evaluated = true
	result.AcceptedDecisionIDs = append(result.AcceptedDecisionIDs, evaluated.AcceptedDecisionIDs...)
	return result, nil
}
