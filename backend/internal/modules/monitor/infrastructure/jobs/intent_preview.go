package jobs

import (
	"context"
	"fmt"
	"sort"
	"strings"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
)

const unavailablePreviewTitle = "标题不可用"

type intentPreviewPreparer interface {
	PrepareIntentPreview(context.Context, monitorapplication.PrepareIntentPreviewQuery) (monitorapplication.PrepareIntentPreviewResult, error)
}

type intentPreviewCompiler interface {
	CompilePreview(context.Context, monitorapplication.CompilePreviewIntentProfileCommand) (monitorapplication.CompilePreviewIntentProfileResult, error)
}

type intentHybridRecall interface {
	Recall(context.Context, ingestionapplication.HybridRecallQuery) (ingestionapplication.HybridRecallResult, error)
}

type IntentPreviewEvaluator struct {
	preparer  intentPreviewPreparer
	compiler  intentPreviewCompiler
	recall    intentHybridRecall
	documents ingestionapplication.RecallPreviewDocumentReader
}

func NewIntentPreviewEvaluator(intents *monitorapplication.IntentService, compiler *monitorapplication.IntentCompiler, recall *ingestionapplication.HybridRecallService, documents ingestionapplication.RecallPreviewDocumentReader) (*IntentPreviewEvaluator, error) {
	return newIntentPreviewEvaluator(intents, compiler, recall, documents)
}

func newIntentPreviewEvaluator(preparer intentPreviewPreparer, compiler intentPreviewCompiler, recall intentHybridRecall, documents ingestionapplication.RecallPreviewDocumentReader) (*IntentPreviewEvaluator, error) {
	if preparer == nil || compiler == nil || recall == nil || documents == nil {
		return nil, ErrIntentPreviewProcessorUnavailable
	}
	return &IntentPreviewEvaluator{preparer: preparer, compiler: compiler, recall: recall, documents: documents}, nil
}

func (evaluator *IntentPreviewEvaluator) EvaluatePreview(ctx context.Context, task monitorapplication.IntentAnalysisTaskDTO) (monitorapplication.IntentPreviewDTO, error) {
	if evaluator == nil || evaluator.preparer == nil || evaluator.compiler == nil || evaluator.recall == nil || evaluator.documents == nil || task.Run.Kind != "preview" {
		return monitorapplication.IntentPreviewDTO{}, ErrIntentPreviewProcessorUnavailable
	}
	prepared, err := evaluator.preparer.PrepareIntentPreview(ctx, monitorapplication.PrepareIntentPreviewQuery{Task: task})
	if err != nil {
		return monitorapplication.IntentPreviewDTO{}, err
	}
	if !sameIntentPreviewTask(prepared.Preview.Task, task) {
		return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: prepared preview identity differs", ErrIntentPreviewProcessorUnavailable)
	}
	compiled, err := evaluator.compiler.CompilePreview(ctx, monitorapplication.CompilePreviewIntentProfileCommand{Preview: prepared.Preview})
	if err != nil {
		return monitorapplication.IntentPreviewDTO{}, err
	}
	profile := compiled.Profile
	if profile.CompiledProfileID <= 0 || profile.MonitorID != task.Run.MonitorID || profile.Purpose != "preview" ||
		profile.ConfigVersionID <= 0 || profile.PreviewRunID != task.Run.RunID || profile.DraftID != task.Run.DraftID ||
		profile.DraftResourceVersion != task.Run.DraftResourceVersion {
		return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: compiled preview identity differs", ErrIntentPreviewProcessorUnavailable)
	}
	recalled, err := evaluator.recall.Recall(ctx, ingestionapplication.HybridRecallQuery{
		MonitorID: task.Run.MonitorID, Purpose: "preview", ConfigVersionID: profile.ConfigVersionID,
		PreviewRunID: task.Run.RunID, DraftID: task.Run.DraftID, DraftResourceVersion: task.Run.DraftResourceVersion,
		CompiledProfileID: profile.CompiledProfileID,
	})
	if err != nil {
		return monitorapplication.IntentPreviewDTO{}, err
	}
	if recalled.MonitorID != task.Run.MonitorID || recalled.Purpose != "preview" || recalled.ConfigVersionID != profile.ConfigVersionID ||
		recalled.PreviewRunID != task.Run.RunID || recalled.CompiledProfileID != profile.CompiledProfileID {
		return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: recall result identity differs", ErrIntentPreviewProcessorUnavailable)
	}
	candidates := recalled.Candidates
	if len(candidates) > task.SampleLimit {
		candidates = candidates[:task.SampleLimit]
	}
	ids := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.DocumentVersionID <= 0 {
			return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: recall candidate identity is invalid", ErrIntentPreviewProcessorUnavailable)
		}
		ids = append(ids, candidate.DocumentVersionID)
	}
	projected, err := evaluator.documents.ReadRecallPreviewDocuments(ctx, ingestionapplication.RecallPreviewDocumentQuery{DocumentVersionIDs: ids})
	if err != nil {
		return monitorapplication.IntentPreviewDTO{}, err
	}
	if len(projected.Documents) != len(candidates) {
		return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: preview document projection is incomplete", ErrIntentPreviewProcessorUnavailable)
	}
	warnings := append([]string{"preview_uncalibrated", "relevance_reranker_unavailable", "structured_extraction_unavailable"}, recalled.DegradationReasons...)
	preview := monitorapplication.IntentPreviewDTO{Samples: make([]monitorapplication.PreviewSampleDTO, 0, len(candidates))}
	for index, candidate := range candidates {
		document := projected.Documents[index]
		if document.DocumentVersionID != candidate.DocumentVersionID {
			return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: preview document order differs", ErrIntentPreviewProcessorUnavailable)
		}
		title := strings.TrimSpace(document.Title)
		if !document.TitleAvailable || title == "" {
			title = unavailablePreviewTitle
			warnings = append(warnings, "preview_title_unavailable")
		}
		signals := make([]monitorapplication.PreviewRecallSignalDTO, 0, len(candidate.Signals))
		reasons := []string{"hybrid_rrf_candidate"}
		for _, signal := range candidate.Signals {
			if signal.Channel == "" || signal.Rank <= 0 {
				return monitorapplication.IntentPreviewDTO{}, fmt.Errorf("%w: recall signal is invalid", ErrIntentPreviewProcessorUnavailable)
			}
			signals = append(signals, monitorapplication.PreviewRecallSignalDTO{Channel: signal.Channel, Rank: signal.Rank, Score: signal.RawScore})
			reasons = append(reasons, "recall_channel:"+signal.Channel)
		}
		preview.Samples = append(preview.Samples, monitorapplication.PreviewSampleDTO{
			DocumentVersionID: candidate.DocumentVersionID, Title: title, Decision: "review",
			RecallSignals: signals, Reasons: sortedUniquePreviewFacts(reasons), ExclusionReasons: []string{},
		})
	}
	if len(preview.Samples) == 0 {
		warnings = append(warnings, "no_historical_candidates")
	}
	preview.Warnings = sortedUniquePreviewFacts(warnings)
	return preview, nil
}

func sameIntentPreviewTask(left, right monitorapplication.IntentAnalysisTaskDTO) bool {
	return left.Run == right.Run && left.AnalysisProfile == right.AnalysisProfile && left.SampleLimit == right.SampleLimit
}

func sortedUniquePreviewFacts(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]byte(value)) > 4000 {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
