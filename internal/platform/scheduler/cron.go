// Package scheduler contains due-time and stable-key rules. It does not run
// business work in the scheduler process; it only asks the queue to enqueue a
// bounded job.
package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type DueSource struct {
	ID        int64
	NextPoll  time.Time
	QueryHash string
}

type Enqueuer interface {
	Enqueue(context.Context, queue.Job) (int64, bool, error)
}

// EnqueueDue performs only the scheduling decision. Business handlers remain
// in workers, and the queue's kind+unique key makes repeated Cron scans safe.
func EnqueueDue(ctx context.Context, store Enqueuer, source DueSource, kind string, version int64, now, windowStart, windowEnd time.Time) (int64, bool, error) {
	if store == nil || !IsDue(source, now) || kind == "" || version < 1 || windowEnd.Before(windowStart) {
		return 0, false, fmt.Errorf("invalid due job")
	}
	job := queue.Job{Kind: kind, UniqueKey: UniqueKey(kind, source.ID, version, windowStart, windowEnd), Payload: queue.Payload{EntityID: source.ID, EntityVersion: version, WindowStart: windowStart.UTC(), WindowEnd: windowEnd.UTC(), InputHash: source.QueryHash}, ScheduledAt: now.UTC(), MaxAttempts: 3, Priority: 1}
	return store.Enqueue(ctx, job)
}

func IsDue(source DueSource, now time.Time) bool {
	return source.ID > 0 && !source.NextPoll.IsZero() && !source.NextPoll.After(now)
}

func UniqueKey(kind string, entityID int64, version int64, windowStart, windowEnd time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s:%s", kind, entityID, version, windowStart.UTC().Format(time.RFC3339Nano), windowEnd.UTC().Format(time.RFC3339Nano))))
	return hex.EncodeToString(sum[:])
}
