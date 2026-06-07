package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	platformredis "github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	servicexauth "github.com/StephenQiu30/hotkey-server/internal/service/xauth"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger) *API {
	authService, err := serviceauth.NewService(serviceauth.NewMemoryRepository(), serviceauth.Config{
		AccessTokenSecret: authSecret(cfg),
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
	})
	if err != nil {
		panic(err)
	}
	redisClient := platformredis.NewClient(cfg.RedisURL, platformredis.Options{DialTimeout: 250 * time.Millisecond})
	adminService := serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
		PostgreSQLPing: func(context.Context) error { return nil },
		RedisPing:      redisClient.Ping,
		DashScopeKey:   cfg.DashScopeAPIKey,
		SMTPHost:       cfg.SMTPHost,
	})
	xAuthService := servicexauth.NewService(servicexauth.NewMemoryRepository(), servicexauth.Config{
		ClientID:     cfg.XClientID,
		ClientSecret: cfg.XClientSecret,
		RedirectURL:  cfg.XRedirectURL,
	})

	return &API{
		server: &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
				AuthService:    authService,
				ChannelService: servicechannel.NewService(servicechannel.NewMemoryRepository()),
				AdminService:   adminService,
				XAuthService:   xAuthService,
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
