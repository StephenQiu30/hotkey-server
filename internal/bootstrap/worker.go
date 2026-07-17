package bootstrap

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/internal/platform/queue"
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
