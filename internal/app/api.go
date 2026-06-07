package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	platformpostgres "github.com/StephenQiu30/hotkey-server/internal/platform/postgres"
	platformredis "github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/authorizationrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/userrepo"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger) *API {
	db, err := platformpostgres.NewPool(cfg.DatabaseURL, platformpostgres.Options{})
	if err != nil {
		logger.Error("failed to connect to postgres, falling back to memory", "error", err)
	}

	var authRepo serviceauth.Repository
	var azRepo serviceauth.AuthorizationRepository
	if db != nil {
		authRepo = userrepo.New(db)
		azRepo = authorizationrepo.New(db)
	} else {
		authRepo = serviceauth.NewMemoryRepository()
		azRepo = serviceauth.NewMemoryAuthorizationRepository()
	}

	authService, err := serviceauth.NewService(authRepo, serviceauth.Config{
		AccessTokenSecret: authSecret(cfg),
		AccessTokenTTL:    cfg.AccessTokenTTL,
		RefreshTokenTTL:   cfg.RefreshTokenTTL,
	})
	if err != nil {
		panic(err)
	}

	encKey := []byte(cfg.EncryptionKey)
	if len(encKey) == 0 {
		encKey = []byte("0123456789abcdef0123456789abcdef") // Fallback for dev
	}
	enc, err := crypto.NewAESGCMEncryptor(encKey)
	if err != nil {
		panic(err)
	}

	azService := serviceauth.NewAuthorizationService(authRepo, azRepo, enc, nil)
	if db != nil {
		azService = azService.WithTransactor(platformpostgres.NewTransactionalDB(db))
	}

	redisClient := platformredis.NewClient(cfg.RedisURL, platformredis.Options{DialTimeout: 250 * time.Millisecond})
	adminService := serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
		PostgreSQLPing: func(ctx context.Context) error {
			if db == nil {
				return errors.New("postgres not connected")
			}
			return db.PingContext(ctx)
		},
		RedisPing:    redisClient.Ping,
		DashScopeKey: cfg.DashScopeAPIKey,
		SMTPHost:     cfg.SMTPHost,
	})
	return &API{
		server: &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
				AuthService:          authService,
				AuthorizationService: azService,
				ChannelService:       servicechannel.NewService(servicechannel.NewMemoryRepository()),
				AdminService:         adminService,
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
