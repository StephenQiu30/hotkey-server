package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestIntentAnalysisHandlerRunsExactBoundedExpansionLifecycle(t *testing.T) {
	t.Parallel()
	controller := &intentRunControllerFake{task: intentAnalysisTaskFixture(31, "expansion")}
	processor := &intentAnalysisProcessorFake{candidates: []monitorapplication.ExpansionCandidateDTO{{
		ID: "launch-outage", Value: "launch outage", Source: "llm", Reason: "semantic neighbor",
		ModelVersion: "model-v1", PromptVersion: "prompt-v1", InputHash: strings.Repeat("a", 64),
		Similarity: 0.8, Risk: "low", ApprovalStatus: "pending",
	}}}
	handler, err := newIntentAnalysisHandler(controller, processor)
	if err != nil {
		t.Fatal(err)
	}
	job := intentAnalysisJob(t, IntentAnalysisJobArgs{
		RunID: 31, DraftID: 11, DraftResourceVersion: 4,
	})
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle(): %v", err)
	}
	if controller.starts != 1 || controller.expansionCompletions != 1 || controller.failures != 0 || processor.expansions != 1 {
		t.Fatalf("lifecycle calls = start:%d expansion:%d fail:%d processor:%d", controller.starts, controller.expansionCompletions, controller.failures, processor.expansions)
	}
	if controller.reference.RunID != 31 || controller.reference.DraftID != 11 || controller.reference.DraftResourceVersion != 4 || controller.reference.InputHash != strings.Repeat("a", 64) {
		t.Fatalf("exact run reference = %#v", controller.reference)
	}
	if controller.candidates[0].PromptVersion != "prompt-v1" || controller.candidates[0].Reason == "" {
		t.Fatalf("candidate provenance = %#v", controller.candidates[0])
	}
}

func TestIntentAnalysisHandlerPersistsSafeFailureWithoutLeakingProcessorError(t *testing.T) {
	t.Parallel()
	controller := &intentRunControllerFake{task: intentAnalysisTaskFixture(32, "preview")}
	processor := &intentAnalysisProcessorFake{err: errors.New("provider prompt contained SECRET objective and body")}
	handler, _ := newIntentAnalysisHandler(controller, processor)
	job := intentAnalysisJob(t, IntentAnalysisJobArgs{
		RunID: 32, DraftID: 11, DraftResourceVersion: 4,
	})
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("handled processor failure should be terminal after durable fail: %v", err)
	}
	if controller.failures != 1 || controller.failureReason != IntentAnalysisFailureReasonPreview || processor.previews != 1 {
		t.Fatalf("failure calls/reason = %d/%q processor=%d", controller.failures, controller.failureReason, processor.previews)
	}
	for _, forbidden := range []string{"SECRET", "objective", "body", "prompt"} {
		if strings.Contains(controller.failureReason, forbidden) {
			t.Fatalf("durable failure leaked %q: %q", forbidden, controller.failureReason)
		}
	}
}

func TestIntentAnalysisHandlerRejectsUnsafeArgsBeforeLifecycle(t *testing.T) {
	t.Parallel()
	controller := &intentRunControllerFake{task: intentAnalysisTaskFixture(31, "expansion")}
	processor := &intentAnalysisProcessorFake{}
	handler, _ := newIntentAnalysisHandler(controller, processor)
	job := queue.Job{
		Kind: queue.KindAnalyzeMonitorIntent, UniqueKey: "intent-unsafe",
		DurableArgs: []byte(`{"run_id":31,"draft_id":11,"draft_resource_version":4,"body":"secret"}`),
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}
	if err := handler.Handle(context.Background(), job); !queue.IsPermanent(err) || controller.starts != 0 || processor.expansions != 0 {
		t.Fatalf("unsafe args error/calls = %v/%d/%d", err, controller.starts, processor.expansions)
	}
}

func TestIntentAnalysisHandlerRequiresRealProcessorAndSkipsTerminalRedelivery(t *testing.T) {
	t.Parallel()
	if _, err := newIntentAnalysisHandler(&intentRunControllerFake{}, nil); err == nil {
		t.Fatal("nil processor must prevent production handler construction")
	}
	controller := &intentRunControllerFake{terminal: true, task: intentAnalysisTaskFixture(33, "preview")}
	processor := &intentAnalysisProcessorFake{}
	handler, _ := newIntentAnalysisHandler(controller, processor)
	job := intentAnalysisJob(t, IntentAnalysisJobArgs{
		RunID: 33, DraftID: 11, DraftResourceVersion: 4,
	})
	if err := handler.Handle(context.Background(), job); err != nil || processor.previews != 0 || controller.previewCompletions != 0 {
		t.Fatalf("terminal redelivery = %v processor:%d completion:%d", err, processor.previews, controller.previewCompletions)
	}
}

