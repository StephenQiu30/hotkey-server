package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/digest"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/platform/x"
	"github.com/StephenQiu30/hotkey-server/internal/scoring"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newJobRunner(cfg config.Config, sqlDB *sql.DB) *jobs.Runner {
	xClient := x.NewClient(cfg.XToken, cfg.XBaseURL)
	connector := jobs.NewXConnectorAdapter(xClient, cfg.XToken)

	hitScorerRepo := jobs.NewDBHitScorerRepo(sqlDB)
	scoringSvc := scoring.NewService(hitScorerRepo)
	scorer := jobs.NewScorerAdapter(scoringSvc)

	runRepo := jobs.NewDBRunRepository(sqlDB)
	postRepo := jobs.NewDBPostRepository(sqlDB)
	hitRepo := jobs.NewDBHitRepository(sqlDB)
	pollJob := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, scorer)

	gdb := gormDBFromSQL(sqlDB)
	topicRepo := database.NewTopicRepo(gdb)
	postCandidateProvider := jobs.NewDBPostCandidateProvider(sqlDB)
	topicPersister := jobs.NewTopicPersisterAdapter(topicRepo)
	aggregateJob := jobs.NewAggregateTopicsJob(postCandidateProvider, topicPersister)

	trendRepo := database.NewTrendRepo(gdb)
	trendSvc := trend.NewService(trendRepo)
	topicProvider := jobs.NewDBTopicProvider(sqlDB)
	snapshotJob := jobs.NewBuildSnapshotsJob(trendSvc, topicProvider)

	deliveryRepo := jobs.NewDBDeliveryRepository(sqlDB)
	emailResolver := jobs.NewDBUserEmailLookup(sqlDB)
	mailer := &noopMailer{}
	dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, emailResolver)

	monitorLister := jobs.NewDBMonitorLister(sqlDB)

	runner := jobs.NewRunner()
	runner.Register("poll_monitor", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "poll_monitor: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if err := pollJob.Run(ctx, jobs.MonitorInfo{ID: monitorID, Platform: "x"}); err != nil {
				log.Printf("poll_monitor: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 1*time.Minute)
	runner.Register("aggregate_topics", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_topics: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if _, err := aggregateJob.Run(jobs.AggregateTopicsInput{MonitorID: monitorID, RunTime: time.Now()}); err != nil {
				log.Printf("aggregate_topics: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 5*time.Minute)
	runner.Register("build_snapshots", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "build_snapshots: running"))
		monitorIDs, err := monitorLister.ListActiveIDs(ctx)
		if err != nil {
			return fmt.Errorf("list monitors: %w", err)
		}
		for _, monitorID := range monitorIDs {
			if _, err := snapshotJob.Run(jobs.BuildSnapshotsInput{MonitorID: monitorID, SnapshotTime: time.Now()}); err != nil {
				log.Printf("build_snapshots: error for monitor %d: %v", monitorID, err)
			}
		}
		return nil
	}, 10*time.Minute)
	runner.Register("dispatch_notifications", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
		return dispatchJob.Run(ctx, 0)
	}, 1*time.Minute)

	if cfg.ObsidianVaultPath != "" {
		exporter := database.NewExporter(gdb)
		llmClient := llm.NewOpenAIClient(llm.OpenAIConfig{
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
		})
		writer := &jobs.DefaultVaultWriter{}

		monitorIDs, err := monitorLister.ListActiveIDs(context.Background())
		if err != nil {
			log.Printf("worker: failed to list monitors: %v", err)
		} else {
			for _, monitorID := range monitorIDs {
				monitorCfg := jobs.MonitorConfig{
					ID:   monitorID,
					Name: "Monitor",
					Slug: "monitor",
				}

				digestSvc := digest.NewService(nil)
				publishJob := jobs.NewPublishDailyTopicsJob(
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
				}, 1*time.Minute)
			}
		}
	} else {
		log.Print("worker: publish_daily_topics disabled (OBSIDIAN_VAULT_PATH not set)")
	}

	return runner
}

// gormDBFromSQL wraps an existing *sql.DB as *gorm.DB for repository use in worker.
func gormDBFromSQL(sqlDB *sql.DB) *gorm.DB {
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Printf("worker: gorm wrap failed: %v", err)
		return nil
	}
	return gdb
}

type noopMailer struct{}

func (m *noopMailer) Send(_ context.Context, to, subject, _ string) (string, error) {
	log.Printf("mailer: would send to %s subject=%q (noop)", to, subject)
	return "noop-msg-id", nil
}
