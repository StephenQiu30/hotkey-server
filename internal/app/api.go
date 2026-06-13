package app

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.uber.org/fx"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
)

// RunAPI starts the API server using Fx.
func RunAPI() {
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
