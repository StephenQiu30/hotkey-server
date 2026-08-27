package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type productEventRefreshJobArgs struct {
	MicroEventID                  int64     `json:"micro_event_id"`
	MicroEventVersion             int64     `json:"micro_event_version"`
	WindowEndedAt                 time.Time `json:"window_ended_at"`
	WindowProfile                 string    `json:"window_profile"`
	HeatProfileVersion            string    `json:"heat_profile_version"`
	EvidenceStateAlgorithmVersion string    `json:"evidence_state_algorithm_version"`
	TraceID                       string    `json:"trace_id"`
}

func (args productEventRefreshJobArgs) validate() error {
	if args.MicroEventID <= 0 || args.MicroEventVersion <= 0 || args.WindowEndedAt.IsZero() ||
		!args.WindowEndedAt.Equal(args.WindowEndedAt.UTC().Truncate(time.Minute)) ||
		args.WindowProfile != eventapplication.ProductEventRefreshWindowProfile || strings.TrimSpace(args.HeatProfileVersion) == "" ||
		args.EvidenceStateAlgorithmVersion != eventapplication.CanonicalEvidenceStateAlgorithmVersion ||
		!validAutomaticClaimEvidenceTraceID(args.TraceID) {
		return fmt.Errorf("product event refresh job args are invalid")
	}
	return nil
}

type productEventRefreshJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type ProductEventRefreshScheduler struct {
	targets eventapplication.ProductEventRefreshScheduleTargetReader
	jobs    productEventRefreshJobEnqueuer
	now     func() time.Time
}

var _ eventapplication.ProductEventRefreshScheduler = (*ProductEventRefreshScheduler)(nil)

func NewProductEventRefreshScheduler(targets eventapplication.ProductEventRefreshScheduleTargetReader,
	jobs *queue.Store) (*ProductEventRefreshScheduler, error) {
	return newProductEventRefreshScheduler(targets, jobs, func() time.Time { return time.Now().UTC() })
}

func newProductEventRefreshScheduler(targets eventapplication.ProductEventRefreshScheduleTargetReader,
	jobs productEventRefreshJobEnqueuer, now func() time.Time) (*ProductEventRefreshScheduler, error) {
	if targets == nil || jobs == nil || now == nil {
		return nil, fmt.Errorf("product event refresh scheduler dependencies are required")
	}
	return &ProductEventRefreshScheduler{targets: targets, jobs: jobs, now: now}, nil
}

