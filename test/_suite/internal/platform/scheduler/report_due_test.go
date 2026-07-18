package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
)

type reportDueReaderFake struct {
	items []ReportSubscription
}

func (fake reportDueReaderFake) ListEnabledReportSubscriptions(context.Context) ([]ReportSubscription, error) {
	return fake.items, nil
}

type reportEnqueuerFake struct {
	jobs []queue.Job
}

func (fake *reportEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.jobs = append(fake.jobs, job)
	return int64(len(fake.jobs)), true, nil
}

func TestCronMatchesFiveFieldSchedule(t *testing.T) {
	at := time.Date(2026, 7, 18, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if !CronMatches("0 9 * * *", at) {
		t.Fatal("daily schedule did not match")
	}
	if CronMatches("1 9 * * *", at) {
		t.Fatal("non-matching minute matched")
	}
}

func TestReportSchedulerRunOnceEnqueuesPerSubscriptionMinute(t *testing.T) {
	store := &reportEnqueuerFake{}
	scheduler := NewReportScheduler(reportDueReaderFake{items: []ReportSubscription{{ID: 7, Version: 3, Timezone: "Asia/Shanghai", Schedule: "0 9 * * *", Enabled: true}}}, store)
	now := time.Date(2026, 7, 18, 1, 0, 32, 0, time.UTC)
	created, err := scheduler.RunOnce(context.Background(), now)
	if err != nil || created != 1 || len(store.jobs) != 1 {
		t.Fatalf("RunOnce() = created=%d jobs=%d err=%v", created, len(store.jobs), err)
	}
	if store.jobs[0].Kind != queue.KindBuildReport || store.jobs[0].Payload.EntityID != 7 || store.jobs[0].Payload.EntityVersion != 3 {
		t.Fatalf("report job = %#v", store.jobs[0])
	}
}