func intentAnalysisJob(t *testing.T, args IntentAnalysisJobArgs) queue.Job {
	t.Helper()
	encoded, err := EncodeIntentAnalysisJobArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	return queue.Job{
		Kind: queue.KindAnalyzeMonitorIntent, UniqueKey: "intent-analysis-test", DurableArgs: encoded,
		ScheduledAt: time.Now().UTC(), MaxAttempts: 3, Priority: 3,
	}
}

type intentRunControllerFake struct {
	task                 monitorapplication.IntentAnalysisTaskDTO
	reference            monitorapplication.IntentRunReferenceDTO
	candidates           []monitorapplication.ExpansionCandidateDTO
	failureReason        string
	starts               int
	failures             int
	expansionCompletions int
	previewCompletions   int
	terminal             bool
}

func (controller *intentRunControllerFake) ReadIntentAnalysisTask(_ context.Context, query monitorapplication.ReadIntentAnalysisTaskQuery) (monitorapplication.ReadIntentAnalysisTaskResult, error) {
	if controller.task.Run.RunID != query.RunID || controller.task.Run.DraftID != query.DraftID || controller.task.Run.DraftResourceVersion != query.DraftResourceVersion {
		return monitorapplication.ReadIntentAnalysisTaskResult{}, monitorapplication.ErrIntentRunNotFound
	}
	return monitorapplication.ReadIntentAnalysisTaskResult{Task: controller.task}, nil
}

func (controller *intentRunControllerFake) StartIntentRun(_ context.Context, command monitorapplication.StartIntentRunCommand) (monitorapplication.StartIntentRunResult, error) {
	controller.starts++
	controller.reference = command.Run
	status := "running"
	if controller.terminal {
		status = "failed"
	}
	return monitorapplication.StartIntentRunResult{Run: monitorapplication.IntentRunDTO{Status: status}, Reused: controller.terminal}, nil
}

func (controller *intentRunControllerFake) FailIntentRun(_ context.Context, command monitorapplication.FailIntentRunCommand) (monitorapplication.FailIntentRunResult, error) {
	controller.failures++
	controller.failureReason = command.Reason
	return monitorapplication.FailIntentRunResult{}, nil
}

func (controller *intentRunControllerFake) CompleteExpansionRun(_ context.Context, command monitorapplication.CompleteExpansionRunCommand) (monitorapplication.CompleteExpansionRunResult, error) {
	controller.expansionCompletions++
	controller.candidates = command.Candidates
	return monitorapplication.CompleteExpansionRunResult{}, nil
}

func (controller *intentRunControllerFake) CompletePreviewRun(_ context.Context, _ monitorapplication.CompletePreviewRunCommand) (monitorapplication.CompletePreviewRunResult, error) {
	controller.previewCompletions++
	return monitorapplication.CompletePreviewRunResult{}, nil
}

type intentAnalysisProcessorFake struct {
	candidates []monitorapplication.ExpansionCandidateDTO
	preview    monitorapplication.IntentPreviewDTO
	err        error
	expansions int
	previews   int
}

func (processor *intentAnalysisProcessorFake) GenerateExpansion(_ context.Context, _ monitorapplication.IntentAnalysisTaskDTO) ([]monitorapplication.ExpansionCandidateDTO, error) {
	processor.expansions++
	return processor.candidates, processor.err
}

func (processor *intentAnalysisProcessorFake) EvaluatePreview(_ context.Context, _ monitorapplication.IntentAnalysisTaskDTO) (monitorapplication.IntentPreviewDTO, error) {
	processor.previews++
	return processor.preview, processor.err
}

func intentAnalysisTaskFixture(runID int64, kind string) monitorapplication.IntentAnalysisTaskDTO {
	inputHash := strings.Repeat("a", 64)
	profile, sampleLimit := "monitor-intent-expansion-v1", 0
	if kind == "preview" {
		inputHash, profile, sampleLimit = strings.Repeat("b", 64), "intent-preview-v1", 25
	}
	return monitorapplication.IntentAnalysisTaskDTO{
		Run: monitorapplication.IntentRunReferenceDTO{
			RunID: runID, Kind: kind, MonitorID: 7, DraftID: 11, DraftResourceVersion: 4, InputHash: inputHash,
		},
		AnalysisProfile: profile, SampleLimit: sampleLimit,
	}
}
