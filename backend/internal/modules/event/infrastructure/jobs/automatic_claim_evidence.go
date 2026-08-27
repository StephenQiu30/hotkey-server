package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type automaticClaimEvidenceJobArgs struct {
	MicroEventID      int64  `json:"micro_event_id"`
	DocumentVersionID int64  `json:"document_version_id"`
	TraceID           string `json:"trace_id"`
}

func (args automaticClaimEvidenceJobArgs) validate() error {
	if args.MicroEventID <= 0 || args.DocumentVersionID <= 0 || !validAutomaticClaimEvidenceTraceID(args.TraceID) {
		return fmt.Errorf("automatic claim evidence job args are invalid")
	}
	return nil
}

func validAutomaticClaimEvidenceTraceID(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if (value[index] < '0' || value[index] > '9') && (value[index] < 'a' || value[index] > 'f') {
			return false
		}
	}
	return true
}

type automaticClaimEvidenceJobEnqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

type AutomaticClaimEvidenceScheduler struct {
	jobs automaticClaimEvidenceJobEnqueuer
	now  func() time.Time
}

var _ eventapplication.AutomaticClaimEvidenceScheduler = (*AutomaticClaimEvidenceScheduler)(nil)

func NewAutomaticClaimEvidenceScheduler(jobs *queue.Store) (*AutomaticClaimEvidenceScheduler, error) {
	return newAutomaticClaimEvidenceScheduler(jobs, func() time.Time { return time.Now().UTC() })
}

func newAutomaticClaimEvidenceScheduler(jobs automaticClaimEvidenceJobEnqueuer, now func() time.Time) (*AutomaticClaimEvidenceScheduler, error) {
	if jobs == nil || now == nil {
		return nil, fmt.Errorf("automatic claim evidence queue is required")
	}
	return &AutomaticClaimEvidenceScheduler{jobs: jobs, now: now}, nil
}

func (scheduler *AutomaticClaimEvidenceScheduler) ScheduleAutomaticClaimEvidence(ctx context.Context,
	command eventapplication.ScheduleAutomaticClaimEvidenceCommand) (eventapplication.ScheduleAutomaticClaimEvidenceResult, error) {
	result := eventapplication.ScheduleAutomaticClaimEvidenceResult{
		MicroEventID: command.MicroEventID, DocumentVersionID: command.DocumentVersionID,
	}
	args := automaticClaimEvidenceJobArgs{MicroEventID: command.MicroEventID,
		DocumentVersionID: command.DocumentVersionID, TraceID: sharedrequestcontext.TraceID(ctx)}
	if scheduler == nil || scheduler.jobs == nil || scheduler.now == nil || args.validate() != nil {
		return result, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return result, fmt.Errorf("encode automatic claim evidence job args")
	}
	jobID, created, err := scheduler.jobs.Enqueue(ctx, queue.Job{
		Kind: queue.KindExtractAutomaticClaimEvidence, UniqueKey: AutomaticClaimEvidenceUniqueKey(args.MicroEventID, args.DocumentVersionID),
		DurableArgs: encoded, ScheduledAt: scheduler.now().UTC(), MaxAttempts: 5, Priority: 6,
	})
	if err != nil {
		return result, fmt.Errorf("enqueue automatic claim evidence: %w", err)
	}
	if jobID <= 0 {
		return result, eventapplication.ErrInvalidAutomaticClaimEvidenceContract
	}
	result.JobID, result.Created = jobID, created
	return result, nil
}

func AutomaticClaimEvidenceUniqueKey(microEventID, documentVersionID int64) string {
	if microEventID <= 0 || documentVersionID <= 0 {
		return ""
	}
	return queue.StableJobHash(queue.KindExtractAutomaticClaimEvidence, strconv.FormatInt(microEventID, 10),
		strconv.FormatInt(documentVersionID, 10))
}

type automaticClaimEvidenceExtractor interface {
	Extract(context.Context, eventapplication.AutomaticClaimEvidenceCommand) (eventapplication.AutomaticClaimEvidenceResult, error)
}

type AutomaticClaimEvidenceHandler struct {
	service automaticClaimEvidenceExtractor
}

func NewAutomaticClaimEvidenceHandler(service *eventapplication.AutomaticClaimEvidenceService) (*AutomaticClaimEvidenceHandler, error) {
	return newAutomaticClaimEvidenceHandler(service)
}

func newAutomaticClaimEvidenceHandler(service automaticClaimEvidenceExtractor) (*AutomaticClaimEvidenceHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("automatic claim evidence service is required")
	}
	return &AutomaticClaimEvidenceHandler{service: service}, nil
}

func (handler *AutomaticClaimEvidenceHandler) Handle(ctx context.Context, job queue.Job) error {
	if err := queue.ValidateHandlerJob(job, queue.KindExtractAutomaticClaimEvidence); err != nil {
		return queue.NewPermanentError(err)
	}
	if handler == nil || handler.service == nil {
		return queue.NewRetryableError(fmt.Errorf("automatic claim evidence handler is unavailable"))
	}
	args, err := decodeAutomaticClaimEvidenceJobArgs(job.DurableArgs)
	if err != nil {
		return queue.NewPermanentError(err)
	}
	if args.TraceID != "" {
		ctx = sharedrequestcontext.WithTraceID(ctx, args.TraceID)
	}
	result, err := handler.service.Extract(ctx, eventapplication.AutomaticClaimEvidenceCommand{
		MicroEventID: args.MicroEventID, DocumentVersionID: args.DocumentVersionID,
	})
	if err != nil {
		return queue.ClassifyHandlerError(ctx, err)
	}
	if result.Status == "degraded" {
		if strings.HasPrefix(result.ReasonCode, "ai_") {
			return queue.NewRetryableError(fmt.Errorf("automatic claim evidence pending analysis"))
		}
		return queue.NewPermanentError(fmt.Errorf("automatic claim evidence degraded"))
	}
	if result.Status != "succeeded" || result.ModelRunID <= 0 || result.EvidenceState == nil || result.Summary == nil {
		return queue.NewPermanentError(eventapplication.ErrInvalidAutomaticClaimEvidenceContract)
	}
	return nil
}

func decodeAutomaticClaimEvidenceJobArgs(encoded []byte) (automaticClaimEvidenceJobArgs, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var args automaticClaimEvidenceJobArgs
	if err := decoder.Decode(&args); err != nil {
		return args, fmt.Errorf("decode automatic claim evidence job args")
	}
	if err := decoder.Decode(new(struct{})); err != io.EOF {
		return args, fmt.Errorf("automatic claim evidence job args contain trailing data")
	}
	if err := args.validate(); err != nil {
		return args, err
	}
	return args, nil
}
