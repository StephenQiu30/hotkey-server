// Package cleanup implements the data retention and cleanup job.
//
// It periodically removes expired platform_posts and archives
// stale HotEvents according to configurable retention policies.
package cleanup

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"gorm.io/gorm"
)

// CleanupJob handles data retention and cleanup.
type CleanupJob struct {
	db       *gorm.DB
	eventSvc *hotevent.Service
}

// Config defines cleanup retention policies.
type Config struct {
	DataRetentionDays  int `mapstructure:"DATA_RETENTION_DAYS"`
	HotEventArchiveDays int `mapstructure:"HOT_EVENT_ARCHIVE_DAYS"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		DataRetentionDays:   30,
		HotEventArchiveDays: 7,
	}
}

// NewCleanupJob creates a new cleanup job.
func NewCleanupJob(db *gorm.DB, eventSvc *hotevent.Service) *CleanupJob {
	return &CleanupJob{db: db, eventSvc: eventSvc}
}

// Register registers the cleanup job with the runner.
func Register(runner *jobs.Runner, db *gorm.DB, eventSvc *hotevent.Service) {
	job := NewCleanupJob(db, eventSvc)
	runner.Register("cleanup_data", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "cleanup_data: running"))
		return job.Run(ctx)
	}, 1*time.Hour)
}

// Run executes one cleanup cycle.
func (j *CleanupJob) Run(ctx context.Context) error {
	cfg := DefaultConfig()
	repo := j.eventSvc.Repo()

	// 1. Archive stale HotEvents (status -> archived)
	archiveCutoff := time.Now().AddDate(0, 0, -cfg.HotEventArchiveDays)
	archived, err := repo.ArchiveOlderThan(archiveCutoff)
	if err != nil {
		log.Printf("cleanup_data: archive hot events error: %v", err)
	} else {
		log.Printf("cleanup_data: archived %d hot events", archived)
	}

	// 2. Delete old platform_posts not referenced by active HotEvents
	postCutoff := time.Now().AddDate(0, 0, -cfg.DataRetentionDays)
	deleted, err := j.deleteOldPosts(ctx, postCutoff)
	if err != nil {
		return fmt.Errorf("delete old posts: %w", err)
	}
	log.Printf("cleanup_data: deleted %d old platform posts", deleted)

	return nil
}

func (j *CleanupJob) deleteOldPosts(ctx context.Context, cutoff time.Time) (int64, error) {
	result := j.db.WithContext(ctx).Exec(`
		DELETE FROM platform_posts
		WHERE updated_at < ?
		  AND platform IN ('weibo', 'zhihu', 'baidu')
		  AND id NOT IN (
			  SELECT unnest(post_ids) FROM hot_events WHERE status = 'active'
		  )
	`, cutoff)
	return result.RowsAffected, result.Error
}
