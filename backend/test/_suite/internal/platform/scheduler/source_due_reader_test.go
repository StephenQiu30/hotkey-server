package scheduler

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
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
		MonitorID: 7, MonitorVersionID: 12, CompiledProfileID: 31,
		SourceConnectionID: 9, QuerySignature: strings.Repeat("a", 64), NextPollAt: windowStart, CollectionInterval: 5 * time.Minute,
	}}}, fake)
	created, err := scheduler.RunOnce(context.Background(), now)
	if err != nil || created != 1 || len(fake.jobs) != 1 {
		t.Fatalf("RunOnce() = %d/%v, jobs=%d", created, err, len(fake.jobs))
	}
	job := fake.jobs[0]
	if job.Kind != queue.KindCollectSource || job.Payload != (queue.Payload{}) {
		t.Fatalf("collection job envelope = %#v", job)
	}
	if job.UniqueKey != CollectionUniqueKey(7, 12, 31, 9, windowStart, now) {
		t.Fatalf("collection unique key = %q", job.UniqueKey)
	}
	args, err := DecodeCollectionJobArgs(job.DurableArgs)
	if err != nil {
		t.Fatalf("DecodeCollectionJobArgs(): %v", err)
	}
	want := CollectionJobArgs{
		MonitorID: 7, MonitorVersionID: 12, CompiledProfileID: 31, SourceConnectionID: 9,
		WindowStart: windowStart, WindowEnd: now, InputHash: strings.Repeat("a", 64), TriggerType: "schedule",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("collection job args = %#v, want %#v", args, want)
	}
	var wire map[string]any
	if err := json.Unmarshal(job.DurableArgs, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire) != 8 {
		t.Fatalf("collection job leaked non-identity fields: %#v", wire)
	}
	for _, key := range []string{"monitor_id", "monitor_version_id", "compiled_profile_id", "source_connection_id", "window_start", "window_end", "input_hash", "trigger_type"} {
		if _, ok := wire[key]; !ok {
			t.Fatalf("collection job args missing %q: %#v", key, wire)
		}
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
	firstKey := ManualCollectionUniqueKey(7, 12, 31, 9, first)
	if secondKey := ManualCollectionUniqueKey(7, 12, 31, 9, second); secondKey != firstKey {
		t.Fatalf("same cooldown bucket keys differ: %q != %q", secondKey, firstKey)
	}
	if nextKey := ManualCollectionUniqueKey(7, 12, 31, 9, next); nextKey == firstKey {
		t.Fatal("next cooldown bucket reused the previous key")
	}
	if got := ManualCollectionCooldownUntil(first); !got.Equal(time.Date(2026, 7, 17, 9, 5, 0, 0, time.UTC)) {
		t.Fatalf("cooldown until = %s", got)
	}
}

func TestCollectionJobArgsRejectUnknownOrTrailingFields(t *testing.T) {
	valid, err := EncodeCollectionJobArgs(CollectionJobArgs{
		MonitorID: 7, MonitorVersionID: 12, CompiledProfileID: 31, SourceConnectionID: 9,
		WindowStart: time.Date(2026, 7, 17, 8, 55, 0, 0, time.UTC), WindowEnd: time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC),
		InputHash: strings.Repeat("a", 64), TriggerType: "schedule",
	})
	if err != nil {
		t.Fatal(err)
	}
	unknown := append([]byte(nil), valid[:len(valid)-1]...)
	unknown = append(unknown, []byte(`,"body":"secret"}`)...)
	for _, encoded := range [][]byte{unknown, append(append([]byte(nil), valid...), []byte(` {}`)...)} {
		if _, err := DecodeCollectionJobArgs(encoded); err == nil {
			t.Fatalf("unsafe collection args accepted: %s", encoded)
		}
	}
}
