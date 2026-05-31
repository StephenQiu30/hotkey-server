package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logger"
	"github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/scheduler"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
)

func main() {
	cfg := config.Load()
	log := logger.New()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := app.NewAPI(cfg, log)
	redisClient := redis.NewClient(cfg.RedisURL, redis.Options{})
	jobQueue := queue.NewRedisQueue(redisClient, queue.RedisQueueOptions{QueueName: "hotkey:jobs:pending"})
	workerRuntime := worker.New(jobQueue, redisClient, log)
	schedulerRuntime := scheduler.NewHourlyCollectScheduler(jobQueue, scheduler.HourlyCollectOptions{
		SourceID: cfg.CollectSourceID,
	})
	runtime := app.NewRuntime(cfg, app.RuntimeComponents{
		API:       api,
		Worker:    workerRuntime,
		Scheduler: schedulerRuntime,
	})

	if err := runtime.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		log.Error("runtime stopped", "error", err)
		panic(err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := runtime.Shutdown(shutdownCtx); err != nil {
		log.Error("runtime shutdown failed", "error", err)
	}
}
