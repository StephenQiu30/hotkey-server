package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"gorm.io/gorm"
)

func runWorkerWithDB(ctx context.Context, cfg config.Config, db *gorm.DB) {
	ctx, cancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Print(observability.RenderLog("worker", "shutting down"))
		cancel()
	}()

	runner := newJobRunner(cfg, db)
	log.Print(observability.RenderLog("worker", "ready, running jobs"))
	runner.Run(ctx)
}
