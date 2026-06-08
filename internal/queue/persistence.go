package queue

import "context"

// JobRepository 定义 job 持久化所需的最小接口。
// 由 PostgreSQL jobrepo.Repository 等实现。
type JobRepository interface {
	Create(ctx context.Context, job Job) error
	UpdateStatus(ctx context.Context, id string, status JobStatus, lastError string, attempt int) error
}

// JobStatePersister 监听 queue 状态变更并同步到持久化存储。
// 作为 StateChangeFunc 注入到 MemoryQueue 或 RedisQueue。
type JobStatePersister struct {
	repo JobRepository
}

func NewJobStatePersister(repo JobRepository) *JobStatePersister {
	return &JobStatePersister{repo: repo}
}

// OnStateChange 实现 StateChangeFunc 签名。
// Enqueue 时调用 Create，其他状态变更调用 UpdateStatus。
func (p *JobStatePersister) OnStateChange(ctx context.Context, job Job) {
	if job.Attempt == 0 && (job.Status == JobStatusPending || job.Status == JobStatusScheduled) {
		_ = p.repo.Create(ctx, job)
		return
	}
	_ = p.repo.UpdateStatus(ctx, job.ID, job.Status, job.LastError, job.Attempt)
}
