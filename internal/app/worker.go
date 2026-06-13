package app

import (
	"context"
	"log"
	"time"

	"go.uber.org/fx"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/jobs"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
)

// RunWorker starts the worker using Fx.
func RunWorker() {
	fx.New(
		fx.Provide(config.Load),
		fx.Invoke(startWorker),
	).Run()
}

func startWorker(lc fx.Lifecycle, cfg config.Config) {
	ctx, cancel := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {
			log.Print(observability.RenderLog("worker", "starting"))

			// Connect to database
			db, err := database.Open(cfg.DatabaseURL)
			if err != nil {
				return err
			}
			_ = db // Will be used when repos are implemented

			// Wire dispatch job (stub dependencies for now)
			deliveryRepo := &stubDeliveryRepo{}
			mailer := &stubMailer{}
			emailResolver := &stubUserEmailLookup{}
			dispatchJob := jobs.NewDispatchJob(deliveryRepo, mailer, emailResolver)

			// Register background jobs
			runner := jobs.NewRunner()
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
			runner.Register("dispatch_notifications", func(ctx context.Context) error {
				log.Print(observability.RenderLog("worker", "dispatch_notifications: running"))
				return dispatchJob.Run(ctx, 0)
			}, 1*time.Minute)

			log.Print(observability.RenderLog("worker", "ready, running jobs"))
			go runner.Run(ctx)
			return nil
		},
		OnStop: func(stopCtx context.Context) error {
			log.Print(observability.RenderLog("worker", "shutting down"))
			cancel()
			return nil
		},
	})
}

// stubDeliveryRepo is a placeholder for the jobs delivery repository.
type stubDeliveryRepo struct{}

func (r *stubDeliveryRepo) CreateDelivery(_ context.Context, _ jobs.EmailDelivery) error {
	return nil
}

func (r *stubDeliveryRepo) UpdateDeliveryStatus(_ context.Context, _ int64, _ string, _ string, _ string) error {
	return nil
}

func (r *stubDeliveryRepo) GetPendingDeliveries(_ context.Context, _ int) ([]jobs.EmailDelivery, error) {
	return nil, nil
}

// stubUserEmailLookup resolves notification IDs to empty email addresses.
type stubUserEmailLookup struct{}

func (r *stubUserEmailLookup) ResolveEmail(_ context.Context, _ int64) (string, error) {
	return "unresolved@example.com", nil
}

// stubMailer is a placeholder mailer that logs instead of sending.
type stubMailer struct{}

func (m *stubMailer) Send(_ context.Context, _, _, _ string) (string, error) {
	return "stub-msg-id", nil
}
