package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
)

// RunAPI starts the API server using Fx.
func RunAPI() {
	if os.Getenv("SMOKE_TEST") == "1" {
		runAPISmoke()
		return
	}

	fx.New(
		fx.Provide(config.Load),
		fx.Provide(func(cfg config.Config) (http.Handler, error) {
			return InitializeAPI(cfg)
		}),
		fx.Invoke(startHTTPServer),
	).Run()
}

func startHTTPServer(lc fx.Lifecycle, cfg config.Config, handler http.Handler) {
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Print(observability.RenderLog("api", "listening on "+cfg.HTTPAddr))
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Print(observability.RenderLog("api", "shutting down"))
			return srv.Shutdown(ctx)
		},
	})
}

// runAPISmoke starts the API in smoke test mode (SMOKE_TEST=1).
// Uses in-memory stubs and bypasses auth — no database required.
func runAPISmoke() {
	log.Print(observability.RenderLog("api", "smoke test mode"))
	fx.New(
		fx.Provide(config.Load),
		fx.Provide(newSmokeRouter),
		fx.Invoke(startHTTPServer),
	).Run()
}
