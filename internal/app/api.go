package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	platformredis "github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/adminrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/channelrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/userrepo"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger, db *sql.DB, redisClient *platformredis.Client) *API {
	authService, err := serviceauth.NewService(userrepo.New(db), serviceauth.Config{
		AccessTokenSecret: authSecret(cfg),
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
	})
	if err != nil {
		panic(err)
	}
	adminService := serviceadmin.NewService(adminrepo.New(db), serviceadmin.Config{
		PostgreSQLPing: func(ctx context.Context) error { return db.PingContext(ctx) },
		RedisPing:      redisClient.Ping,
		DashScopeKey:   cfg.DashScopeAPIKey,
		SMTPHost:       cfg.SMTPHost,
	})
	return &API{
		server: &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
				AuthService:    authService,
				ChannelService: servicechannel.NewService(channelrepo.New(db)),
				AdminService:   adminService,
			}),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func authSecret(cfg config.Config) string {
	if cfg.AuthTokenSecret != "" {
		return cfg.AuthTokenSecret
	}
	panic("AuthTokenSecret must be configured")
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
