package application

import (
	"context"
	"testing"
)

func TestPublishedMonitorMatchBackfillUsesOneExactTriggerForWholeRecall(t *testing.T) {
	reader := &publishedMonitorTriggerReaderFake{result: ReadPublishedMonitorTriggerResult{Exists: true, DocumentVersionID: 41}}
	targets := &publishedBackfillTargetReaderFake{result: PublishedMatchTargetsResult{Targets: []PublishedMatchTargetDTO{{
		MonitorID: 7, MonitorVersionID: 31, CompiledProfileID: 51, RelevanceProfileID: 61,
	}}}}
	evaluator := &publishedBackfillEvaluatorFake{result: EvaluatePublishedDocumentMatchesResult{Decisions: []DocumentMatchDecisionDTO{{
		ID: 71, DocumentVersionID: 41, Decision: "accepted",
	}}}}
	accepted := &publishedBackfillAcceptedConsumerFake{}
	evaluationService, err := NewPublishedMatchEvaluationService(targets, evaluator, accepted)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPublishedMonitorMatchBackfillService(reader, evaluationService)
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Backfill(context.Background(), BackfillPublishedMonitorMatchesCommand{
		MonitorID: 7, MonitorVersionID: 31, CompiledProfileID: 51,
	})
	if err != nil {
		t.Fatalf("Backfill(): %v", err)
	}
	if !result.Evaluated || result.TriggerDocumentVersionID != 41 || len(result.AcceptedDecisionIDs) != 1 || result.AcceptedDecisionIDs[0] != 71 {
		t.Fatalf("result = %#v", result)
	}
	if reader.query != (ReadPublishedMonitorTriggerQuery{MonitorID: 7, MonitorVersionID: 31, CompiledProfileID: 51}) {
		t.Fatalf("trigger query = %#v", reader.query)
	}
	if targets.query != (PublishedMatchTargetsQuery{DocumentVersionID: 41, TriggerMonitorVersionID: 31}) {
		t.Fatalf("target query = %#v", targets.query)
	}
	if evaluator.calls != 1 || accepted.command != (ConsumeAcceptedDocumentMatchCommand{DocumentMatchDecisionID: 71, DocumentVersionID: 41}) {
		t.Fatalf("evaluator calls=%d accepted=%#v", evaluator.calls, accepted.command)
	}
}

func TestPublishedMonitorMatchBackfillSafelyCompletesWhenNoEligibleDocumentExists(t *testing.T) {
	reader := &publishedMonitorTriggerReaderFake{result: ReadPublishedMonitorTriggerResult{}}
	targets := &publishedBackfillTargetReaderFake{}
	evaluator := &publishedBackfillEvaluatorFake{}
	evaluationService, err := NewPublishedMatchEvaluationService(targets, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewPublishedMonitorMatchBackfillService(reader, evaluationService)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Backfill(context.Background(), BackfillPublishedMonitorMatchesCommand{MonitorID: 7, MonitorVersionID: 31, CompiledProfileID: 51})
	if err != nil || result.Evaluated || evaluator.calls != 0 {
		t.Fatalf("result/error/calls = %#v / %v / %d", result, err, evaluator.calls)
	}
}

type publishedMonitorTriggerReaderFake struct {
	query  ReadPublishedMonitorTriggerQuery
	result ReadPublishedMonitorTriggerResult
	err    error
}

func (reader *publishedMonitorTriggerReaderFake) ReadPublishedMonitorTrigger(_ context.Context, query ReadPublishedMonitorTriggerQuery) (ReadPublishedMonitorTriggerResult, error) {
	reader.query = query
	return reader.result, reader.err
}

type publishedBackfillTargetReaderFake struct {
	query  PublishedMatchTargetsQuery
	result PublishedMatchTargetsResult
	err    error
}

func (reader *publishedBackfillTargetReaderFake) ReadPublishedMatchTargets(_ context.Context, query PublishedMatchTargetsQuery) (PublishedMatchTargetsResult, error) {
	reader.query = query
	return reader.result, reader.err
}

type publishedBackfillEvaluatorFake struct {
	command EvaluatePublishedDocumentMatchesCommand
	result  EvaluatePublishedDocumentMatchesResult
	err     error
	calls   int
}

func (evaluator *publishedBackfillEvaluatorFake) EvaluatePublished(_ context.Context, command EvaluatePublishedDocumentMatchesCommand) (EvaluatePublishedDocumentMatchesResult, error) {
	evaluator.calls++
	evaluator.command = command
	return evaluator.result, evaluator.err
}

type publishedBackfillAcceptedConsumerFake struct {
	command ConsumeAcceptedDocumentMatchCommand
}

func (consumer *publishedBackfillAcceptedConsumerFake) ConsumeAcceptedDocumentMatch(_ context.Context, command ConsumeAcceptedDocumentMatchCommand) (ConsumeAcceptedDocumentMatchResult, error) {
	consumer.command = command
	return ConsumeAcceptedDocumentMatchResult(command), nil
}
