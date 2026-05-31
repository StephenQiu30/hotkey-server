package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger) *API {
	return &API{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           transporthttp.NewRouter(),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func (api *API) Run(ctx context.Context) error {
	api.logger.Info("starting hotkey api", "addr", api.server.Addr)
	errs := make(chan error, 1)
	go func() {
		errs <- api.server.ListenAndServe()
	}()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := api.server.Shutdown(shutdownCtx)
		listenErr := <-errs
		if shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
			return errors.Join(ctx.Err(), shutdownErr)
		}
		if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			return errors.Join(ctx.Err(), listenErr)
		}
		return ctx.Err()
	}
}

func (api *API) Shutdown(ctx context.Context) error {
	return api.server.Shutdown(ctx)
}
