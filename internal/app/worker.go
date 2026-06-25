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

func runWorkerWithDB(ctx context.Context, cfg config.Config, gdb *gorm.DB) {
	ctx, cancel := context.WithCancel(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Print(observability.RenderLog("worker", "shutting down"))
		cancel()
	}()

	sqlDB, err := gdb.DB()
	if err != nil {
		log.Printf("worker: failed to get sql db: %v", err)
		return
	}

	runner := newJobRunner(cfg, sqlDB)
	log.Print(observability.RenderLog("worker", "ready, running jobs"))
	runner.Run(ctx)
}
