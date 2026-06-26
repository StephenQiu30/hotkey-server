package jobs

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// JobFunc is a function executed by the runner on each tick.
type JobFunc func(ctx context.Context) error

// RunStatus is the audited lifecycle status for a job run.
type RunStatus string

const (
	RunStatusStarted   RunStatus = "started"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusSkipped   RunStatus = "skipped"
)

// RunEvent records a traceable job lifecycle transition.
type RunEvent struct {
	JobName    string
	RunKey     string
	Status     RunStatus
	Attempt    int
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Error      string
}

// RunRecorder persists or emits job run audit events.
type RunRecorder interface {
	RecordJobRun(ctx context.Context, event RunEvent) error
}

// LogRunRecorder writes job audit events as JSON lines.
type LogRunRecorder struct {
	out io.Writer
}

// NewLogRunRecorder creates a JSON-line recorder for worker audit events.
func NewLogRunRecorder(out io.Writer) *LogRunRecorder {
	return &LogRunRecorder{out: out}
}

// RecordJobRun writes one traceable job audit event.
func (r *LogRunRecorder) RecordJobRun(_ context.Context, event RunEvent) error {
	body := map[string]any{
		"time":        eventTime(event),
		"module":      "worker",
		"job":         event.JobName,
		"run_key":     event.RunKey,
		"status":      event.Status,
		"attempt":     event.Attempt,
		"duration_ms": event.Duration.Milliseconds(),
	}
	if !event.StartedAt.IsZero() {
		body["started_at"] = event.StartedAt.UTC().Format(time.RFC3339)
	}
	if !event.FinishedAt.IsZero() {
		body["finished_at"] = event.FinishedAt.UTC().Format(time.RFC3339)
	}
	if event.Error != "" {
		body["error"] = event.Error
	}
	return json.NewEncoder(r.out).Encode(body)
}

// RetryPolicy controls how many times a runner invokes a failing job per tick.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// RunnerOption customizes a Runner.
type RunnerOption func(*Runner)

// JobOption customizes one registered job.
type JobOption func(*registeredJob)

// RunKeyFunc builds the idempotency key for one scheduled job execution.
type RunKeyFunc func(now time.Time) string

// JobSpec exposes registered scheduler metadata for verification and ops.
type JobSpec struct {
	Name      string
	Interval  time.Duration
	HasRunKey bool
	runKey    RunKeyFunc
}

// RunKeyAt returns the idempotency key that would be used at now.
func (s JobSpec) RunKeyAt(now time.Time) string {
	if s.runKey == nil {
		return ""
	}
	return s.runKey(now)
}

// registeredJob holds a job's execution function and interval.
type registeredJob struct {
	name     string
	run      JobFunc
	interval time.Duration
	runKey   RunKeyFunc
}

// Runner periodically executes registered background jobs.
type Runner struct {
	jobs        []registeredJob
	recorder    RunRecorder
	retryPolicy RetryPolicy
	mu          sync.Mutex
	succeeded   map[string]struct{}
}

// NewRunner creates a new Runner.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		recorder:    NewLogRunRecorder(os.Stderr),
		retryPolicy: RetryPolicy{MaxAttempts: 1},
		succeeded:   make(map[string]struct{}),
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.retryPolicy.MaxAttempts < 1 {
		r.retryPolicy.MaxAttempts = 1
	}
	return r
}

// WithRunRecorder configures job lifecycle audit recording.
func WithRunRecorder(recorder RunRecorder) RunnerOption {
	return func(r *Runner) {
		r.recorder = recorder
	}
}

// WithRetryPolicy configures per-tick retry behavior for failing jobs.
func WithRetryPolicy(policy RetryPolicy) RunnerOption {
	return func(r *Runner) {
		r.retryPolicy = policy
	}
}

// WithRunKey configures a stable idempotency key for a registered job.
func WithRunKey(fn RunKeyFunc) JobOption {
	return func(j *registeredJob) {
		j.runKey = fn
	}
}

// Register adds a job to be executed at the given interval.
func (r *Runner) Register(name string, fn JobFunc, interval time.Duration, opts ...JobOption) {
	j := registeredJob{name: name, run: fn, interval: interval}
	for _, opt := range opts {
		opt(&j)
	}
	r.jobs = append(r.jobs, j)
}

