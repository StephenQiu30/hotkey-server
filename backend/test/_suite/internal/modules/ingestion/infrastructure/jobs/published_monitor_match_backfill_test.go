package jobs

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestPublishedMonitorMatchBackfillSchedulerPersistsOnlyImmutablePublicationIdentity(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	enqueuer := &publishedMonitorBackfillEnqueuerFake{id: 81, created: true}
	scheduler, err := newPublishedMonitorMatchBackfillScheduler(enqueuer, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := scheduler.SchedulePublishedMonitorMatchBackfill(context.Background(), ingestionapplication.SchedulePublishedMonitorMatchBackfillCommand{
		MonitorID: 7, MonitorVersionID: 31, CompiledProfileID: 51,
	})
	if err != nil {
		t.Fatalf("SchedulePublishedMonitorMatchBackfill(): %v", err)
	}
	if result.JobID != 81 || !result.Created || result.MonitorVersionID != 31 {
		t.Fatalf("result = %#v", result)
	}
	if enqueuer.job.Kind != queue.KindBackfillPublishedMonitorMatches || enqueuer.job.UniqueKey != PublishedMonitorMatchBackfillUniqueKey(31, 51) ||
		!enqueuer.job.ScheduledAt.Equal(now) {
		t.Fatalf("job = %#v", enqueuer.job)
	}
	var args map[string]any
	if err := json.Unmarshal(enqueuer.job.DurableArgs, &args); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"compiled_profile_id", "monitor_id", "monitor_version_id", "trace_id"}
	gotKeys := make([]string, 0, len(args))
	for key := range args {
		gotKeys = append(gotKeys, key)
	}
	// The explicit reflection assertion prevents accidental body/object-key
	// additions even if JSON field order changes.
	if reflect.TypeOf(publishedMonitorMatchBackfillJobArgs{}).NumField() != len(wantKeys) || len(gotKeys) != len(wantKeys) {
		t.Fatalf("durable args keys = %#v", args)
	}
	for _, key := range wantKeys {
		if _, ok := args[key]; !ok {
			t.Fatalf("durable args missing %q: %#v", key, args)
		}
	}
}

func TestPublishedMonitorMatchBackfillDecoderRejectsUnknownOrTrailingFields(t *testing.T) {
	valid := []byte(`{"monitor_id":7,"monitor_version_id":31,"compiled_profile_id":51,"trace_id":""}`)
	if _, err := decodePublishedMonitorMatchBackfillJobArgs(valid); err != nil {
		t.Fatalf("valid args: %v", err)
	}
	for _, encoded := range [][]byte{
		[]byte(`{"monitor_id":7,"monitor_version_id":31,"compiled_profile_id":51,"trace_id":"","body":"secret"}`),
		append(append([]byte(nil), valid...), []byte(` {}`)...),
	} {
		if _, err := decodePublishedMonitorMatchBackfillJobArgs(encoded); err == nil {
			t.Fatalf("unsafe args accepted: %s", encoded)
		}
	}
}

type publishedMonitorBackfillEnqueuerFake struct {
	job     queue.Job
	id      int64
	created bool
	err     error
}

func (enqueuer *publishedMonitorBackfillEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	enqueuer.job = job
	return enqueuer.id, enqueuer.created, enqueuer.err
}
