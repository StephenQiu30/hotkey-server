package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/aggregator"
	"github.com/StephenQiu30/hotkey-server/internal/cleanup"
	"github.com/StephenQiu30/hotkey-server/internal/collector"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/platform/baidu"
	"github.com/StephenQiu30/hotkey-server/internal/platform/weibo"
	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
	"github.com/StephenQiu30/hotkey-server/internal/platform/zhihu"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/connector"
	"gorm.io/gorm"
)

// newJobRunner creates the job runner and registers all jobs.
//
// Each job group registers itself via a dedicated register* function,
// making it easy to add or remove job groups without modifying other registrations.
func newJobRunner(cfg config.Config, db *gorm.DB) *jobs.Runner {
	runner := jobs.NewRunner(jobs.WithRetryPolicy(jobs.RetryPolicy{MaxAttempts: 3, Backoff: time.Second}))

	registerPollMonitor(runner, cfg, db)
	registerAggregateTopics(runner, cfg, db)
	registerBuildSnapshots(runner, cfg, db)
	registerDispatchNotifications(runner, cfg, db)
	registerPublishDailyTopics(runner, cfg, db)
	registerTrendingCollector(runner, cfg, db)
	registerEventAggregator(runner, cfg, db)
	registerCleanup(runner, cfg, db)

	return runner
}

// registerPollMonitor registers the X keyword search polling job.
func registerPollMonitor(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	xClient := x.NewClient(cfg.XToken, cfg.XBaseURL)
	connector := jobs.NewXConnectorAdapter(xClient, cfg.XToken)

	hitScorerRepo := database.NewHitScoreRepo(db)
	scoringSvc := scoring.NewService(hitScorerRepo)
	scorer := jobs.NewScorerAdapter(scoringSvc)

	runRepo := database.NewRunRepo(db)
	postRepo := database.NewPollPostRepo(db)
	hitRepo := database.NewPollHitRepo(db)
	pollJob := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, scorer)

	monitorRepo := database.NewMonitorRepo(db)

	runner.Register("poll_monitor", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "poll_monitor: running"))
		monitorIDs, err := monitorRepo.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		return runMonitorJob(ctx, "poll_monitor", monitorIDs, func(ctx context.Context, monitorID int64) error {
			if err := pollJob.Run(ctx, jobs.MonitorInfo{ID: monitorID, Platform: "x"}); err != nil {
				log.Printf("poll_monitor: error for monitor %d: %v", monitorID, err)
				return err
			}
			return nil
		})
	}, 1*time.Minute, minuteRunKey("poll_monitor"))
}

// registerAggregateTopics registers the X topic clustering job.
func registerAggregateTopics(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	topicRepo := database.NewTopicRepo(db)
	jobQuery := database.NewJobQueryRepo(db)
	aggregateJob := jobs.NewAggregateTopicsJob(jobQuery, topicRepo)

	monitorRepo := database.NewMonitorRepo(db)

	runner.Register("aggregate_topics", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_topics: running"))
		monitorIDs, err := monitorRepo.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		return runMonitorJob(ctx, "aggregate_topics", monitorIDs, func(_ context.Context, monitorID int64) error {
			if _, err := aggregateJob.Run(jobs.AggregateTopicsInput{MonitorID: monitorID, RunTime: time.Now()}); err != nil {
				log.Printf("aggregate_topics: error for monitor %d: %v", monitorID, err)
				return err
			}
			return nil
		})
	}, 5*time.Minute, minuteRunKey("aggregate_topics"))
}

// registerBuildSnapshots registers the trend snapshot job.
func registerBuildSnapshots(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	trendRepo := database.NewTrendRepo(db)
	trendSvc := trend.NewService(trendRepo)
	jobQuery := database.NewJobQueryRepo(db)
	snapshotJob := jobs.NewBuildSnapshotsJob(trendSvc, jobQuery)

	monitorRepo := database.NewMonitorRepo(db)

	runner.Register("build_snapshots", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "build_snapshots: running"))
		snapshotTime := time.Now()
		if startedAt, ok := jobs.RunStartedAtFromContext(ctx); ok {
			snapshotTime = startedAt
		}
		monitorIDs, err := monitorRepo.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		return runMonitorJob(ctx, "build_snapshots", monitorIDs, func(_ context.Context, monitorID int64) error {
			if _, err := snapshotJob.Run(jobs.BuildSnapshotsInput{MonitorID: monitorID, SnapshotTime: snapshotTime}); err != nil {
				log.Printf("build_snapshots: error for monitor %d: %v", monitorID, err)
				return err
			}
			return nil
		})
	}, 10*time.Minute, minuteRunKey("build_snapshots"))
}

