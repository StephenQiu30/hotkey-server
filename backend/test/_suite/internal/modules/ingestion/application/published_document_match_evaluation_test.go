package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPublishedMatchEvaluationReadsExactTargetsAndEvaluatesDeterministically(t *testing.T) {
	t.Parallel()
	reader := &publishedMatchTargetReaderFake{result: PublishedMatchTargetsResult{Targets: []PublishedMatchTargetDTO{
		{MonitorID: 3, MonitorVersionID: 13, CompiledProfileID: 23, RelevanceProfileID: 33},
		{MonitorID: 4, MonitorVersionID: 14, CompiledProfileID: 24, RelevanceProfileID: 34},
	}}}
	evaluator := &publishedMatchEvaluatorFake{}
	service, err := NewPublishedMatchEvaluationService(reader, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.EvaluateForDocument(context.Background(), EvaluatePublishedMatchesForDocumentCommand{DocumentVersionID: 71})
	if err != nil {
		t.Fatalf("EvaluateForDocument() error = %v", err)
	}
	if reader.query.DocumentVersionID != 71 || result.DocumentVersionID != 71 || result.TargetCount != 2 || result.EvaluatedTargetCount != 2 {
		t.Fatalf("evaluation result/query = %#v / %#v", result, reader.query)
	}
	want := []EvaluatePublishedDocumentMatchesCommand{
		{MonitorID: 3, MonitorVersionID: 13, CompiledProfileID: 23, RelevanceProfileID: 33},
		{MonitorID: 4, MonitorVersionID: 14, CompiledProfileID: 24, RelevanceProfileID: 34},
	}
	if !reflect.DeepEqual(evaluator.commands, want) {
		t.Fatalf("evaluation commands = %#v, want %#v", evaluator.commands, want)
	}
	if reflect.TypeOf(PublishedMatchTargetDTO{}).NumField() != 4 {
		t.Fatalf("target DTO exposes more than four exact database identities")
	}
}

func TestPublishedMatchEvaluationFailsClosedOnDuplicateTargetOrPartialFailure(t *testing.T) {
	t.Parallel()
	t.Run("duplicate target", func(t *testing.T) {
		reader := &publishedMatchTargetReaderFake{result: PublishedMatchTargetsResult{Targets: []PublishedMatchTargetDTO{
			{MonitorID: 3, MonitorVersionID: 13, CompiledProfileID: 23, RelevanceProfileID: 33},
			{MonitorID: 3, MonitorVersionID: 13, CompiledProfileID: 23, RelevanceProfileID: 33},
		}}}
		evaluator := &publishedMatchEvaluatorFake{}
		service, _ := NewPublishedMatchEvaluationService(reader, evaluator)
		if _, err := service.EvaluateForDocument(context.Background(), EvaluatePublishedMatchesForDocumentCommand{DocumentVersionID: 71}); !errors.Is(err, ErrInvalidDocumentMatchContract) || len(evaluator.commands) != 0 {
			t.Fatalf("duplicate target error/calls = %v/%d", err, len(evaluator.commands))
		}
	})

	t.Run("later target fails", func(t *testing.T) {
		reader := &publishedMatchTargetReaderFake{result: PublishedMatchTargetsResult{Targets: []PublishedMatchTargetDTO{
			{MonitorID: 3, MonitorVersionID: 13, CompiledProfileID: 23, RelevanceProfileID: 33},
			{MonitorID: 4, MonitorVersionID: 14, CompiledProfileID: 24, RelevanceProfileID: 34},
		}}}
		evaluator := &publishedMatchEvaluatorFake{failAt: 2, err: errors.New("recall unavailable")}
		service, _ := NewPublishedMatchEvaluationService(reader, evaluator)
		result, err := service.EvaluateForDocument(context.Background(), EvaluatePublishedMatchesForDocumentCommand{DocumentVersionID: 71})
		if err == nil || result.TargetCount != 2 || result.EvaluatedTargetCount != 1 || len(evaluator.commands) != 2 {
			t.Fatalf("partial failure result/error/calls = %#v / %v / %d", result, err, len(evaluator.commands))
		}
	})
}

type publishedMatchTargetReaderFake struct {
	query  PublishedMatchTargetsQuery
	result PublishedMatchTargetsResult
	err    error
}

func (reader *publishedMatchTargetReaderFake) ReadPublishedMatchTargets(_ context.Context, query PublishedMatchTargetsQuery) (PublishedMatchTargetsResult, error) {
	reader.query = query
	return reader.result, reader.err
}

type publishedMatchEvaluatorFake struct {
	commands []EvaluatePublishedDocumentMatchesCommand
	failAt   int
	err      error
}

func (evaluator *publishedMatchEvaluatorFake) EvaluatePublished(_ context.Context, command EvaluatePublishedDocumentMatchesCommand) (EvaluatePublishedDocumentMatchesResult, error) {
	evaluator.commands = append(evaluator.commands, command)
	if evaluator.failAt == len(evaluator.commands) {
		return EvaluatePublishedDocumentMatchesResult{}, evaluator.err
	}
	return EvaluatePublishedDocumentMatchesResult{}, nil
}
