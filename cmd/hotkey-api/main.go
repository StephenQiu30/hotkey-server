package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logger"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/scheduler"
	"github.com/StephenQiu30/hotkey-server/internal/service/dedup"
	"github.com/StephenQiu30/hotkey-server/internal/service/embedding"
	"github.com/StephenQiu30/hotkey-server/internal/service/filter"
	"github.com/StephenQiu30/hotkey-server/internal/service/ingest"
	"github.com/StephenQiu30/hotkey-server/internal/service/mail"
	"github.com/StephenQiu30/hotkey-server/internal/service/normalize"
	"github.com/StephenQiu30/hotkey-server/internal/service/quality"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
	"github.com/StephenQiu30/hotkey-server/internal/worker"

	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败: %s\n", err)
		os.Exit(1)
	}
	logSlog := logger.New()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 统一依赖装配：DB、Redis、Queue、Repository、基础设施客户端
	deps, err := app.NewDeps(cfg)
	if err != nil {
		logSlog.Error("依赖初始化失败", "error", err)
		fmt.Fprintf(os.Stderr, "依赖初始化失败: %s\n", err)
		os.Exit(1)
	}
	defer deps.Close()

	// Initialize Services（worker pipeline 所需）
	normalizeSvc := normalize.NewService(normalize.DefaultConfig())
	filterSvc := filter.NewService(filter.Config{
		MinTitleRunes:   4,
		MinSnippetRunes: 10,
	})
	qualitySvc := quality.NewService(quality.DefaultConfig())
	dedupSvc := dedup.NewService(dedup.Config{
		SimilarityThreshold: cfg.HotspotSimilarityThreshold,
	}, deps.ContentRepo, deps.HotspotRepo)
	embeddingSvc := embedding.NewService(embedding.Config{
		Model: cfg.EmbeddingModel,
	}, deps.ContentRepo, deps.HotspotRepo, deps.DashScope)
	sourceSvc := servicesource.NewService(deps.SourceRepo)

	// Initialize Fetchers
	multiFetcher := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS:        fetcher.NewRSSFetcher(nil),
		fetcher.SourceTypePublicPage: fetcher.NewPublicPageFetcher(nil),
	})

	ingestSvc := ingest.NewService(deps.ContentRepo, deps.JobQueue,
		ingest.WithNormalize(normalizeSvc),
		ingest.WithFilter(filterSvc),
		ingest.WithQuality(qualitySvc),
	)

	mailService := mail.NewService(nil, nil, mail.Config{
		Host:       cfg.SMTPHost,
		Port:       cfg.SMTPPort,
		Username:   cfg.SMTPUsername,
		Password:   cfg.SMTPPassword,
		From:       cfg.SMTPFrom,
		TLS:        cfg.SMTPTLS,
		StartTLS:   cfg.SMTPStartTLS,
		Configured: cfg.SMTPHost != "",
	})

	// Initialize API Runtime
	api := app.NewAPI(cfg, logSlog, deps.DB, deps.RedisClient)

	// Initialize Worker Runtime
	workerRuntime := worker.New(deps.JobQueue, deps.RedisClient, logSlog,
		worker.WithHandler(queue.JobTypeCollectSource, worker.NewCollectSourceHandler(sourceSvc, multiFetcher, ingestSvc)),
		worker.WithHandler(queue.JobTypeGenerateEmbedding, worker.NewGenerateEmbeddingHandler(embeddingSvc, dedupSvc, deps.ContentRepo)),
		worker.WithHandler(queue.JobTypeSendDailyEmail, worker.NewSendDailyEmailHandler(mailService)),
		worker.WithHandler(queue.JobTypeSendWeeklyEmail, worker.NewSendWeeklyEmailHandler(mailService)),
	)

	// Initialize Scheduler Runtime
	dailyEmailScheduler := scheduler.NewDailyEmailScheduler(deps.JobQueue, scheduler.DailyEmailOptions{
		DefaultDailySendAt: "08:30",
	})
	weeklyEmailScheduler := scheduler.NewWeeklyEmailScheduler(deps.JobQueue, scheduler.WeeklyEmailOptions{
		DefaultWeeklySendAt: "09:00",
		WeeklySendDay:       time.Monday,
	})
	schedulerRuntime := scheduler.NewCompositeScheduler(
		scheduler.NewHourlyCollectScheduler(deps.JobQueue, scheduler.HourlyCollectOptions{
			SourceID: cfg.CollectSourceID,
		}),
		dailyEmailScheduler,
		weeklyEmailScheduler,
	)

	runtime := app.NewRuntime(cfg, app.RuntimeComponents{
		API:       api,
		Worker:    workerRuntime,
		Scheduler: schedulerRuntime,
	})

	if err := runtime.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		logSlog.Error("runtime stopped", "error", err)
		panic(err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(shutdownCtx); err != nil {
		logSlog.Error("runtime shutdown failed", "error", err)
	}
}
