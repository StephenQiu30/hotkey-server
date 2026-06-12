package jobs

import (
	"context"
	"fmt"
	"time"
)

// MonitorRun represents a single execution of a monitor poll job.
type MonitorRun struct {
	ID           int64
	MonitorID    int64
	Platform     string
	RunType      string
	Status       string
	StartedAt    time.Time
	FinishedAt   *time.Time
	FetchedCount int
	StoredCount  int
	ErrorMessage string
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
	MonitorID      int64
	PostID         int64
	MatchedKeywords []string
}

// RunRepository persists monitor run records.
type RunRepository interface {
	CreateRun(ctx context.Context, run MonitorRun) (int64, error)
	UpdateRun(ctx context.Context, runID int64, run MonitorRun) error
}

// PostRepository persists platform posts.
type PostRepository interface {
	UpsertPost(ctx context.Context, post PostResult) (int64, error)
}

// HitRepository persists monitor-post hits.
type HitRepository interface {
	UpsertHit(ctx context.Context, hit HitResult) error
}

// PlatformConnector fetches posts from an external platform.
type PlatformConnector interface {
	SearchPosts(ctx context.Context, query string, cursor string) ([]PostResult, string, error)
}

// HitScorer computes and persists scores for a monitor-post hit.
type HitScorer interface {
	ScoreHit(hitID int64, post PostResult, matchedKeywords []string, totalKeywords int, publishedMinutesAgo float64) error
}

// PollMonitorJob orchestrates a single monitor poll cycle.
type PollMonitorJob struct {
	runRepo   RunRepository
	postRepo  PostRepository
	hitRepo   HitRepository
	connector PlatformConnector
	scorer    HitScorer
}

// NewPollMonitorJob creates a new PollMonitorJob.
func NewPollMonitorJob(runRepo RunRepository, postRepo PostRepository, hitRepo HitRepository, connector PlatformConnector, scorer HitScorer) *PollMonitorJob {
	return &PollMonitorJob{
		runRepo:   runRepo,
		postRepo:  postRepo,
		hitRepo:   hitRepo,
		connector: connector,
		scorer:    scorer,
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

	runID, err := j.runRepo.CreateRun(ctx, run)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}

	posts, _, err := j.connector.SearchPosts(ctx, monitor.QueryText, "")
	if err != nil {
		run.Status = "failed"
		run.ErrorMessage = err.Error()
		_ = j.runRepo.UpdateRun(ctx, runID, run)
		return fmt.Errorf("search posts: %w", err)
	}

	run.FetchedCount = len(posts)
	for _, post := range posts {
		postID, err := j.postRepo.UpsertPost(ctx, post)
		if err != nil {
			run.Status = "failed"
			run.ErrorMessage = err.Error()
			_ = j.runRepo.UpdateRun(ctx, runID, run)
			return fmt.Errorf("upsert post %s: %w", post.ID, err)
		}
		run.StoredCount++

		if err := j.hitRepo.UpsertHit(ctx, HitResult{
			MonitorID:      monitor.ID,
			PostID:         postID,
			MatchedKeywords: monitor.Keywords,
		}); err != nil {
			run.Status = "failed"
			run.ErrorMessage = err.Error()
			_ = j.runRepo.UpdateRun(ctx, runID, run)
			return fmt.Errorf("upsert hit for post %s: %w", post.ID, err)
		}

		// Score the hit if scorer is available
		if j.scorer != nil {
			publishedMinutesAgo := time.Since(post.PublishedAt).Minutes()
			if err := j.scorer.ScoreHit(postID, post, monitor.Keywords, len(monitor.Keywords), publishedMinutesAgo); err != nil {
				// Log but don't fail the run for scoring errors
				run.ErrorMessage = fmt.Sprintf("score warning for post %s: %v", post.ID, err)
			}
		}
	}

	now := time.Now()
	run.Status = "success"
	run.FinishedAt = &now
	if err := j.runRepo.UpdateRun(ctx, runID, run); err != nil {
		return fmt.Errorf("update run: %w", err)
	}

	return nil
}
