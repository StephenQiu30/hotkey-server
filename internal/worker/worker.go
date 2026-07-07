package worker

import (
	"context"
	"log"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// Params groups all worker dependencies.
type Params struct {
	fx.In

	LC fx.Lifecycle
	DB *gorm.DB
	Cfg *config.Config
}

// RegisterWorkers starts background workers managed by Fx lifecycle.
func RegisterWorkers(p Params) {
	p.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go runPollWorker(ctx, p.DB, p.Cfg)
			go runDigestWorker(ctx, p.DB, p.Cfg)
			go runSnapshotWorker(ctx, p.DB, p.Cfg)
			go runCleanupWorker(ctx, p.DB, p.Cfg)
			log.Printf("workers: all background workers registered")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Printf("workers: stopping...")
			return nil
		},
	})
}

func runPollWorker(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	log.Printf("worker poll: started")
	<-ctx.Done()
	log.Printf("worker poll: stopped")
}

func runDigestWorker(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	log.Printf("worker digest: started")
	<-ctx.Done()
	log.Printf("worker digest: stopped")
}

func runSnapshotWorker(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	log.Printf("worker snapshot: started")
	<-ctx.Done()
	log.Printf("worker snapshot: stopped")
}

func runCleanupWorker(ctx context.Context, db *gorm.DB, cfg *config.Config) {
	log.Printf("worker cleanup: started")
	<-ctx.Done()
	log.Printf("worker cleanup: stopped")
}
