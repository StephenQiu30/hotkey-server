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
)

func newJobRunner(cfg config.Config, db *sql.DB) *jobs.Runner {
	xClient := x.NewClient(cfg.XToken, cfg.XBaseURL)
	connector := jobs.NewXConnectorAdapter(xClient, cfg.XToken)

	hitScorerRepo := jobs.NewDBHitScorerRepo(db)
	scoringSvc := scoring.NewService(hitScorerRepo)
	scorer := jobs.NewScorerAdapter(scoringSvc)

	runRepo := jobs.NewDBRunRepository(db)
	postRepo := jobs.NewDBPostRepository(db)
	hitRepo := jobs.NewDBHitRepository(db)
	pollJob := jobs.NewPollMonitorJob(runRepo, postRepo, hitRepo, connector, scorer)

	topicRepo := database.NewTopicRepo(db)
	postCandidateProvider := jobs.NewDBPostCandidateProvider(db)
	topicPersister := jobs.NewTopicPersisterAdapter(topicRepo)
	aggregateJob := jobs.NewAggregateTopicsJob(postCandidateProvider, topicPersister)

	trendRepo := database.NewTrendRepo(db)
	trendSvc := trend.NewService(trendRepo)
	topicProvider := jobs.NewDBTopicProvider(db)
	snapshotJob := jobs.NewBuildSnapshotsJob(trendSvc, topicProvider)

	deliveryRepo := jobs.NewDBDeliveryRepository(db)
	emailResolver := jobs.NewDBUserEmailLookup(db)
	mailer := &noopMailer{}
	dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, emailResolver)

	monitorLister := jobs.NewDBMonitorLister(db)

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

	// Daily digest publish job — gates on DAILY_DIGEST_TIME CST
	if cfg.ObsidianVaultPath != "" {
		scheduler := jobs.NewDailyScheduler(cfg.DailyDigestTime, cfg.DailyDigestTimezone)
		exportRepo := jobs.NewExportRepoAdapter(database.NewDigestRepo(db))
		digestSvc := jobs.NewDigestServiceAdapter(digest.NewService(nil)) // TODO: provide TopicFilter
		llmClient := jobs.NewLLMClientAdapter(llm.NewOpenAIClient(llm.OpenAIConfig{
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
		}))
		obsidianWriter := jobs.NewObsidianWriterAdapter()
		publishJob := jobs.NewPublishDailyTopicsJob(
			monitorLister,
			digestSvc,
			llmClient,
			obsidianWriter,
			exportRepo,
			scheduler,
			jobs.PublishDailyTopicsConfig{
				VaultPath: cfg.ObsidianVaultPath,
				Target:    cfg.DailyDigestTarget,
				TopN:      cfg.DailyDigestTopN,
			},
		)
		runner.Register("publish_daily_topics", func(ctx context.Context) error {
			log.Print(observability.RenderLog("worker", "publish_daily_topics: running"))
			return publishJob.Run(ctx, time.Now())
		}, 1*time.Minute)
	} else {
		log.Print("worker: publish_daily_topics disabled (OBSIDIAN_VAULT_PATH not set)")
	}

	return runner
}

type noopMailer struct{}

func (m *noopMailer) Send(_ context.Context, to, subject, _ string) (string, error) {
	log.Printf("mailer: would send to %s subject=%q (noop)", to, subject)
	return "noop-msg-id", nil
}
