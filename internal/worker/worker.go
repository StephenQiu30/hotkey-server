package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/service/ingest"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
)

type Queue interface {
	Claim(context.Context) (queue.Job, error)
	Complete(context.Context, string) (queue.Job, error)
	Fail(context.Context, string, error) (queue.Job, error)
}

type RedisHealth interface {
	Ping(context.Context) error
}

type Worker struct {
	queue    Queue
	redis    RedisHealth
	logger   *slog.Logger
	handlers map[queue.JobType]JobHandler
}

type Option func(*Worker)

type JobHandler interface {
	Handle(context.Context, queue.Job) error
}

func WithHandler(jobType queue.JobType, handler JobHandler) Option {
	return func(w *Worker) {
		if handler != nil {
			w.handlers[jobType] = handler
		}
	}
}

func New(jobQueue Queue, redis RedisHealth, logger *slog.Logger, opts ...Option) *Worker {
	if jobQueue == nil {
		panic("worker requires queue")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	worker := &Worker{
		queue:    jobQueue,
		redis:    redis,
		logger:   logger,
		handlers: make(map[queue.JobType]JobHandler),
	}
	for _, opt := range opts {
		opt(worker)
	}
	return worker
}

func (w *Worker) Run(ctx context.Context) error {
	if w.redis != nil {
		if err := w.redis.Ping(ctx); err != nil {
			w.logger.Warn("redis unavailable for worker", "error", err)
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	if err := w.runOnce(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.runOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	job, err := w.queue.Claim(ctx)
	if errors.Is(err, queue.ErrNoJobs) {
		return nil
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		w.logger.Warn("worker claim failed", "error", err)
		return nil
	}

	if handler, ok := w.handlers[job.Type]; ok {
		if err := handler.Handle(ctx, job); err != nil {
			w.logger.Warn("worker handler failed; marking job failed", "job_id", job.ID, "job_type", job.Type, "error", err)
			if _, failErr := w.queue.Fail(ctx, job.ID, err); failErr != nil {
				if errors.Is(failErr, context.Canceled) || errors.Is(failErr, context.DeadlineExceeded) {
					return failErr
				}
				w.logger.Warn("worker failure fallback failed", "job_id", job.ID, "job_type", job.Type, "error", failErr)
			}
			return nil
		}
	}

	completed, err := w.queue.Complete(ctx, job.ID)
	if err != nil {
		w.logger.Warn("worker complete failed; marking job failed", "job_id", job.ID, "job_type", job.Type, "error", err)
		if _, failErr := w.queue.Fail(ctx, job.ID, err); failErr != nil {
			if errors.Is(failErr, context.Canceled) || errors.Is(failErr, context.DeadlineExceeded) {
				return failErr
			}
			w.logger.Warn("worker failure fallback failed", "job_id", job.ID, "job_type", job.Type, "error", failErr)
		}
		return nil
	}
	w.logger.Info("completed placeholder job", "job_id", completed.ID, "job_type", completed.Type)
	return nil
}

func (w *Worker) Shutdown(context.Context) error {
	return nil
}

type CollectIngestInput = ingest.Input

type SourceService interface {
	SourceByID(context.Context, string) (servicesource.Source, error)
	RecordCollectionRun(context.Context, servicesource.RecordCollectionRunInput) (servicesource.CollectionRun, error)
}

type SourceFetcher interface {
	Fetch(context.Context, fetcher.Source) ([]fetcher.Item, error)
}

type IngestService interface {
	Ingest(context.Context, CollectIngestInput) (ingest.Result, error)
}

type CollectSourceHandler struct {
	sourceService SourceService
	fetcher       SourceFetcher
	ingest        IngestService
	now           func() time.Time
}

func NewCollectSourceHandler(sourceService SourceService, sourceFetcher SourceFetcher, ingestService IngestService) *CollectSourceHandler {
	return &CollectSourceHandler{
		sourceService: sourceService,
		fetcher:       sourceFetcher,
		ingest:        ingestService,
		now:           time.Now,
	}
}

func (h *CollectSourceHandler) Handle(ctx context.Context, job queue.Job) error {
	var payload queue.CollectSourcePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}
	if payload.SourceID == "" {
		return errors.New("collect source payload missing source_id")
	}
	startedAt := h.now().UTC()
	source, err := h.sourceService.SourceByID(ctx, payload.SourceID)
	if err != nil {
		_, _ = h.recordRun(ctx, payload.SourceID, servicesource.CollectionRunStatusFailed, 0, err.Error(), startedAt)
		return err
	}
	items, err := h.fetcher.Fetch(ctx, fetcher.Source{
		ID:             source.ID,
		Type:           fetcher.SourceType(source.Type),
		URL:            source.URL,
		ComplianceNote: source.ComplianceNote,
	})
	if err != nil {
		_, _ = h.recordRun(ctx, source.ID, servicesource.CollectionRunStatusFailed, 0, err.Error(), startedAt)
		return err
	}

	ingested := 0
	for _, item := range items {
		snippet := strings.TrimSpace(item.Title)
		if _, err := h.ingest.Ingest(ctx, CollectIngestInput{
			SourceID:    source.ID,
			Title:       item.Title,
			Snippet:     snippet,
			URL:         item.URL,
			Language:    "unknown",
			PublishedAt: item.PublishedAt,
		}); err != nil {
			continue
		}
		ingested++
	}
	_, err = h.recordRun(ctx, source.ID, servicesource.CollectionRunStatusSuccess, ingested, "", startedAt)
	return err
}

func (h *CollectSourceHandler) recordRun(ctx context.Context, sourceID string, status servicesource.CollectionRunStatus, itemsFetched int, message string, startedAt time.Time) (servicesource.CollectionRun, error) {
	return h.sourceService.RecordCollectionRun(ctx, servicesource.RecordCollectionRunInput{
		SourceID:     sourceID,
		Status:       status,
		ItemsFetched: itemsFetched,
		Error:        message,
		StartedAt:    startedAt,
		FinishedAt:   h.now().UTC(),
	})
}