// registerDispatchNotifications registers the notification dispatch job.
func registerDispatchNotifications(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	deliveryRepo := database.NewDeliveryRepo(db)
	mailer := &noopMailer{}
	dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, deliveryRepo)

	runner.Register("dispatch_notifications", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
		return dispatchJob.RunPending(ctx, 100)
	}, 1*time.Minute, minuteRunKey("dispatch_notifications"))
}

// registerPublishDailyTopics registers the Obsidian daily digest job.
func registerPublishDailyTopics(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	if cfg.ObsidianVaultPath == "" {
		log.Print("worker: publish_daily_topics disabled (OBSIDIAN_VAULT_PATH not set)")
		return
	}

	exporter := database.NewExporter(db)
	llmClient := llm.NewOpenAIClient(llm.OpenAIConfig{
		APIKey:  cfg.LLMAPIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.LLMModel,
	})
	writer := &jobs.DefaultVaultWriter{}

	monitorRepo := database.NewMonitorRepo(db)
	monitorIDs, err := monitorRepo.ListActiveIDs(context.Background())
	if err != nil {
		log.Printf("worker: failed to list monitors: %v", err)
		return
	}

	for _, monitorID := range monitorIDs {
		monitorCfg := jobs.MonitorConfig{
			ID:   monitorID,
			Name: "Monitor",
			Slug: "monitor",
		}

		digestSvc := digest.NewService(database.NewDigestQueryService(db))
		publishJob := jobs.NewPublishDailyTopicsJobWithDelegate(
			digestSvc,
			llmClient,
			exporter,
			writer,
			cfg.ObsidianVaultPath,
			monitorCfg,
		)

		runner.Register("publish_daily_topics", func(ctx context.Context) error {
			log.Print(observability.RenderLog("worker", "publish_daily_topics: running"))
			_, err := publishJob.Run(ctx, time.Now(), cfg.DailyDigestTarget)
			return err
		}, 1*time.Minute, dailyRunKey(fmt.Sprintf("publish_daily_topics:%d", monitorID)))
	}
}

// registerTrendingCollector registers the multi-platform trending collector job.
func registerTrendingCollector(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	weiboClient := weibo.NewClient(10 * time.Second)
	zhihuClient := zhihu.NewClient(10 * time.Second)
	baiduClient := baidu.NewClient(15 * time.Second)

	collectors := []connector.TrendingCollector{
		weiboClient,
		zhihuClient,
		baiduClient,
	}

	collector.Register(runner, db, collectors)
}

// registerEventAggregator registers the hot event aggregation job.
func registerEventAggregator(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	eventRepo := database.NewHotEventRepo(db)
	eventSvc := hotevent.NewService(eventRepo)

	aggregator.Register(runner, db, eventSvc)
}

// registerCleanup registers the data cleanup job.
func registerCleanup(runner *jobs.Runner, cfg config.Config, db *gorm.DB) {
	eventRepo := database.NewHotEventRepo(db)
	eventSvc := hotevent.NewService(eventRepo)

	cleanup.Register(runner, db, eventSvc)
}

// dailyRunKey returns a JobOption that scopes the run key to a day.
func dailyRunKey(name string) jobs.JobOption {
	return jobs.WithRunKey(func(now time.Time) string {
		return fmt.Sprintf("%s:%s", name, now.Format("2006-01-02"))
	})
}

// minuteRunKey returns a JobOption that scopes the run key to a minute.
func minuteRunKey(name string) jobs.JobOption {
	return jobs.WithRunKey(func(now time.Time) string {
		return fmt.Sprintf("%s:%s", name, now.Format("2006-01-02T15:04"))
	})
}

// runMonitorJob iterates over monitorIDs and runs the given function for each.
func runMonitorJob(ctx context.Context, jobName string, monitorIDs []int64, run func(context.Context, int64) error) error {
	var errs []error
	for _, monitorID := range monitorIDs {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("%s monitor %d: %w", jobName, monitorID, err))
			break
		}
		if err := run(ctx, monitorID); err != nil {
			errs = append(errs, fmt.Errorf("%s monitor %d: %w", jobName, monitorID, err))
		}
	}
	return errors.Join(errs...)
}

type noopMailer struct{}

func (m *noopMailer) Send(_ context.Context, to, subject, _ string) (string, error) {
	log.Printf("mailer: would send to %s subject=%q (noop)", to, subject)
	return "noop-msg-id", nil
}
