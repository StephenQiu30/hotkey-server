// Package collector implements the trending data collection job.
//
// It periodically fetches trending/hot lists from configured platforms
// (weibo, zhihu, baidu) and upserts the results into platform_posts,
// making them available for downstream aggregation and analysis.
package collector

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/connector"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"gorm.io/gorm"
)

// TrendingCollectorJob periodically fetches trending data from all configured
// platforms and stores them in the database.
type TrendingCollectorJob struct {
	collectors []connector.TrendingCollector
	postRepo   PostRepository
}

// PostRepository defines the storage interface needed for trending items.
type PostRepository interface {
	UpsertTrendingPost(ctx context.Context, item connector.TrendingItem, platformID string) error
}

// NewTrendingCollectorJob creates a new trending collector job.
func NewTrendingCollectorJob(collectors []connector.TrendingCollector, postRepo PostRepository) *TrendingCollectorJob {
	return &TrendingCollectorJob{
		collectors: collectors,
		postRepo:   postRepo,
	}
}

// Register registers this job with the job runner.
func Register(runner *jobs.Runner, db *gorm.DB, collectors []connector.TrendingCollector) {
	postRepo := NewTrendingPostRepo(db)
	job := NewTrendingCollectorJob(collectors, postRepo)

	runner.Register("collect_trending", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "collect_trending: running"))
		return job.Run(ctx)
	}, 5*time.Minute)
}

// Run executes one collection cycle across all registered collectors.
func (j *TrendingCollectorJob) Run(ctx context.Context) error {
	var errs []error

	for _, collector := range j.collectors {
		if err := j.collectFrom(ctx, collector); err != nil {
			log.Printf("collect_trending: %s error: %v", collector.Name(), err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("collect_trending: %d platform(s) failed: %v", len(errs), errs[0])
	}
	return nil
}

func (j *TrendingCollectorJob) collectFrom(ctx context.Context, c connector.TrendingCollector) error {
	items, err := c.FetchTrending(ctx)
	if err != nil {
		return fmt.Errorf("%s: fetch: %w", c.Name(), err)
	}

	var stored int
	for _, item := range items {
		if err := j.postRepo.UpsertTrendingPost(ctx, item, c.Name()); err != nil {
			log.Printf("collect_trending: %s: upsert error for %q: %v", c.Name(), item.Title, err)
			continue
		}
		stored++
	}

	log.Printf("collect_trending: %s: fetched %d, stored %d items", c.Name(), len(items), stored)
	return nil
}
