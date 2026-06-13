package app

import (
	"context"
	"log"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"go.uber.org/fx"
)

// NewWorkerApp constructs the Fx application for the worker.
func NewWorkerApp(cfg config.Config) *fx.App {
	return fx.New(
		fx.Supply(cfg),
		fx.Invoke(startWorker),
	)
}

func startWorker(lc fx.Lifecycle, cfg config.Config) {
	log.Print(observability.RenderLog("worker", "starting"))

	runner := jobs.NewRunner()
	dispatchJob := jobs.NewDispatchJob(&stubDeliveryRepo{}, &stubMailer{}, &stubUserEmailLookup{})

	runner.Register("poll_monitor", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "poll_monitor: running"))
		return nil
	}, 1*time.Minute)
	runner.Register("aggregate_topics", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "aggregate_topics: running"))
		return nil
	}, 5*time.Minute)
	runner.Register("build_snapshots", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "build_snapshots: running"))
		return nil
	}, 10*time.Minute)
	// 0 means no limit — process all pending deliveries
	runner.Register("dispatch_notifications", func(ctx context.Context) error {
		log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
		return dispatchJob.Run(ctx, 0)
	}, 1*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			go func() {
				log.Print(observability.RenderLog("worker", "ready, running jobs"))
				runner.Run(ctx)
			}()
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			log.Print(observability.RenderLog("worker", "shutting down"))
			cancel()
			return nil
		},
	})
}
