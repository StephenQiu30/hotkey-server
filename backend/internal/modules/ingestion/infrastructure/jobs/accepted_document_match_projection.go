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

type acceptedDocumentMatchProjectionJobArgs struct {
	DocumentMatchDecisionID int64  `json:"document_match_decision_id"`
	DocumentVersionID       int64  `json:"document_version_id"`
	EffectiveSequence       int64  `json:"effective_sequence"`
	TraceID                 string `json:"trace_id"`
}

func (args acceptedDocumentMatchProjectionJobArgs) validate() error {
	if args.DocumentMatchDecisionID <= 0 || args.DocumentVersionID <= 0 || args.EffectiveSequence <= 0 || !validOptionalTraceID(args.TraceID) {
		return fmt.Errorf("accepted document match projection job args are invalid")
	}
	return nil
}

type acceptedDocumentMatchProjectionJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type AcceptedDocumentMatchProjectionScheduler struct {
	jobs acceptedDocumentMatchProjectionJobEnqueuer
	now  func() time.Time
}

var _ ingestionapplication.AcceptedDocumentMatchProjectionScheduler = (*AcceptedDocumentMatchProjectionScheduler)(nil)

func NewAcceptedDocumentMatchProjectionScheduler(jobs *queue.Store) (*AcceptedDocumentMatchProjectionScheduler, error) {
	return newAcceptedDocumentMatchProjectionScheduler(jobs, func() time.Time { return time.Now().UTC() })
}

func newAcceptedDocumentMatchProjectionScheduler(jobs acceptedDocumentMatchProjectionJobEnqueuer, now func() time.Time) (*AcceptedDocumentMatchProjectionScheduler, error) {
	if jobs == nil || now == nil {
		return nil, fmt.Errorf("accepted document match projection queue is required")
	}
	return &AcceptedDocumentMatchProjectionScheduler{jobs: jobs, now: now}, nil
}

func (scheduler *AcceptedDocumentMatchProjectionScheduler) ScheduleAcceptedDocumentMatchProjection(ctx context.Context,
	command ingestionapplication.ScheduleAcceptedDocumentMatchProjectionCommand) (ingestionapplication.ScheduleAcceptedDocumentMatchProjectionResult, error) {
	result := ingestionapplication.ScheduleAcceptedDocumentMatchProjectionResult{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID:       command.DocumentVersionID,
		EffectiveSequence:       command.EffectiveSequence,
	}
	args := acceptedDocumentMatchProjectionJobArgs{
		DocumentMatchDecisionID: command.DocumentMatchDecisionID,
		DocumentVersionID:       command.DocumentVersionID,
		EffectiveSequence:       command.EffectiveSequence,
		TraceID:                 sharedrequestcontext.TraceID(ctx),
	}
	if scheduler == nil || scheduler.jobs == nil || scheduler.now == nil || args.validate() != nil {
		return result, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return result, fmt.Errorf("encode accepted document match projection job args")
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindProjectAcceptedDocumentMatch,
		UniqueKey:   AcceptedDocumentMatchProjectionUniqueKey(args.DocumentMatchDecisionID, args.EffectiveSequence),
		DurableArgs: encoded,
		ScheduledAt: scheduler.now().UTC(),
		MaxAttempts: 5,
		Priority:    4,
	})
	if err != nil {
		return result, fmt.Errorf("enqueue accepted document match projection: %w", err)
	}
	if jobID <= 0 {
		return result, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	result.JobID, result.Created = jobID, created
	return result, nil
}

func AcceptedDocumentMatchProjectionUniqueKey(documentMatchDecisionID, effectiveSequence int64) string {
	if documentMatchDecisionID <= 0 || effectiveSequence <= 0 {
		return ""
	}
	return queue.StableJobHash(queue.KindProjectAcceptedDocumentMatch, strconv.FormatInt(documentMatchDecisionID, 10),
		strconv.FormatInt(effectiveSequence, 10))
}

type AcceptedDocumentMatchProjectionHandler struct {
	consumer ingestionapplication.AcceptedDocumentMatchConsumer
}

func NewAcceptedDocumentMatchProjectionHandler(consumer ingestionapplication.AcceptedDocumentMatchConsumer) (*AcceptedDocumentMatchProjectionHandler, error) {
	if consumer == nil {
		return nil, fmt.Errorf("accepted document match projection consumer is required")
	}
	return &AcceptedDocumentMatchProjectionHandler{consumer: consumer}, nil
}

func (handler *AcceptedDocumentMatchProjectionHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindProjectAcceptedDocumentMatch); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.consumer == nil {
		return queue.NewRetryableError(fmt.Errorf("accepted document match projection handler is unavailable"))
	}
	args, err := decodeAcceptedDocumentMatchProjectionJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	consumed, err := handler.consumer.ConsumeAcceptedDocumentMatch(ctx, ingestionapplication.ConsumeAcceptedDocumentMatchCommand{
		DocumentMatchDecisionID: args.DocumentMatchDecisionID,
		DocumentVersionID:       args.DocumentVersionID,
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if consumed.DocumentMatchDecisionID != args.DocumentMatchDecisionID || consumed.DocumentVersionID != args.DocumentVersionID {
		return queue.NewPermanentError(ingestionapplication.ErrInvalidDocumentMatchContract)
	}
	return nil
}

func decodeAcceptedDocumentMatchProjectionJobArgs(encoded []byte) (acceptedDocumentMatchProjectionJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args acceptedDocumentMatchProjectionJobArgs
	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("decode accepted document match projection job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return args, fmt.Errorf("accepted document match projection job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return args, err
	}
	return args, nil
}
