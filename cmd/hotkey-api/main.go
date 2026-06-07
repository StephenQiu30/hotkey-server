package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/dashscope"
	"github.com/StephenQiu30/hotkey-server/internal/platform/fetcher"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logger"
	"github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/contentrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/hotspotrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/sourcerepo"
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
	logSlog := logger.New()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize Database
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logSlog.Error("failed to open database", "error", err)
		panic(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logSlog.Error("failed to close database", "error", err)
		}
	}()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		logSlog.Error("failed to ping database", "error", err)
		panic(err)
	}

	// Initialize Redis
	redisClient := redis.NewClient(cfg.RedisURL, redis.Options{})
	if err := redisClient.Ping(ctx); err != nil {
		logSlog.Error("failed to ping redis", "error", err)
		panic(err)
	}

	// Initialize Repositories
	contentRepo := contentrepo.New(db)
	hotspotRepo := hotspotrepo.New(db)
	sourceRepo := sourcerepo.New(db)

	// Initialize Infrastructure Providers
	embeddingProvider := dashscope.New(cfg.DashScopeAPIKey)

	// Initialize Services
	normalizeSvc := normalize.NewService(normalize.DefaultConfig())
	filterSvc := filter.NewService(filter.Config{
		MinTitleRunes:   4,
		MinSnippetRunes: 10,
	})
	qualitySvc := quality.NewService(quality.DefaultConfig())
	dedupSvc := dedup.NewService(dedup.Config{
		SimilarityThreshold: cfg.HotspotSimilarityThreshold,
	}, contentRepo, hotspotRepo)
	embeddingSvc := embedding.NewService(embedding.Config{
		Model: cfg.EmbeddingModel,
	}, contentRepo, hotspotRepo, embeddingProvider)

	sourceSvc := servicesource.NewService(sourceRepo)

	// Initialize Fetchers
	multiFetcher := fetcher.NewMultiFetcher(map[fetcher.SourceType]fetcher.Fetcher{
		fetcher.SourceTypeRSS:        fetcher.NewRSSFetcher(nil),
		fetcher.SourceTypePublicPage: fetcher.NewPublicPageFetcher(nil),
	})

	jobQueue := queue.NewRedisQueue(redisClient, queue.RedisQueueOptions{QueueName: "hotkey:jobs:pending"})
	ingestSvc := ingest.NewService(contentRepo, jobQueue,
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
	api := app.NewAPI(cfg, logSlog, db, redisClient)

	// Initialize Worker Runtime
	workerRuntime := worker.New(jobQueue, redisClient, logSlog,
		worker.WithHandler(queue.JobTypeCollectSource, worker.NewCollectSourceHandler(sourceSvc, multiFetcher, ingestSvc)),
		worker.WithHandler(queue.JobTypeGenerateEmbedding, worker.NewGenerateEmbeddingHandler(embeddingSvc, dedupSvc, contentRepo)),
		worker.WithHandler(queue.JobTypeSendDailyEmail, worker.NewSendDailyEmailHandler(mailService)),
		worker.WithHandler(queue.JobTypeSendWeeklyEmail, worker.NewSendWeeklyEmailHandler(mailService)),
	)

	dailyEmailScheduler := scheduler.NewDailyEmailScheduler(jobQueue, scheduler.DailyEmailOptions{
		DefaultDailySendAt: "08:30",
	})
	weeklyEmailScheduler := scheduler.NewWeeklyEmailScheduler(jobQueue, scheduler.WeeklyEmailOptions{
		DefaultWeeklySendAt: "09:00",
		WeeklySendDay:       time.Monday,
	})
	schedulerRuntime := scheduler.NewCompositeScheduler(
		scheduler.NewHourlyCollectScheduler(jobQueue, scheduler.HourlyCollectOptions{
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
