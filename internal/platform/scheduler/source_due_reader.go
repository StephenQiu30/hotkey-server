package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

// CollectionDueSource is the scheduler-safe projection of one or more
// immutable published Monitor targets sharing a source/signature/window.
// Cron never receives connector details and never mutates a checkpoint.
type CollectionDueSource struct {
	SourceConnectionID int64
	ConfigVersionID    int64
	QuerySignature     string
	NextPollAt         time.Time
	CollectionInterval time.Duration
}

func (source CollectionDueSource) Validate() error {
	if source.SourceConnectionID <= 0 || source.ConfigVersionID <= 0 || source.QuerySignature == "" || len(source.QuerySignature) > 128 || source.NextPollAt.IsZero() {
		return fmt.Errorf("invalid collection due source")
	}
	if source.CollectionInterval < 5*time.Minute || source.CollectionInterval > 24*time.Hour || source.CollectionInterval%time.Minute != 0 {
		return fmt.Errorf("invalid collection collection interval")
	}
	return nil
}

type CollectionDueReader interface {
	ListDueCollections(context.Context, time.Time) ([]CollectionDueSource, error)
}

type CollectionScheduler struct {
	reader CollectionDueReader
	store  Enqueuer
}

func NewCollectionScheduler(reader CollectionDueReader, store Enqueuer) *CollectionScheduler {
	return &CollectionScheduler{reader: reader, store: store}
}

// RunOnce scans only due published targets and submits collect_source jobs.
// It does not call a connector, create a collection run, or advance a
// checkpoint; those facts belong to the worker and Source application.
func (scheduler *CollectionScheduler) RunOnce(ctx context.Context, now time.Time) (int, error) {
	if scheduler == nil || scheduler.reader == nil || scheduler.store == nil || now.IsZero() {
		return 0, fmt.Errorf("collection scheduler is not initialized")
	}
	sources, err := scheduler.reader.ListDueCollections(ctx, now.UTC())
	if err != nil {
		return 0, err
	}
	created := 0
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			return created, err
		}
		if !IsDue(DueSource{ID: source.SourceConnectionID, NextPoll: source.NextPollAt}, now) {
			return created, fmt.Errorf("collection due source is not due")
		}
		windowStart := source.NextPollAt.UTC()
		windowEnd := windowStart.Add(source.CollectionInterval)
		_, wasCreated, err := scheduler.store.Enqueue(ctx, queue.Job{
			Kind:        queue.KindCollectSource,
			UniqueKey:   CollectionUniqueKey(source.SourceConnectionID, source.QuerySignature, windowStart, windowEnd),
			Payload:     queue.Payload{EntityID: source.SourceConnectionID, EntityVersion: source.ConfigVersionID, WindowStart: windowStart, WindowEnd: windowEnd, InputHash: source.QuerySignature},
			ScheduledAt: now.UTC(),
			MaxAttempts: 3,
			Priority:    1,
		})
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (scheduler *CollectionScheduler) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("collection scheduler interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := scheduler.RunOnce(ctx, time.Now().UTC()); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func CollectionUniqueKey(sourceConnectionID int64, querySignature string, windowStart, windowEnd time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("collect_source:%d:%s:%s:%s", sourceConnectionID, querySignature, windowStart.UTC().Format(time.RFC3339Nano), windowEnd.UTC().Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:])
}
