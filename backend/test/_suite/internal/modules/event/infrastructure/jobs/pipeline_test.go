package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	eventapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/application"
	eventdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/event/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func TestEventHandlersRejectWrongKinds(t *testing.T) {
	job := queue.Job{Kind: queue.KindEvaluateRelevance, UniqueKey: "x", ScheduledAt: time.Now().UTC(), MaxAttempts: 1, Priority: 1, Payload: queue.Payload{EntityID: 1, EntityVersion: 1}}
	if err := (&ClusterHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("cluster wrong kind = %v", err)
	}
	if err := (&HeatHandler{}).Handle(context.Background(), job); !queue.IsPermanent(err) {
		t.Fatalf("heat wrong kind = %v", err)
	}
}

func TestHeatHandlerFansOutActionableUpdateBeforeSummary(t *testing.T) {
	job := heatPipelineJob()
	heat := &heatRecomputerFake{results: []eventdomain.HeatResult{
		{EventID: job.Payload.EntityID, WindowHours: 1},
		heatPipelineResult(job.Payload.EntityID, job.Payload.WindowEnd),
	}}
	update := &eventdomain.EventUpdate{ID: 81, Version: 1, EventID: job.Payload.EntityID, SequenceNo: 4, Kind: eventdomain.EventUpdateRising, IdempotencyKey: strings.Repeat("c", 64), EvidenceSetHash: strings.Repeat("a", 64)}
	updates := &updateRecorderFake{update: update, created: false}
	jobs := newJobEnqueuerFake()
	handler, err := NewHeatHandler(heat, updates, jobs)
	if err != nil {
		t.Fatalf("NewHeatHandler() error = %v", err)
	}
	if err := handler.Handle(context.Background(), job); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if updates.calls != 1 || updates.heat.WindowHours != 24 {
		t.Fatalf("UpdateRecorder.Record() = %d calls with %#v", updates.calls, updates.heat)
	}
	if len(jobs.attempts) != 2 || jobs.attempts[0].Kind != queue.KindEvaluateEventAlerts || jobs.attempts[1].Kind != queue.KindGenerateEventSummary {
		t.Fatalf("fan-out order = %#v, want alert then summary", jobKinds(jobs.attempts))
	}
	alert := jobs.attempts[0]
	if alert.Payload.EntityID != update.ID || alert.Payload.EntityVersion != update.Version || alert.Payload.InputHash == "" || alert.UniqueKey == "" {
		t.Fatalf("alert job = %#v, want bounded EventUpdate identity", alert)
	}
	summary := jobs.attempts[1]
	if summary.Payload.EntityID != job.Payload.EntityID || summary.Payload.EntityVersion != job.Payload.EntityVersion || summary.UniqueKey == "" {
		t.Fatalf("summary job = %#v, want Event identity", summary)
	}
}

func TestHeatHandlerOnlyEnqueuesSummaryWithoutActionableUpdate(t *testing.T) {
	tests := []struct {
		name   string
		update *eventdomain.EventUpdate
	}{
		{name: "no material update"},
		{name: "cooling update", update: &eventdomain.EventUpdate{ID: 82, Version: 1, EventID: 7, SequenceNo: 5, Kind: eventdomain.EventUpdateCooling, IdempotencyKey: strings.Repeat("d", 64)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := heatPipelineJob()
			heat := &heatRecomputerFake{results: []eventdomain.HeatResult{heatPipelineResult(job.Payload.EntityID, job.Payload.WindowEnd)}}
			updates := &updateRecorderFake{update: test.update, created: test.update != nil}
			jobs := newJobEnqueuerFake()
			handler, err := NewHeatHandler(heat, updates, jobs)
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.Handle(context.Background(), job); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if len(jobs.successful) != 1 || jobs.successful[0].Kind != queue.KindGenerateEventSummary {
				t.Fatalf("successful jobs = %#v, want one summary", jobKinds(jobs.successful))
			}
		})
	}
}

