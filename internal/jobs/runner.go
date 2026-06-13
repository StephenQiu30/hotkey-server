package jobs

import (
	"context"
	"log"
	"time"
)

// JobFunc is a function executed by the runner on each tick.
type JobFunc func(ctx context.Context) error

// registeredJob holds a job's execution function and interval.
type registeredJob struct {
	name     string
	run      JobFunc
	interval time.Duration
}

// Runner periodically executes registered background jobs.
type Runner struct {
	jobs []registeredJob
}

// NewRunner creates a new Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Register adds a job to be executed at the given interval.
func (r *Runner) Register(name string, fn JobFunc, interval time.Duration) {
	r.jobs = append(r.jobs, registeredJob{name: name, run: fn, interval: interval})
}

// Run starts all registered jobs and blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	for _, j := range r.jobs {
		go r.loop(ctx, j)
	}
	<-ctx.Done()
}

func (r *Runner) loop(ctx context.Context, j registeredJob) {
	log.Printf("job %s: started (interval %s)", j.name, j.interval)
	if err := j.run(ctx); err != nil {
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
			if err := j.run(ctx); err != nil {
				log.Printf("job %s: error: %v", j.name, err)
			}
		}
	}
}
