package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	monitorapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

const (
	IntentAnalysisFailureReasonExpansion = "expansion_processor_failed"
	IntentAnalysisFailureReasonPreview   = "preview_processor_failed"
)

// IntentAnalysisJobArgs is the bounded durable worker contract. The worker
// rereads the exact draft revision and never receives objective, examples,
// candidates, document text, or raw evidence through River.
type IntentAnalysisJobArgs struct {
	RunID                int64 `json:"run_id"`
	DraftID              int64 `json:"draft_id"`
	DraftResourceVersion int64 `json:"draft_resource_version"`
}

func (args IntentAnalysisJobArgs) validate() error {
	if args.RunID <= 0 || args.DraftID <= 0 || args.DraftResourceVersion <= 0 {
		return fmt.Errorf("monitor intent analysis job identity is invalid")
	}
	return nil
}

func EncodeIntentAnalysisJobArgs(args IntentAnalysisJobArgs) ([]byte, error) {
	if err := args.validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode monitor intent analysis job args: %w", err)
	}
	return encoded, nil
}

func DecodeIntentAnalysisJobArgs(encoded []byte) (IntentAnalysisJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args IntentAnalysisJobArgs
	if err := decoder.Decode(&args); err != nil {
		return IntentAnalysisJobArgs{}, fmt.Errorf("decode monitor intent analysis job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return IntentAnalysisJobArgs{}, fmt.Errorf("monitor intent analysis job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return IntentAnalysisJobArgs{}, err
	}
	return args, nil
}

// IntentAnalysisProcessor is intentionally strict: production wiring requires
// a real generator/evaluator. Bootstrap must not satisfy this port with a
// no-op, fixed candidates, or an empty preview implementation.
type IntentAnalysisProcessor interface {
	GenerateExpansion(context.Context, monitorapplication.IntentAnalysisTaskDTO) ([]monitorapplication.ExpansionCandidateDTO, error)
	EvaluatePreview(context.Context, monitorapplication.IntentAnalysisTaskDTO) (monitorapplication.IntentPreviewDTO, error)
}

type intentRunController interface {
	ReadIntentAnalysisTask(context.Context, monitorapplication.ReadIntentAnalysisTaskQuery) (monitorapplication.ReadIntentAnalysisTaskResult, error)
	StartIntentRun(context.Context, monitorapplication.StartIntentRunCommand) (monitorapplication.StartIntentRunResult, error)
	FailIntentRun(context.Context, monitorapplication.FailIntentRunCommand) (monitorapplication.FailIntentRunResult, error)
	CompleteExpansionRun(context.Context, monitorapplication.CompleteExpansionRunCommand) (monitorapplication.CompleteExpansionRunResult, error)
	CompletePreviewRun(context.Context, monitorapplication.CompletePreviewRunCommand) (monitorapplication.CompletePreviewRunResult, error)
}

type IntentAnalysisHandler struct {
	controller intentRunController
	processor  IntentAnalysisProcessor
}

func NewIntentAnalysisHandler(controller *monitorapplication.IntentService, processor IntentAnalysisProcessor) (*IntentAnalysisHandler, error) {
	return newIntentAnalysisHandler(controller, processor)
}

func newIntentAnalysisHandler(controller intentRunController, processor IntentAnalysisProcessor) (*IntentAnalysisHandler, error) {
	if controller == nil || processor == nil {
		return nil, fmt.Errorf("monitor intent analysis handler requires a real controller and processor")
	}
	return &IntentAnalysisHandler{controller: controller, processor: processor}, nil
}

func (handler *IntentAnalysisHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindAnalyzeMonitorIntent); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.controller == nil || handler.processor == nil {
		return queue.NewRetryableError(fmt.Errorf("monitor intent analysis handler is unavailable"))
	}
	args, err := DecodeIntentAnalysisJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	resolved, err := handler.controller.ReadIntentAnalysisTask(ctx, monitorapplication.ReadIntentAnalysisTaskQuery{
		RunID: args.RunID, DraftID: args.DraftID, DraftResourceVersion: args.DraftResourceVersion,
	})
	if err != nil {
		return classifyIntentAnalysisControllerError(ctx, err)
	}
	task := resolved.Task
	reference := task.Run
	started, err := handler.controller.StartIntentRun(ctx, monitorapplication.StartIntentRunCommand{Run: reference})
	if err != nil {
		return classifyIntentAnalysisControllerError(ctx, err)
	}
	if started.Run.Status == "failed" || started.Run.Status == "succeeded" || started.Run.Status == "invalidated" {
		return nil
	}
	if started.Run.Status != "running" {
		return queue.NewPermanentError(fmt.Errorf("monitor intent run start returned an invalid status"))
	}

	switch task.Run.Kind {
	case "expansion":
		candidates, processErr := handler.processor.GenerateExpansion(ctx, task)
		if processErr != nil {
			return handler.persistSafeFailure(ctx, reference, IntentAnalysisFailureReasonExpansion)
		}
		_, err = handler.controller.CompleteExpansionRun(ctx, monitorapplication.CompleteExpansionRunCommand{Run: reference, Candidates: candidates})
	case "preview":
		preview, processErr := handler.processor.EvaluatePreview(ctx, task)
		if processErr != nil {
			return handler.persistSafeFailure(ctx, reference, IntentAnalysisFailureReasonPreview)
		}
		_, err = handler.controller.CompletePreviewRun(ctx, monitorapplication.CompletePreviewRunCommand{Run: reference, Preview: preview})
	default:
		return queue.NewPermanentError(fmt.Errorf("monitor intent analysis kind is invalid"))
	}
	return classifyIntentAnalysisControllerError(ctx, err)
}

func (handler *IntentAnalysisHandler) persistSafeFailure(ctx context.Context, reference monitorapplication.IntentRunReferenceDTO, reason string) error {
	_, err := handler.controller.FailIntentRun(ctx, monitorapplication.FailIntentRunCommand{Run: reference, Reason: reason})
	return classifyIntentAnalysisControllerError(ctx, err)
}

func classifyIntentAnalysisControllerError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, monitorapplication.ErrInvalidIntentContract),
		errors.Is(err, monitorapplication.ErrIntentDraftNotFound),
		errors.Is(err, monitorapplication.ErrIntentRunNotFound),
		errors.Is(err, monitorapplication.ErrIntentVersionConflict),
		errors.Is(err, monitorapplication.ErrIntentRunStateConflict),
		errors.Is(err, monitorapplication.ErrIntentRunResultConflict):
		return queue.NewPermanentError(err)
	default:
		return queue.ClassifyHandlerError(ctx, err)
	}
}