func TestHeatHandlerRetryCompletesFanOutWithoutDuplicateFacts(t *testing.T) {
	for _, failedKind := range []string{queue.KindEvaluateEventAlerts, queue.KindGenerateEventSummary} {
		t.Run(failedKind, func(t *testing.T) {
			job := heatPipelineJob()
			heat := &heatRecomputerFake{results: []eventdomain.HeatResult{heatPipelineResult(job.Payload.EntityID, job.Payload.WindowEnd)}}
			updates := &updateRecorderFake{update: &eventdomain.EventUpdate{ID: 83, Version: 1, EventID: job.Payload.EntityID, SequenceNo: 2, Kind: eventdomain.EventUpdateMetricChanged, IdempotencyKey: strings.Repeat("e", 64)}}
			jobs := newJobEnqueuerFake()
			jobs.failKind, jobs.failuresRemaining = failedKind, 1
			handler, err := NewHeatHandler(heat, updates, jobs)
			if err != nil {
				t.Fatal(err)
			}
			if err := handler.Handle(context.Background(), job); !queue.IsRetryable(err) {
				t.Fatalf("first Handle() error = %v, want retryable", err)
			}
			if err := handler.Handle(context.Background(), job); err != nil {
				t.Fatalf("retry Handle() error = %v", err)
			}
			if heat.calls != 2 || updates.calls != 2 {
				t.Fatalf("retry calls = heat %d/update %d, want 2/2", heat.calls, updates.calls)
			}
			if countJobsByKind(jobs.successful, queue.KindEvaluateEventAlerts) != 1 || countJobsByKind(jobs.successful, queue.KindGenerateEventSummary) != 1 {
				t.Fatalf("successful jobs = %#v, want one alert and one summary", jobKinds(jobs.successful))
			}
		})
	}
}

type heatRecomputerFake struct {
	results []eventdomain.HeatResult
	err     error
	calls   int
}

func (fake *heatRecomputerFake) RecomputeEventMetrics(_ context.Context, _ eventapplication.MetricRecomputeCommand) ([]eventdomain.HeatResult, error) {
	fake.calls++
	return fake.results, fake.err
}

type updateRecorderFake struct {
	heat    eventdomain.HeatResult
	update  *eventdomain.EventUpdate
	created bool
	err     error
	calls   int
}

func (fake *updateRecorderFake) Record(_ context.Context, heat eventdomain.HeatResult) (*eventdomain.EventUpdate, bool, error) {
	fake.calls++
	fake.heat = heat
	return fake.update, fake.created, fake.err
}

type jobEnqueuerFake struct {
	attempts          []queue.Job
	successful        []queue.Job
	seen              map[string]int64
	failKind          string
	failuresRemaining int
}

func newJobEnqueuerFake() *jobEnqueuerFake { return &jobEnqueuerFake{seen: make(map[string]int64)} }

func (fake *jobEnqueuerFake) Enqueue(_ context.Context, job queue.Job) (int64, bool, error) {
	fake.attempts = append(fake.attempts, job)
	if fake.failKind == job.Kind && fake.failuresRemaining > 0 {
		fake.failuresRemaining--
		return 0, false, errors.New("queue unavailable")
	}
	identity := job.Kind + ":" + job.UniqueKey
	if id, found := fake.seen[identity]; found {
		return id, false, nil
	}
	id := int64(len(fake.seen) + 1)
	fake.seen[identity] = id
	fake.successful = append(fake.successful, job)
	return id, true, nil
}

func heatPipelineJob() queue.Job {
	windowEnd := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	return queue.Job{
		Kind: queue.KindRecomputeEventHeat, UniqueKey: "heat-7", ScheduledAt: windowEnd, MaxAttempts: 3, Priority: 5,
		Payload: queue.Payload{EntityID: 7, EntityVersion: 3, WindowStart: windowEnd.Add(-24 * time.Hour), WindowEnd: windowEnd, InputHash: strings.Repeat("f", 64)},
	}
}

func heatPipelineResult(eventID int64, observedAt time.Time) eventdomain.HeatResult {
	return eventdomain.HeatResult{
		EventID: eventID, HeatScore: 75, TrendScore: 20, TrendStatus: eventdomain.TrendRising,
		SourceCount: 3, ContentCount: 5, HeatVersion: eventdomain.HeatAlgorithmVersionV1,
		EvidenceSetHash: strings.Repeat("a", 64), CapabilityProfileSetHash: strings.Repeat("b", 64),
		WindowHours: 24, WindowEnd: observedAt,
	}
}

func jobKinds(jobs []queue.Job) []string {
	result := make([]string, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, job.Kind)
	}
	return result
}

func countJobsByKind(jobs []queue.Job, kind string) int {
	count := 0
	for _, job := range jobs {
		if job.Kind == kind {
			count++
		}
	}
	return count
}
