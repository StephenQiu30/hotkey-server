package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type publishedDocumentMatchEvaluationJobArgs struct {
	DocumentVersionID int64  `json:"document_version_id"`
	TraceID           string `json:"trace_id"`
}

func (args publishedDocumentMatchEvaluationJobArgs) validate() error {
	if args.DocumentVersionID <= 0 || !validOptionalTraceID(args.TraceID) {
		return fmt.Errorf("published document match evaluation job args are invalid")
	}
	return nil
}

type publishedDocumentMatchJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type PublishedDocumentMatchEvaluationScheduler struct {
	jobs publishedDocumentMatchJobEnqueuer
	now  func() time.Time
}

var _ ingestionapplication.PublishedDocumentMatchEvaluationScheduler = (*PublishedDocumentMatchEvaluationScheduler)(nil)

func NewPublishedDocumentMatchEvaluationScheduler(jobs *queue.Store) (*PublishedDocumentMatchEvaluationScheduler, error) {
	return newPublishedDocumentMatchEvaluationScheduler(jobs, func() time.Time { return time.Now().UTC() })
}

func newPublishedDocumentMatchEvaluationScheduler(jobs publishedDocumentMatchJobEnqueuer, now func() time.Time) (*PublishedDocumentMatchEvaluationScheduler, error) {
	if jobs == nil || now == nil {
		return nil, fmt.Errorf("published document match job enqueuer and clock are required")
	}
	return &PublishedDocumentMatchEvaluationScheduler{jobs: jobs, now: now}, nil
}

func (scheduler *PublishedDocumentMatchEvaluationScheduler) SchedulePublishedDocumentMatchEvaluation(ctx context.Context, command ingestionapplication.SchedulePublishedDocumentMatchEvaluationCommand) (ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult, error) {
	if scheduler == nil || scheduler.jobs == nil || scheduler.now == nil || command.DocumentVersionID <= 0 {
		return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	args := publishedDocumentMatchEvaluationJobArgs{
		DocumentVersionID: command.DocumentVersionID,
		TraceID:           sharedrequestcontext.TraceID(ctx),
	}
	if err := args.validate(); err != nil {
		return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{}, fmt.Errorf("encode published document match evaluation job args")
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindEvaluatePublishedDocumentMatches, UniqueKey: PublishedDocumentMatchEvaluationUniqueKey(command.DocumentVersionID),
		DurableArgs: encoded, ScheduledAt: scheduler.now().UTC(), MaxAttempts: 5, Priority: 4,
	})
	if err != nil {
		return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{}, fmt.Errorf("enqueue published document match evaluation: %w", err)
	}
	if jobID <= 0 {
		return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{}, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	return ingestionapplication.SchedulePublishedDocumentMatchEvaluationResult{
		DocumentVersionID: command.DocumentVersionID, JobID: jobID, Created: created,
	}, nil
}

func PublishedDocumentMatchEvaluationUniqueKey(documentVersionID int64) string {
	if documentVersionID <= 0 {
		return ""
	}
	return queue.StableJobHash(queue.KindEvaluatePublishedDocumentMatches, strconv.FormatInt(documentVersionID, 10))
}

type publishedMatchEvaluator interface {
	EvaluateForDocument(context.Context, ingestionapplication.EvaluatePublishedMatchesForDocumentCommand) (ingestionapplication.EvaluatePublishedMatchesForDocumentResult, error)
}

type PublishedDocumentMatchEvaluationHandler struct {
	evaluator publishedMatchEvaluator
}

func NewPublishedDocumentMatchEvaluationHandler(evaluator *ingestionapplication.PublishedMatchEvaluationService) (*PublishedDocumentMatchEvaluationHandler, error) {
	return newPublishedDocumentMatchEvaluationHandler(evaluator)
}

func newPublishedDocumentMatchEvaluationHandler(evaluator publishedMatchEvaluator) (*PublishedDocumentMatchEvaluationHandler, error) {
	if evaluator == nil {
		return nil, fmt.Errorf("published match evaluator is required")
	}
	return &PublishedDocumentMatchEvaluationHandler{evaluator: evaluator}, nil
}

func (handler *PublishedDocumentMatchEvaluationHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindEvaluatePublishedDocumentMatches); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.evaluator == nil {
		return queue.NewRetryableError(fmt.Errorf("published document match evaluator is unavailable"))
	}
	args, err := decodePublishedDocumentMatchEvaluationJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	_, err = handler.evaluator.EvaluateForDocument(ctx, ingestionapplication.EvaluatePublishedMatchesForDocumentCommand{
		DocumentVersionID: args.DocumentVersionID,
	})
	return queue.ClassifyHandlerError(ctx, err)
}

func decodePublishedDocumentMatchEvaluationJobArgs(encoded []byte) (publishedDocumentMatchEvaluationJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args publishedDocumentMatchEvaluationJobArgs
	if err := decoder.Decode(&args); err != nil {
		return publishedDocumentMatchEvaluationJobArgs{}, fmt.Errorf("decode published document match evaluation job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return publishedDocumentMatchEvaluationJobArgs{}, fmt.Errorf("published document match evaluation job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return publishedDocumentMatchEvaluationJobArgs{}, err
	}
	return args, nil
}
