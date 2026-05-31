package admin

import (
	"context"
	"database/sql"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type MemoryRepository struct {
	mu        sync.RWMutex
	auditLogs []AuditLog
	jobs      map[string]queue.Job
	jobOrder  []string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{jobs: make(map[string]queue.Job)}
}

func (r *MemoryRepository) CreateAuditLog(_ context.Context, entry AuditLog) (AuditLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditLogs = append(r.auditLogs, entry)
	return entry, nil
}

func (r *MemoryRepository) ListAuditLogs(_ context.Context) ([]AuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	logs := make([]AuditLog, len(r.auditLogs))
	copy(logs, r.auditLogs)
	return logs, nil
}

func (r *MemoryRepository) CreateJob(_ context.Context, job queue.Job) (queue.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
	r.jobOrder = append(r.jobOrder, job.ID)
	return job, nil
}

func (r *MemoryRepository) ListJobs(_ context.Context) ([]queue.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	jobs := make([]queue.Job, 0, len(r.jobOrder))
	for _, id := range r.jobOrder {
		jobs = append(jobs, r.jobs[id])
	}
	return jobs, nil
}

func (r *MemoryRepository) JobByID(_ context.Context, jobID string) (queue.Job, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, exists := r.jobs[jobID]
	if !exists {
		return queue.Job{}, sql.ErrNoRows
	}
	return job, nil
}

func (r *MemoryRepository) UpdateJob(_ context.Context, job queue.Job) (queue.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.jobs[job.ID]; !exists {
		return queue.Job{}, sql.ErrNoRows
	}
	r.jobs[job.ID] = job
	return job, nil
}
