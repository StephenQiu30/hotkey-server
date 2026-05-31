package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger) *API {
	authService, err := serviceauth.NewService(serviceauth.NewMemoryRepository(), serviceauth.Config{
		AccessTokenSecret: cfg.AuthTokenSecret,
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
	})
	if err != nil {
		panic(err)
	}
	return &API{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           transporthttp.NewRouterWithAuth(authService),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func (api *API) Run() error {
	api.logger.Info("starting hotkey api", "addr", api.server.Addr)
	return api.server.ListenAndServe()
}

func (api *API) Shutdown(ctx context.Context) error {
	return api.server.Shutdown(ctx)
}