// JobSpecs returns a read-only snapshot of registered scheduler metadata.
func (r *Runner) JobSpecs() []JobSpec {
	specs := make([]JobSpec, 0, len(r.jobs))
	for _, j := range r.jobs {
		specs = append(specs, JobSpec{
			Name:      j.name,
			Interval:  j.interval,
			HasRunKey: j.runKey != nil,
			runKey:    j.runKey,
		})
	}
	return specs
}

// RetryPolicy returns the configured retry policy.
func (r *Runner) RetryPolicy() RetryPolicy {
	return r.retryPolicy
}

// Run starts all registered jobs and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, j := range r.jobs {
		wg.Add(1)
		go func(job registeredJob) {
			defer wg.Done()
			r.loop(ctx, job)
		}(j)
	}
	<-ctx.Done()
	wg.Wait()
}

func (r *Runner) loop(ctx context.Context, j registeredJob) {
	log.Printf("job %s: started (interval %s)", j.name, j.interval)
	if err := r.runOnce(ctx, j); err != nil {
		log.Printf("job %s: error: %v", j.name, err)
	}

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("job %s: stopped", j.name)
			return
		case <-ticker.C:
			if err := r.runOnce(ctx, j); err != nil {
				log.Printf("job %s: error: %v", j.name, err)
			}
		}
	}
}

func (r *Runner) runOnce(ctx context.Context, j registeredJob) error {
	now := time.Now()
	runKey := j.name + ":" + now.Format("20060102150405")
	if j.runKey != nil {
		runKey = j.runKey(now)
	}
	if j.runKey != nil && r.hasSucceeded(runKey) {
		r.record(ctx, RunEvent{
			JobName:   j.name,
			RunKey:    runKey,
			Status:    RunStatusSkipped,
			StartedAt: now,
		})
		return nil
	}
	started := RunEvent{
		JobName:   j.name,
		RunKey:    runKey,
		Status:    RunStatusStarted,
		Attempt:   1,
		StartedAt: now,
	}
	r.record(ctx, started)

	var err error
	for attempt := 1; attempt <= r.retryPolicy.MaxAttempts; attempt++ {
		err = j.run(ctx)
		if err == nil {
			finished := time.Now()
			r.record(ctx, RunEvent{
				JobName:    j.name,
				RunKey:     runKey,
				Status:     RunStatusSucceeded,
				Attempt:    attempt,
				StartedAt:  now,
				FinishedAt: finished,
				Duration:   finished.Sub(now),
			})
			if j.runKey != nil {
				r.markSucceeded(runKey)
			}
			return nil
		}
		if attempt < r.retryPolicy.MaxAttempts && r.retryPolicy.Backoff > 0 {
			select {
			case <-ctx.Done():
				finished := time.Now()
				r.record(ctx, RunEvent{
					JobName:    j.name,
					RunKey:     runKey,
					Status:     RunStatusFailed,
					Attempt:    attempt,
					StartedAt:  now,
					FinishedAt: finished,
					Duration:   finished.Sub(now),
					Error:      ctx.Err().Error(),
				})
				return ctx.Err()
			case <-time.After(r.retryPolicy.Backoff):
			}
		}
	}

	finished := time.Now()
	r.record(ctx, RunEvent{
		JobName:    j.name,
		RunKey:     runKey,
		Status:     RunStatusFailed,
		Attempt:    r.retryPolicy.MaxAttempts,
		StartedAt:  now,
		FinishedAt: finished,
		Duration:   finished.Sub(now),
		Error:      err.Error(),
	})
	return err
}

func (r *Runner) record(ctx context.Context, event RunEvent) {
	if r.recorder == nil {
		return
	}
	if err := r.recorder.RecordJobRun(ctx, event); err != nil {
		log.Printf("job %s: audit record error: %v", event.JobName, err)
	}
}

func (r *Runner) hasSucceeded(runKey string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.succeeded[runKey]
	return ok
}

func (r *Runner) markSucceeded(runKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.succeeded[runKey] = struct{}{}
}

func eventTime(event RunEvent) string {
	t := event.FinishedAt
	if t.IsZero() {
		t = event.StartedAt
	}
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339)
}
