package admin

import (
	"context"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type MemoryRepository struct {
	mu              sync.RWMutex
	auditLogs       []AuditLog
	jobs            map[string]queue.Job
	jobOrder        []string
	users           map[string]UserRecord
	rssFeeds        map[string]RSSFeedRecord
	dailyReports    map[string]DailyReportRecord
	cleanupTasks    map[string]CleanupTask
	deleteReportErr error
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		jobs:         make(map[string]queue.Job),
		users:        make(map[string]UserRecord),
		rssFeeds:     make(map[string]RSSFeedRecord),
		dailyReports: make(map[string]DailyReportRecord),
		cleanupTasks: make(map[string]CleanupTask),
	}
}

// SetUser adds a user record for testing.
func (r *MemoryRepository) SetUser(id string, u UserRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[id] = u
}

// SetRSSFeed adds an RSS feed record for testing.
func (r *MemoryRepository) SetRSSFeed(userID string, f RSSFeedRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rssFeeds[userID] = f
}

// SetDailyReport adds a daily report record for testing.
func (r *MemoryRepository) SetDailyReport(id string, d DailyReportRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dailyReports[id] = d
}

// SetDeleteReportError configures an injected error for DeleteDailyReportsByUser.
func (r *MemoryRepository) SetDeleteReportError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deleteReportErr = err
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
		return queue.Job{}, ErrNotFound
	}
	return job, nil
}

func (r *MemoryRepository) UpdateJob(_ context.Context, job queue.Job) (queue.Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.jobs[job.ID]; !exists {
		return queue.Job{}, ErrNotFound
	}
	r.jobs[job.ID] = job
	return job, nil
}

func (r *MemoryRepository) UserByID(_ context.Context, userID string) (UserRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, exists := r.users[userID]
	if !exists {
		return UserRecord{}, ErrNotFound
	}
	return u, nil
}

func (r *MemoryRepository) DeleteUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[userID]; !exists {
		return ErrNotFound
	}
	delete(r.users, userID)
	return nil
}

func (r *MemoryRepository) DeleteRSSFeedByUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rssFeeds, userID)
	return nil
}

func (r *MemoryRepository) DeleteDailyReportsByUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteReportErr != nil {
		return r.deleteReportErr
	}
	filtered := make(map[string]DailyReportRecord)
	for id, rpt := range r.dailyReports {
		if rpt.UserID != userID {
			filtered[id] = rpt
		}
	}
	r.dailyReports = filtered
	return nil
}

// SaveCleanupTask stores a cleanup task with defensive copies of Steps.
func (r *MemoryRepository) SaveCleanupTask(_ context.Context, task CleanupTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	taskCopy := task
	taskCopy.Steps = append([]CleanupStep(nil), task.Steps...)
	r.cleanupTasks[task.ID] = taskCopy
	return nil
}

// CleanupTaskByID returns a cleanup task with defensive copies of Steps.
func (r *MemoryRepository) CleanupTaskByID(_ context.Context, taskID string) (CleanupTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, exists := r.cleanupTasks[taskID]
	if !exists {
		return CleanupTask{}, ErrNotFound
	}
	taskCopy := task
	taskCopy.Steps = append([]CleanupStep(nil), task.Steps...)
	return taskCopy, nil
}
