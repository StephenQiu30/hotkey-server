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

type publishedMonitorMatchBackfillJobArgs struct {
	MonitorID         int64  `json:"monitor_id"`
	MonitorVersionID  int64  `json:"monitor_version_id"`
	CompiledProfileID int64  `json:"compiled_profile_id"`
	TraceID           string `json:"trace_id"`
}

func (args publishedMonitorMatchBackfillJobArgs) validate() error {
	if args.MonitorID <= 0 || args.MonitorVersionID <= 0 || args.CompiledProfileID <= 0 || !validOptionalTraceID(args.TraceID) {
		return fmt.Errorf("published monitor match backfill job args are invalid")
	}
	return nil
}

type PublishedMonitorMatchBackfillScheduler struct {
	jobs publishedMonitorMatchBackfillJobEnqueuer
	now  func() time.Time
}

type publishedMonitorMatchBackfillJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

var _ ingestionapplication.PublishedMonitorMatchBackfillScheduler = (*PublishedMonitorMatchBackfillScheduler)(nil)

func NewPublishedMonitorMatchBackfillScheduler(jobs *queue.Store) (*PublishedMonitorMatchBackfillScheduler, error) {
	return newPublishedMonitorMatchBackfillScheduler(jobs, func() time.Time { return time.Now().UTC() })
}

func newPublishedMonitorMatchBackfillScheduler(jobs publishedMonitorMatchBackfillJobEnqueuer, now func() time.Time) (*PublishedMonitorMatchBackfillScheduler, error) {
	if jobs == nil || now == nil {
		return nil, fmt.Errorf("published monitor match backfill queue is required")
	}
	return &PublishedMonitorMatchBackfillScheduler{jobs: jobs, now: now}, nil
}

func (scheduler *PublishedMonitorMatchBackfillScheduler) SchedulePublishedMonitorMatchBackfill(ctx context.Context, command ingestionapplication.SchedulePublishedMonitorMatchBackfillCommand) (ingestionapplication.SchedulePublishedMonitorMatchBackfillResult, error) {
	result := ingestionapplication.SchedulePublishedMonitorMatchBackfillResult{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID, CompiledProfileID: command.CompiledProfileID,
	}
	if scheduler == nil || scheduler.jobs == nil || scheduler.now == nil {
		return result, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	args := publishedMonitorMatchBackfillJobArgs{
		MonitorID: command.MonitorID, MonitorVersionID: command.MonitorVersionID,
		CompiledProfileID: command.CompiledProfileID, TraceID: sharedrequestcontext.TraceID(ctx),
	}
	jobID, created, err := scheduler.enqueue(ctx, args)
	if err != nil {
		return result, err
	}
	result.JobID, result.Created = jobID, created
	return result, nil
}

func (scheduler *PublishedMonitorMatchBackfillScheduler) enqueue(ctx context.Context, args publishedMonitorMatchBackfillJobArgs) (int64, bool, error) {
	if scheduler == nil || scheduler.jobs == nil || scheduler.now == nil || args.validate() != nil {
		return 0, false, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return 0, false, fmt.Errorf("encode published monitor match backfill job args")
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
		Kind:        queue.KindBackfillPublishedMonitorMatches,
		UniqueKey:   PublishedMonitorMatchBackfillUniqueKey(args.MonitorVersionID, args.CompiledProfileID),
		DurableArgs: encoded, ScheduledAt: scheduler.now().UTC(), MaxAttempts: 5, Priority: 4,
	})
	if err != nil {
		return 0, false, fmt.Errorf("enqueue published monitor match backfill: %w", err)
	}
	if jobID <= 0 {
		return 0, false, ingestionapplication.ErrInvalidDocumentMatchContract
	}
	return jobID, created, nil
}

func PublishedMonitorMatchBackfillUniqueKey(monitorVersionID, compiledProfileID int64) string {
	if monitorVersionID <= 0 || compiledProfileID <= 0 {
		return ""
	}
	return queue.StableJobHash(queue.KindBackfillPublishedMonitorMatches, strconv.FormatInt(monitorVersionID, 10),
		strconv.FormatInt(compiledProfileID, 10))
}

type PublishedMonitorMatchBackfillHandler struct {
	service   *ingestionapplication.PublishedMonitorMatchBackfillService
	scheduler *PublishedMonitorMatchBackfillScheduler
}

func NewPublishedMonitorMatchBackfillHandler(service *ingestionapplication.PublishedMonitorMatchBackfillService, scheduler *PublishedMonitorMatchBackfillScheduler) (*PublishedMonitorMatchBackfillHandler, error) {
	if service == nil || scheduler == nil {
		return nil, fmt.Errorf("published monitor match backfill service and scheduler are required")
	}
	return &PublishedMonitorMatchBackfillHandler{service: service, scheduler: scheduler}, nil
}

func (handler *PublishedMonitorMatchBackfillHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindBackfillPublishedMonitorMatches); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.service == nil || handler.scheduler == nil {
		return queue.NewRetryableError(fmt.Errorf("published monitor match backfill handler is unavailable"))
	}
	args, err := decodePublishedMonitorMatchBackfillJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	_, err = handler.service.Backfill(ctx, ingestionapplication.BackfillPublishedMonitorMatchesCommand{
		MonitorID: args.MonitorID, MonitorVersionID: args.MonitorVersionID,
		CompiledProfileID: args.CompiledProfileID,
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	return nil
}

func decodePublishedMonitorMatchBackfillJobArgs(encoded []byte) (publishedMonitorMatchBackfillJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args publishedMonitorMatchBackfillJobArgs
	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("decode published monitor match backfill job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return args, fmt.Errorf("published monitor match backfill job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return args, err
	}
	return args, nil
}
