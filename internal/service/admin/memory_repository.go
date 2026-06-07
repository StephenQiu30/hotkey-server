package admin

import (
	"context"
	"sync"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

type UserRecord struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	Status       string
}

type RSSFeedRecord struct {
	UserID    string
	TokenHash string
	Enabled   bool
}

type DailyReportRecord struct {
	ID     string
	UserID string
	Date   string
}

type MemoryRepository struct {
	mu            sync.RWMutex
	auditLogs     []AuditLog
	jobs          map[string]queue.Job
	jobOrder      []string
	users         map[string]UserRecord
	rssFeeds      map[string]RSSFeedRecord
	dailyReports  []DailyReportRecord
	cleanupTasks  map[string]CleanupTask
	deleteReportErr error
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		jobs:         make(map[string]queue.Job),
		users:        make(map[string]UserRecord),
		rssFeeds:     make(map[string]RSSFeedRecord),
		cleanupTasks: make(map[string]CleanupTask),
	}
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
	filtered := make([]DailyReportRecord, 0, len(r.dailyReports))
	for _, rpt := range r.dailyReports {
		if rpt.UserID != userID {
			filtered = append(filtered, rpt)
		}
	}
	r.dailyReports = filtered
	return nil
}

func (r *MemoryRepository) SaveCleanupTask(_ context.Context, task CleanupTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanupTasks[task.ID] = task
	return nil
}

func (r *MemoryRepository) CleanupTaskByID(_ context.Context, taskID string) (CleanupTask, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, exists := r.cleanupTasks[taskID]
	if !exists {
		return CleanupTask{}, ErrNotFound
	}
	return task, nil
}
