package jobs

import (
	"context"
	"reflect"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

func TestIntentPreviewEvaluatorCompilesAndRecallsExactVersionWithoutInventingRelevance(t *testing.T) {
	t.Parallel()
	task := monitorapplication.IntentAnalysisTaskDTO{
		Run: monitorapplication.IntentRunReferenceDTO{
			RunID: 91, Kind: "preview", MonitorID: 7, DraftID: 11, DraftResourceVersion: 3,
			InputHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}, AnalysisProfile: "hybrid-preview-v1", SampleLimit: 2,
	}
	preparer := &intentPreviewPreparerFake{result: monitorapplication.PrepareIntentPreviewResult{
		Preview: monitorapplication.PreparedIntentPreviewDTO{Task: task, Draft: intentExpansionDraftFixture()},
	}}
	compiler := &intentPreviewCompilerFake{result: monitorapplication.CompilePreviewIntentProfileResult{
		Profile: monitorapplication.CompiledIntentProfileDTO{
			CompiledProfileID: 701, MonitorID: 7, Purpose: "preview", ConfigVersionID: 301,
			PreviewRunID: 91, DraftID: 11, DraftResourceVersion: 3, IntentRevisionID: 401,
			ProfileHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}}
	semanticScore := .76
	recall := &intentHybridRecallFake{result: ingestionapplication.HybridRecallResult{
		MonitorID: 7, ConfigVersionID: 301, CompiledProfileID: 701, Purpose: "preview", PreviewRunID: 91,
		MatchingAlgorithmVersion: ingestionapplication.HybridRecallMatchingAlgorithmVersion,
		Candidates: []ingestionapplication.HybridRecallCandidateDTO{
			{DocumentVersionID: 41, RRFScore: .031, Signals: []ingestionapplication.RecallSignalDTO{{Channel: "lexical", Rank: 1, RawScore: 3.5, AlgorithmVersion: ingestionapplication.LexicalRecallAlgorithmVersion}}},
			{DocumentVersionID: 42, RRFScore: .030, SemanticScore: &semanticScore, Signals: []ingestionapplication.RecallSignalDTO{{Channel: "semantic", Rank: 2, RawScore: semanticScore, AlgorithmVersion: ingestionapplication.SemanticRecallAlgorithmVersion}}},
		},
		Degraded: true, DegradationReasons: []string{"semantic_recall_unavailable"},
	}}
	documents := &recallPreviewDocumentReaderFake{result: ingestionapplication.RecallPreviewDocumentResult{
		Documents: []ingestionapplication.RecallPreviewDocumentDTO{
			{DocumentVersionID: 41, Title: "HotKey launch interrupted", TitleAvailable: true},
			{DocumentVersionID: 42, TitleAvailable: false},
		},
	}}
	evaluator, err := newIntentPreviewEvaluator(preparer, compiler, recall, documents)
	if err != nil {
		t.Fatalf("newIntentPreviewEvaluator(): %v", err)
	}

	preview, err := evaluator.EvaluatePreview(context.Background(), task)
	if err != nil {
		t.Fatalf("EvaluatePreview(): %v", err)
	}
	if preview.EstimatedAlertCount != 0 || len(preview.Samples) != 2 {
		t.Fatalf("preview summary = %#v", preview)
	}
	for _, sample := range preview.Samples {
		if sample.Decision != "review" {
			t.Fatalf("uncalibrated candidate received automatic decision: %#v", sample)
		}
	}
	if preview.Samples[0].Title != "HotKey launch interrupted" || preview.Samples[1].Title != "标题不可用" {
		t.Fatalf("safe preview titles = %#v", preview.Samples)
	}
	if !reflect.DeepEqual(preview.Warnings, []string{"preview_title_unavailable", "preview_uncalibrated", "relevance_reranker_unavailable", "semantic_recall_unavailable", "structured_extraction_unavailable"}) {
		t.Fatalf("warnings = %#v", preview.Warnings)
	}
	if recall.query.Purpose != "preview" || recall.query.PreviewRunID != 91 || recall.query.ConfigVersionID != 301 || recall.query.CompiledProfileID != 701 {
		t.Fatalf("recall query = %#v", recall.query)
	}
	if !reflect.DeepEqual(documents.query.DocumentVersionIDs, []int64{41, 42}) {
		t.Fatalf("document projection query = %#v", documents.query)
	}
}

type intentPreviewPreparerFake struct {
	result monitorapplication.PrepareIntentPreviewResult
	err    error
}

func (fake *intentPreviewPreparerFake) PrepareIntentPreview(_ context.Context, _ monitorapplication.PrepareIntentPreviewQuery) (monitorapplication.PrepareIntentPreviewResult, error) {
	return fake.result, fake.err
}

type intentPreviewCompilerFake struct {
	result monitorapplication.CompilePreviewIntentProfileResult
	err    error
}

func (fake *intentPreviewCompilerFake) CompilePreview(_ context.Context, _ monitorapplication.CompilePreviewIntentProfileCommand) (monitorapplication.CompilePreviewIntentProfileResult, error) {
	return fake.result, fake.err
}

type intentHybridRecallFake struct {
	query  ingestionapplication.HybridRecallQuery
	result ingestionapplication.HybridRecallResult
	err    error
}

func (fake *intentHybridRecallFake) Recall(_ context.Context, query ingestionapplication.HybridRecallQuery) (ingestionapplication.HybridRecallResult, error) {
	fake.query = query
	return fake.result, fake.err
}

type recallPreviewDocumentReaderFake struct {
	query  ingestionapplication.RecallPreviewDocumentQuery
	result ingestionapplication.RecallPreviewDocumentResult
	err    error
}

func (fake *recallPreviewDocumentReaderFake) ReadRecallPreviewDocuments(_ context.Context, query ingestionapplication.RecallPreviewDocumentQuery) (ingestionapplication.RecallPreviewDocumentResult, error) {
	fake.query = query
	return fake.result, fake.err
}
