package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

type collectionDueReaderFake struct {
	sources []CollectionDueSource
}

func (fake collectionDueReaderFake) ListDueCollections(context.Context, time.Time) ([]CollectionDueSource, error) {
	return fake.sources, nil
}

func TestCollectionSchedulerEnqueuesStableCollectionJobs(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	windowStart := now.Add(-5 * time.Minute)
	fake := &fakeEnqueuer{}
	scheduler := NewCollectionScheduler(collectionDueReaderFake{sources: []CollectionDueSource{{
		SourceConnectionID: 9, ConfigVersionID: 12, QuerySignature: "query-signature", NextPollAt: windowStart, CollectionInterval: 5 * time.Minute,
	}}}, fake)
	created, err := scheduler.RunOnce(context.Background(), now)
	if err != nil || created != 1 || len(fake.jobs) != 1 {
		t.Fatalf("RunOnce() = %d/%v, jobs=%d", created, err, len(fake.jobs))
	}
	job := fake.jobs[0]
	if job.Kind != queue.KindCollectSource || job.Payload.EntityID != 9 || job.Payload.EntityVersion != 12 || job.Payload.InputHash != "query-signature" {
		t.Fatalf("collection job envelope = %#v", job)
	}
	if job.UniqueKey != CollectionUniqueKey(9, "query-signature", windowStart, now) {
		t.Fatalf("collection unique key = %q", job.UniqueKey)
	}
}

func TestCollectionSchedulerRejectsInvalidDueSource(t *testing.T) {
	fake := &fakeEnqueuer{}
	scheduler := NewCollectionScheduler(collectionDueReaderFake{sources: []CollectionDueSource{{SourceConnectionID: 0}}}, fake)
	if _, err := scheduler.RunOnce(context.Background(), time.Now().UTC()); err == nil {
		t.Fatal("invalid due source was accepted")
	}
}

func TestManualCollectionUniqueKeyUsesFiveMinuteCooldownBuckets(t *testing.T) {
	first := time.Date(2026, 7, 17, 9, 1, 0, 0, time.UTC)
	second := first.Add(3 * time.Minute)
	next := first.Add(5 * time.Minute)
	firstKey := ManualCollectionUniqueKey(9, "query-signature", first)
	if secondKey := ManualCollectionUniqueKey(9, "query-signature", second); secondKey != firstKey {
		t.Fatalf("same cooldown bucket keys differ: %q != %q", secondKey, firstKey)
	}
	if nextKey := ManualCollectionUniqueKey(9, "query-signature", next); nextKey == firstKey {
		t.Fatal("next cooldown bucket reused the previous key")
	}
	if got := ManualCollectionCooldownUntil(first); !got.Equal(time.Date(2026, 7, 17, 9, 5, 0, 0, time.UTC)) {
		t.Fatalf("cooldown until = %s", got)
	}
}
