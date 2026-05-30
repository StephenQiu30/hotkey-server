package main

import (
	"errors"
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New()
	api := app.NewAPI(cfg, log)

	if err := api.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("api stopped", "error", err)
		panic(err)
	}
}