func (scheduler *ProductEventRefreshScheduler) ScheduleProductEventRefresh(ctx context.Context,
	command eventapplication.ScheduleProductEventRefreshCommand) (eventapplication.ScheduleProductEventRefreshResult, error) {
	result := eventapplication.ScheduleProductEventRefreshResult{MicroEventID: command.MicroEventID}
	if scheduler == nil || scheduler.targets == nil || scheduler.jobs == nil || scheduler.now == nil ||
		command.MicroEventID <= 0 || command.ExpectedEventVersion < 0 {
		return result, eventapplication.ErrInvalidProductEventRefreshContract
	}
	target, err := scheduler.targets.ReadProductEventRefreshScheduleTarget(ctx, eventapplication.ProductEventRefreshScheduleTargetQuery{
		MicroEventID: command.MicroEventID, ExpectedEventVersion: command.ExpectedEventVersion,
	})
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read product event refresh schedule target: %w", err)
	}
	if target.MicroEventID != command.MicroEventID || target.MicroEventVersion <= 0 ||
		command.ExpectedEventVersion > 0 && target.MicroEventVersion != command.ExpectedEventVersion ||
		target.HeatProfileID <= 0 || strings.TrimSpace(target.HeatProfileVersion) == "" || target.EvidenceStateProfileID <= 0 ||
		target.EvidenceStateAlgorithmVersion != eventapplication.CanonicalEvidenceStateAlgorithmVersion {
		return result, eventapplication.ErrInvalidProductEventRefreshContract
	}
	windowEnd := command.WindowEndedAt
	if windowEnd.IsZero() {
		windowEnd = scheduler.now()
	}
	windowEnd = windowEnd.UTC().Truncate(time.Minute)
	args := productEventRefreshJobArgs{MicroEventID: target.MicroEventID, MicroEventVersion: target.MicroEventVersion,
		WindowEndedAt: windowEnd, WindowProfile: eventapplication.ProductEventRefreshWindowProfile,
		HeatProfileVersion:            target.HeatProfileVersion,
		EvidenceStateAlgorithmVersion: target.EvidenceStateAlgorithmVersion, TraceID: sharedrequestcontext.TraceID(ctx)}
	if args.validate() != nil {
		return result, eventapplication.ErrInvalidProductEventRefreshContract
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return result, fmt.Errorf("encode product event refresh job args")
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{Kind: queue.KindRefreshProductEvent,
		UniqueKey: ProductEventRefreshUniqueKey(args), DurableArgs: encoded, ScheduledAt: scheduler.now().UTC(),
		MaxAttempts: 5, Priority: 7})
	if err != nil {
		return result, fmt.Errorf("enqueue product event refresh: %w", err)
	}
	if jobID <= 0 {
		return result, eventapplication.ErrInvalidProductEventRefreshContract
	}
	result.MicroEventVersion, result.JobID, result.Created, result.Available = target.MicroEventVersion, jobID, created, true
	return result, nil
}

func ProductEventRefreshUniqueKey(args productEventRefreshJobArgs) string {
	if args.validate() != nil {
		return ""
	}
	return queue.StableJobHash(queue.KindRefreshProductEvent, strconv.FormatInt(args.MicroEventID, 10),
		strconv.FormatInt(args.MicroEventVersion, 10), args.WindowEndedAt.Format(time.RFC3339), args.WindowProfile,
		args.HeatProfileVersion, args.EvidenceStateAlgorithmVersion)
}

type productEventRefresher interface {
	Refresh(context.Context, eventapplication.RefreshProductEventCommand) (eventapplication.ProductEventRefreshResult, error)
}

type ProductEventRefreshHandler struct{ service productEventRefresher }

func NewProductEventRefreshHandler(service *eventapplication.ProductEventRefreshService) (*ProductEventRefreshHandler, error) {
	return newProductEventRefreshHandler(service)
}

func newProductEventRefreshHandler(service productEventRefresher) (*ProductEventRefreshHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("product event refresh service is required")
	}
	return &ProductEventRefreshHandler{service: service}, nil
}

func (handler *ProductEventRefreshHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindRefreshProductEvent); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.service == nil {
		return queue.NewRetryableError(fmt.Errorf("product event refresh handler is unavailable"))
	}
	args, err := decodeProductEventRefreshJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	result, err := handler.service.Refresh(ctx, eventapplication.RefreshProductEventCommand{
		MicroEventID: args.MicroEventID, ExpectedEventVersion: args.MicroEventVersion,
		WindowEndedAt: args.WindowEndedAt, WindowProfile: args.WindowProfile,
		HeatProfileVersion:            args.HeatProfileVersion,
		EvidenceStateAlgorithmVersion: args.EvidenceStateAlgorithmVersion,
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if result.Update.ID <= 0 || result.Update.MicroEventID != args.MicroEventID ||
		result.Update.MicroEventVersion != args.MicroEventVersion || result.Update.RefreshKey == "" {
		return queue.NewPermanentError(eventapplication.ErrInvalidProductEventRefreshContract)
	}
	return nil
}

func decodeProductEventRefreshJobArgs(encoded []byte) (productEventRefreshJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args productEventRefreshJobArgs
	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("decode product event refresh job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return args, fmt.Errorf("product event refresh job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return args, err
	}
	return args, nil
}
