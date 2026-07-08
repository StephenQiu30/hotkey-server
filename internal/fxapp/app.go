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
	"github.com/StephenQiu30/hotkey-server/internal/llm"
	"github.com/StephenQiu30/hotkey-server/internal/module"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// NewApp creates the Fx-powered application.
func NewApp() *fx.App {
	return fx.New(
		module.Infra,

		// Repository implementations (direct GORM implementations of domain interfaces)
		fx.Provide(fx.Annotate(gormimpl.NewUserRepo, fx.As(new(auth.Repository)))),
		fx.Provide(fx.Annotate(gormimpl.NewMonitorRepo, fx.As(new(monitor.Repository)))),
		fx.Provide(fx.Annotate(gormimpl.NewNotifyRepo, fx.As(new(notify.Repository)))),
		fx.Provide(fx.Annotate(gormimpl.NewHotEventRepo, fx.As(new(hotevent.Repository)))),
		fx.Provide(fx.Annotate(gormimpl.NewReportRepo, fx.As(new(report.Repository)))),
		fx.Provide(fx.Annotate(gormimpl.NewReportExportRepo, fx.As(new(report.ExportRepository)))),

		// Query services — annotate concrete -> interface for DI
		fx.Provide(fx.Annotate(database.NewContentQueryService, fx.As(new(content.PostQueryService)))),
		fx.Provide(fx.Annotate(database.NewTopicQueryService, fx.As(new(topic.TopicQueryService)))),
		fx.Provide(fx.Annotate(database.NewTrendQueryService, fx.As(new(trend.TrendQueryService)))),

		// Business services
		fx.Provide(auth.NewService),
		fx.Provide(monitor.NewService),
		fx.Provide(notify.NewService),
		fx.Provide(newReportService),
		fx.Provide(fx.Annotate(hotevent.NewQueryService, fx.As(new(platformhttp.HotEventManager)))),

		// HTTP server
		fx.Provide(NewHTTPServer),

		// LLM 内容聚合
		fx.Provide(fx.Annotate(llm.NewProvider, fx.As(new(llm.Provider)))),
		fx.Provide(fx.Annotate(llm.NewService, fx.As(new(llm.Service)))),
		fx.Provide(llm.NewChain),

		// Daily obsidian publish worker
		fx.Provide(newDailyObsidianPublishJob),

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
	ReportSvc   *report.Service

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
		ReportSvc:       in.ReportSvc,
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

func newReportService(repo report.Repository) *report.Service {
	return report.NewService(repo, time.Now)
}

func newDailyObsidianPublishJob(cfg *config.Config, monitorSvc *monitor.Service, reportSvc *report.Service, exportRepo report.ExportRepository) *worker.DailyObsidianPublishJob {
	return worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: cfg.ObsidianVaultPath,
		Monitors:  monitorSvc,
		Reports:   reportSvc,
		Exports:   exportRepo,
		Now:       time.Now,
	})
}

func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config, dailyJob *worker.DailyObsidianPublishJob) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if os.Getenv("SMOKE_TEST") == "1" {
				// Smoke test: start HTTP server only, no worker
				go func() {
					if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Printf("http server error: %v", err)
					}
				}()
				return nil
			}
			go func() {
				log.Printf("worker: started")
				<-ctx.Done()
				log.Printf("worker: stopped")
			}()
			go func() {
				ticker := time.NewTicker(time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case now := <-ticker.C:
						shouldRun, targetDate, err := worker.ShouldRun(now, nil, worker.DailyScheduleConfig{
							Time:     cfg.DailyDigestTime,
							Timezone: cfg.DailyDigestTimezone,
							Target:   cfg.DailyDigestTarget,
						})
						if err != nil {
							log.Printf("daily obsidian scheduler error: %v", err)
							continue
						}
						if !shouldRun {
							continue
						}
						if err := dailyJob.RunOnce(context.Background(), targetDate); err != nil {
							log.Printf("daily obsidian publish failed: %v", err)
						}
					}
				}
			}()
			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Printf("http server error: %v", err)
				}
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
