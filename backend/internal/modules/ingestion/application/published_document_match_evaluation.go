package application

import (
	"context"
	"fmt"
)

// SchedulePublishedDocumentMatchEvaluationCommand is intentionally limited to
// one immutable database identity. Queue adapters may add a request trace from
// context, but body text, recall facts, and object-store coordinates are never
// durable job input.
type SchedulePublishedDocumentMatchEvaluationCommand struct {
	DocumentVersionID int64
}

type SchedulePublishedDocumentMatchEvaluationResult struct {
	DocumentVersionID int64
	JobID             int64
	Created           bool
}

type PublishedDocumentMatchEvaluationScheduler interface {
	SchedulePublishedDocumentMatchEvaluation(context.Context, SchedulePublishedDocumentMatchEvaluationCommand) (SchedulePublishedDocumentMatchEvaluationResult, error)
}

type PublishedMatchTargetsQuery struct {
	DocumentVersionID       int64
	TriggerMonitorVersionID int64
}

type PublishedMatchTargetDTO struct {
	MonitorID, MonitorVersionID, CompiledProfileID, RelevanceProfileID int64
}

type PublishedMatchTargetsResult struct {
	Targets []PublishedMatchTargetDTO
}

type PublishedMatchTargetReader interface {
	ReadPublishedMatchTargets(context.Context, PublishedMatchTargetsQuery) (PublishedMatchTargetsResult, error)
}

type PublishedDocumentMatchEvaluator interface {
	EvaluatePublished(context.Context, EvaluatePublishedDocumentMatchesCommand) (EvaluatePublishedDocumentMatchesResult, error)
}

type EvaluatePublishedMatchesForDocumentCommand struct {
	DocumentVersionID       int64
	TriggerMonitorVersionID int64
}

type EvaluatePublishedMatchesForDocumentResult struct {
	DocumentVersionID    int64
	TargetCount          int
	EvaluatedTargetCount int
	AcceptedDecisionIDs  []int64
}

type ConsumeAcceptedDocumentMatchCommand struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
}

type ConsumeAcceptedDocumentMatchResult struct {
	DocumentMatchDecisionID int64
	DocumentVersionID       int64
}

type AcceptedDocumentMatchConsumer interface {
	ConsumeAcceptedDocumentMatch(context.Context, ConsumeAcceptedDocumentMatchCommand) (ConsumeAcceptedDocumentMatchResult, error)
}

// PublishedMatchEvaluationService converts one durable DocumentVersion trigger
// into exact immutable monitor/profile evaluations. The target reader prefers
// an active evaluation-gated profile, otherwise shadow and uncalibrated facts
// remain review-only under the same conservative decision service.
type PublishedMatchEvaluationService struct {
	targets   PublishedMatchTargetReader
	evaluator PublishedDocumentMatchEvaluator
	accepted  AcceptedDocumentMatchConsumer
}

func NewPublishedMatchEvaluationService(targets PublishedMatchTargetReader, evaluator PublishedDocumentMatchEvaluator, consumers ...AcceptedDocumentMatchConsumer) (*PublishedMatchEvaluationService, error) {
	if targets == nil || evaluator == nil {
		return nil, fmt.Errorf("%w: published match evaluation dependencies are required", ErrInvalidDocumentMatchContract)
	}
	service := &PublishedMatchEvaluationService{targets: targets, evaluator: evaluator}
	if len(consumers) > 1 || len(consumers) == 1 && consumers[0] == nil {
		return nil, fmt.Errorf("%w: accepted match consumer is invalid", ErrInvalidDocumentMatchContract)
	}
	if len(consumers) == 1 {
		service.accepted = consumers[0]
	}
	return service, nil
}

func (service *PublishedMatchEvaluationService) EvaluateForDocument(ctx context.Context, command EvaluatePublishedMatchesForDocumentCommand) (EvaluatePublishedMatchesForDocumentResult, error) {
	if service == nil || service.targets == nil || service.evaluator == nil || command.DocumentVersionID <= 0 {
		return EvaluatePublishedMatchesForDocumentResult{}, ErrInvalidDocumentMatchContract
	}
	if command.TriggerMonitorVersionID < 0 {
		return EvaluatePublishedMatchesForDocumentResult{}, ErrInvalidDocumentMatchContract
	}
	read, err := service.targets.ReadPublishedMatchTargets(ctx, PublishedMatchTargetsQuery{
		DocumentVersionID: command.DocumentVersionID, TriggerMonitorVersionID: command.TriggerMonitorVersionID,
	})
	if err != nil {
		return EvaluatePublishedMatchesForDocumentResult{DocumentVersionID: command.DocumentVersionID}, fmt.Errorf("read published match targets: %w", err)
	}
	result := EvaluatePublishedMatchesForDocumentResult{DocumentVersionID: command.DocumentVersionID, TargetCount: len(read.Targets)}
	seen := make(map[[4]int64]struct{}, len(read.Targets))
	var previousMonitorID int64
	for _, target := range read.Targets {
		identity := [4]int64{target.MonitorID, target.MonitorVersionID, target.CompiledProfileID, target.RelevanceProfileID}
		if target.MonitorID <= 0 || target.MonitorVersionID <= 0 || target.CompiledProfileID <= 0 || target.RelevanceProfileID <= 0 ||
			(previousMonitorID > 0 && target.MonitorID < previousMonitorID) {
			return result, ErrInvalidDocumentMatchContract
		}
		if _, duplicate := seen[identity]; duplicate {
			return result, ErrInvalidDocumentMatchContract
		}
		seen[identity] = struct{}{}
		previousMonitorID = target.MonitorID
	}
	for _, target := range read.Targets {
		evaluated, err := service.evaluator.EvaluatePublished(ctx, EvaluatePublishedDocumentMatchesCommand{
			MonitorID: target.MonitorID, MonitorVersionID: target.MonitorVersionID,
			CompiledProfileID: target.CompiledProfileID, RelevanceProfileID: target.RelevanceProfileID,
		})
		if err != nil {
			return result, fmt.Errorf("evaluate published match target: %w", err)
		}
		for _, decision := range evaluated.Decisions {
			if decision.Decision != "accepted" {
				continue
			}
			if service.accepted == nil {
				continue
			}
			consumed, consumeErr := service.accepted.ConsumeAcceptedDocumentMatch(ctx, ConsumeAcceptedDocumentMatchCommand{
				DocumentMatchDecisionID: decision.ID, DocumentVersionID: decision.DocumentVersionID})
			if consumeErr != nil {
				return result, fmt.Errorf("consume accepted document match: %w", consumeErr)
			}
			if consumed.DocumentMatchDecisionID != decision.ID || consumed.DocumentVersionID != decision.DocumentVersionID {
				return result, ErrInvalidDocumentMatchContract
			}
			result.AcceptedDecisionIDs = append(result.AcceptedDecisionIDs, decision.ID)
		}
		result.EvaluatedTargetCount++
	}
	return result, nil
}

func validatePublishedDocumentMatchScheduleReceipt(command SchedulePublishedDocumentMatchEvaluationCommand, result SchedulePublishedDocumentMatchEvaluationResult) error {
	if command.DocumentVersionID <= 0 || result.DocumentVersionID != command.DocumentVersionID || result.JobID <= 0 {
		return ErrInvalidDocumentMatchContract
	}
	return nil
}
