package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type fakeEnqueuer struct{ jobs []queue.Job }

func (fake *fakeEnqueuer) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.jobs = append(fake.jobs, job)
	return int64(len(fake.jobs)), true, nil
}

func TestEnqueueDueUsesStableJobEnvelope(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	fake := &fakeEnqueuer{}
	id, created, err := EnqueueDue(context.Background(), fake, DueSource{ID: 9, NextPoll: now.Add(-time.Minute), QueryHash: "query-v1"}, "collect_source", 2, now, now.Add(-time.Hour), now)
	if err != nil || id != 1 || !created || len(fake.jobs) != 1 {
		t.Fatalf("EnqueueDue() = %d/%t/%v", id, created, err)
	}
	job := fake.jobs[0]
	if job.UniqueKey != UniqueKey("collect_source", 9, 2, now.Add(-time.Hour), now) || job.Payload.EntityID != 9 || job.Payload.InputHash != "query-v1" {
		t.Fatalf("job envelope = %#v", job)
	}
}
