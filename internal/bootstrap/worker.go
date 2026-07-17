package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	monitorpostgres "github.com/StephenQiu30/hotkey-server/internal/modules/monitor/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
	platformscheduler "github.com/StephenQiu30/hotkey-server/internal/platform/scheduler"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type workerRunner interface {
	Run(context.Context, time.Duration) error
	ReclaimStale(context.Context, time.Duration) (int64, error)
}

func newQueueWorker(runtime *database.Runtime) *queue.Worker {
	return queue.NewWorker(runtime, nil)
}

func exposeWorkerRunner(worker *queue.Worker) workerRunner { return worker }

type collectionSchedulerRunner interface {
	Run(context.Context, time.Duration) error
}

func newQueueStore(runtime *database.Runtime) *queue.Store { return queue.NewStore(runtime) }

func exposeCollectionDueReader(reader *monitorpostgres.PublishedCollectionTargetReader) platformscheduler.CollectionDueReader {
	return reader
}

func newCollectionScheduler(reader platformscheduler.CollectionDueReader, store *queue.Store) *platformscheduler.CollectionScheduler {
	return platformscheduler.NewCollectionScheduler(reader, store)
}

func exposeCollectionSchedulerRunner(scheduler *platformscheduler.CollectionScheduler) collectionSchedulerRunner {
	return scheduler
}

func workerPollInterval(cfg config.Config) time.Duration {
	if cfg.WorkerPollInterval <= 0 {
		return time.Second
	}
	return cfg.WorkerPollInterval
}

func workerConcurrency(cfg config.Config) int {
	if cfg.WorkerConcurrency <= 0 {
		return 1
	}
	return cfg.WorkerConcurrency
}

func workerLeaseTimeout(cfg config.Config) time.Duration {
	if cfg.WorkerLeaseTimeout <= 0 {
		return 5 * time.Minute
	}
	return cfg.WorkerLeaseTimeout
}

func cronInterval(cfg config.Config) time.Duration {
	if cfg.CronInterval <= 0 {
		return time.Minute
	}
	return cfg.CronInterval
}

func registerPersistentWorkerLifecycle(lifecycle fx.Lifecycle, runner workerRunner, cfg config.Config, logger *zap.Logger) {
	var cancel context.CancelFunc
	var workers sync.WaitGroup
	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if _, err := runner.ReclaimStale(ctx, workerLeaseTimeout(cfg)); err != nil {
				return err
			}
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop
			for range workerConcurrency(cfg) {
				workers.Add(1)
				go func() {
					defer workers.Done()
					if err := runner.Run(runCtx, workerPollInterval(cfg)); err != nil && !errors.Is(err, context.Canceled) {
						logger.Error("worker loop stopped", zap.Error(err))
					}
				}()
			}
			logger.Info("worker runtime started", zap.Int("concurrency", workerConcurrency(cfg)), zap.Duration("poll_interval", workerPollInterval(cfg)))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancel != nil {
				cancel()
			}
			done := make(chan struct{})
			go func() {
				workers.Wait()
				close(done)
			}()
			select {
			case <-done:
				logger.Info("worker runtime stopped")
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}

func registerCollectionSchedulerLifecycle(lifecycle fx.Lifecycle, runner collectionSchedulerRunner, cfg config.Config, logger *zap.Logger) {
	var cancel context.CancelFunc
	var done chan struct{}
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			runCtx, stop := context.WithCancel(context.Background())
			cancel = stop
			done = make(chan struct{})
			go func() {
				defer close(done)
				if err := runner.Run(runCtx, cronInterval(cfg)); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("collection scheduler stopped", zap.Error(err))
				}
			}()
			logger.Info("collection scheduler started", zap.Duration("interval", cronInterval(cfg)))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cancel == nil {
				return nil
			}
			cancel()
			select {
			case <-done:
				logger.Info("collection scheduler stopped")
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}
