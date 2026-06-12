package jobs

import (
	"context"
	"fmt"
	"time"
)

// MonitorRun represents a single execution of a monitor poll job.
type MonitorRun struct {
	ID            int64
	MonitorID     int64
	Platform      string
	RunType       string
	Status        string
	StartedAt     time.Time
	FinishedAt    *time.Time
	FetchedCount  int
	StoredCount   int
	ErrorMessage  string
}

// MonitorInfo contains the configuration for a monitor to poll.
type MonitorInfo struct {
	ID        int64
	Platform  string
	QueryText string
	Keywords  []string
}

// PostResult represents a post fetched from a platform connector.
type PostResult struct {
	ID           string
	AuthorID     string
	AuthorName   string
	AuthorHandle string
	Text         string
	Language     string
	PublishedAt  time.Time
	LikeCount    int
	ReplyCount   int
	RepostCount  int
	QuoteCount   int
	ViewCount    int
}

// HitResult represents a monitor-post hit relationship.
type HitResult struct {
	MonitorID int64
	PostID    int64
}

// RunRepository persists monitor run records.
type RunRepository interface {
	CreateRun(ctx context.Context, run MonitorRun) error
	UpdateRunStatus(ctx context.Context, runID int64, status string, errMsg string) error
}

// PostRepository persists platform posts.
type PostRepository interface {
	UpsertPost(ctx context.Context, post PostResult) error
}

// HitRepository persists monitor-post hits.
type HitRepository interface {
	UpsertHit(ctx context.Context, hit HitResult) error
}

// PlatformConnector fetches posts from an external platform.
type PlatformConnector interface {
	SearchPosts(ctx context.Context, query string, cursor string) ([]PostResult, string, error)
}

// PollMonitorJob orchestrates a single monitor poll cycle.
type PollMonitorJob struct {
	runRepo  RunRepository
	postRepo PostRepository
	hitRepo  HitRepository
	connector PlatformConnector
}

// NewPollMonitorJob creates a new PollMonitorJob.
func NewPollMonitorJob(runRepo RunRepository, postRepo PostRepository, hitRepo HitRepository, connector PlatformConnector) *PollMonitorJob {
	return &PollMonitorJob{
		runRepo:   runRepo,
		postRepo:  postRepo,
		hitRepo:   hitRepo,
		connector: connector,
	}
}

// Run executes a single poll cycle for the given monitor.
func (j *PollMonitorJob) Run(ctx context.Context, monitor MonitorInfo) error {
	run := MonitorRun{
		MonitorID: monitor.ID,
		Platform:  monitor.Platform,
		RunType:   "poll",
		Status:    "running",
		StartedAt: time.Now(),
	}

	if err := j.runRepo.CreateRun(ctx, run); err != nil {
		return fmt.Errorf("create run: %w", err)
	}

	posts, _, err := j.connector.SearchPosts(ctx, monitor.QueryText, "")
	if err != nil {
		_ = j.runRepo.UpdateRunStatus(ctx, run.ID, "failed", err.Error())
		return fmt.Errorf("search posts: %w", err)
	}

	run.FetchedCount = len(posts)
	for _, post := range posts {
		if err := j.postRepo.UpsertPost(ctx, post); err != nil {
			_ = j.runRepo.UpdateRunStatus(ctx, run.ID, "failed", err.Error())
			return fmt.Errorf("upsert post %s: %w", post.ID, err)
		}
		run.StoredCount++
	}

	run.Status = "success"
	if err := j.runRepo.CreateRun(ctx, run); err != nil {
		return fmt.Errorf("update run: %w", err)
	}

	return nil
}
