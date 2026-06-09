package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	platformpostgres "github.com/StephenQiu30/hotkey-server/internal/platform/postgres"
	platformredis "github.com/StephenQiu30/hotkey-server/internal/platform/redis"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/adminrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/authorizationrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/channelrepo"
	"github.com/StephenQiu30/hotkey-server/internal/repository/postgres/userrepo"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
	db     *sql.DB
}

func NewAPI(cfg config.Config, logger *slog.Logger, db *sql.DB, redisClient *platformredis.Client, scoringSvc *servicehotspot.ScoringService) *API {
	var authRepo serviceauth.Repository
	var azRepo serviceauth.AuthorizationRepository
	if db != nil {
		authRepo = userrepo.New(db)
		azRepo = authorizationrepo.New(db)
	} else {
		logger.Warn("using in-memory repositories")
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

	if cfg.EncryptionKey == "" {
		panic("EncryptionKey must be configured")
	}
	encKey := []byte(cfg.EncryptionKey)
	enc, err := crypto.NewAESGCMEncryptor(encKey)
	if err != nil {
		panic(err)
	}

	azService, err := serviceauth.NewAuthorizationService(authRepo, azRepo, enc, nil)
	if err != nil {
		panic(err)
	}
	if db != nil {
		azService = azService.WithTransactor(platformpostgres.NewTransactionalDB(db))
	}

	var adminService *serviceadmin.Service
	if db != nil {
		adminService = serviceadmin.NewService(adminrepo.New(db), serviceadmin.Config{
			PostgreSQLPing: func(ctx context.Context) error { return db.PingContext(ctx) },
			RedisPing:      redisClient.Ping,
			DashScopeKey:   cfg.DashScopeAPIKey,
			SMTPHost:       cfg.SMTPHost,
		})
	} else {
		adminService = serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
			PostgreSQLPing: func(ctx context.Context) error {
				return errors.New("postgres not connected")
			},
			RedisPing:    redisClient.Ping,
			DashScopeKey: cfg.DashScopeAPIKey,
			SMTPHost:     cfg.SMTPHost,
		})
	}

	var channelSvc *servicechannel.Service
	if db != nil {
		channelSvc = servicechannel.NewService(channelrepo.New(db))
	} else {
		channelSvc = servicechannel.NewService(servicechannel.NewMemoryRepository())
	}

	return &API{
		server: &http.Server{
			Addr: cfg.HTTPAddr,
			Handler: transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
				AuthService:          authService,
				AuthorizationService: azService,
				ChannelService:       channelSvc,
				AdminService:         adminService,
				ScoringService:       scoringSvc,
			}),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
		db:     db,
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
	shutdownErr := api.server.Shutdown(ctx)
	if api.db != nil {
		if err := api.db.Close(); err != nil {
			api.logger.Error("failed to close database pool", "error", err)
		}
	}
	return shutdownErr
}
