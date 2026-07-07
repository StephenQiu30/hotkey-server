package fxapp

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/module"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// NewApp creates the Fx-powered application.
func NewApp() *fx.App {
	return fx.New(
		module.Infra,

		// Wire existing repo constructors (will transition to gormimpl)
		fx.Provide(database.NewAuthRepo),
		fx.Provide(database.NewMonitorRepo),
		fx.Provide(database.NewNotifyRepo),
		fx.Provide(database.NewContentQueryService),
		fx.Provide(database.NewTopicQueryService),
		fx.Provide(database.NewTrendQueryService),
		fx.Provide(database.NewHotEventRepo),

		// Business services
		fx.Provide(auth.NewService),
		fx.Provide(monitor.NewService),
		fx.Provide(notify.NewService),
		fx.Provide(hotevent.NewQueryService),

		// HTTP server
		fx.Provide(NewHTTPServer),

		// Lifecycle hooks
		fx.Invoke(registerHooks),
	)
}

// HTTPServerIn groups dependencies for the HTTP server.
type HTTPServerIn struct {
	fx.In

	Config      *config.Config
	AuthService *auth.Service
	MonitorSvc  *monitor.Service
	NotifySvc   *notify.Service

	PostQuerySvc  content.PostQueryService
	TopicQuerySvc topic.TopicQueryService
	TrendQuerySvc trend.TrendQueryService
	HotEventMgr   platformhttp.HotEventManager
}

func NewHTTPServer(in HTTPServerIn) *http.Server {
	smokeTest := os.Getenv("SMOKE_TEST") == "1"

	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:       in.Config.JWTSecret,
		SmokeTest:       smokeTest,
		SwaggerEnabled:  in.Config.SwaggerEnabled,
		AuthService:     in.AuthService,
		MonitorSvc:      in.MonitorSvc,
		NotifySvc:       in.NotifySvc,
		PostQuerySvc:    in.PostQuerySvc,
		TopicQuerySvc:   in.TopicQuerySvc,
		TrendQuerySvc:   in.TrendQuerySvc,
		HotEventManager: in.HotEventMgr,
	})

	return &http.Server{
		Addr:         in.Config.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if os.Getenv("SMOKE_TEST") == "1" {
				return nil
			}
			go func() {
				log.Printf("worker: started")
				<-ctx.Done()
				log.Printf("worker: stopped")
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if db != nil {
				sqlDB, err := db.DB()
				if err == nil && sqlDB != nil {
					sqlDB.Close()
				}
			}
			return srv.Shutdown(ctx)
		},
	})
}
