package fxapp

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/report"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/internal/worker"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"go.uber.org/fx"
	"go.uber.org/zap"
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
		fx.Provide(fx.Annotate(gormimpl.NewKnowledgeRunRepo, fx.As(new(worker.RunRepository)))),

		// Query services — annotate concrete -> interface for DI
		fx.Provide(fx.Annotate(database.NewContentQueryService, fx.As(new(content.PostQueryService)))),
		fx.Provide(fx.Annotate(database.NewTopicQueryService, fx.As(new(topic.TopicQueryService)))),
		fx.Provide(fx.Annotate(database.NewTrendQueryService, fx.As(new(trend.TrendQueryService)))),

		// Business services
		fx.Provide(auth.NewService),
		fx.Provide(newMonitorService),
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

func newMonitorService(repo monitor.Repository) *monitor.Service {
	return monitor.NewService(repo, nil)
}

func newDailyObsidianPublishJob(cfg *config.Config, monitorSvc *monitor.Service, reportSvc *report.Service, exportRepo report.ExportRepository, runRepo worker.RunRepository) *worker.DailyObsidianPublishJob {
	return worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: cfg.ObsidianVaultPath,
		Monitors:  monitorSvc,
		Reports:   reportSvc,
		Exports:   exportRepo,
		Runs:      runRepo,
		Now:       time.Now,
	})
}

func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config, dailyJob *worker.DailyObsidianPublishJob) {
	var (
		producer *queue.Producer
		rdb      *redis.Client
		consumer *queue.Consumer
		cronS    *cron.Cron
	)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := logging.Init(cfg.LogLevel, cfg.LogFormat, cfg.LogOutput); err != nil {
				return fmt.Errorf("logging init: %w", err)
			}

			if os.Getenv("SMOKE_TEST") == "1" {
				go func() {
					if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						logging.L().Error("http server error", zap.Error(err))
					}
				}()
				return nil
			}

			// --- Kafka producer ---
			producer = queue.NewProducer(cfg.KafkaBrokers)

			// --- Redis dedupe (needed by dispatcher) ---
			var dedupe *queue.Dedupe
			if cfg.RedisAddr != "" {
				rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
				if err := rdb.Ping(ctx).Err(); err != nil {
					logging.L().Warn("redis ping failed, dedup degraded",
						zap.String("addr", cfg.RedisAddr),
						zap.Error(err),
					)
				}
				dedupe = queue.NewDedupe(rdb)
			}

			// --- Dispatcher ---
			dispatcher := queue.NewDispatcher(producer, dedupe)
			dispatcher.Register(dailyJob)

			// --- DLQ recorder — persists exhausted-retry messages to DB ---
			dispatcher.SetDLQRecorder(func(ctx context.Context, topic string, msg queue.Message, errMsg string) {
				if db != nil {
					if dberr := db.WithContext(ctx).Create(&queue.DLQRecord{
						Topic:       topic,
						MessageID:   msg.ID,
						MessageType: msg.Type,
						Payload:     string(msg.Payload),
						ErrorMsg:    errMsg,
						RetryCount:  msg.RetryCount,
						CreatedAt:   time.Now(),
					}).Error; dberr != nil {
						logging.L().Error("dlq record persist failed",
							zap.String("topic", topic),
							zap.String("msg_id", msg.ID),
							zap.Error(dberr),
						)
					}
				}
			})

			// --- Kafka consumer ---
			consumer = queue.NewConsumer(
				cfg.KafkaBrokers,
				queue.TopicDigestRun,
				cfg.KafkaConsumerGroup,
				dispatcher,
			)
			go func() {
				logging.L().Info("kafka consumer starting",
					zap.String("topic", queue.TopicDigestRun),
				)
				if err := consumer.Run(ctx); err != nil && err != queue.ErrConsumerClosed {
					logging.L().Error("kafka consumer error", zap.Error(err))
				}
			}()

			// --- Cron scheduler ---
			loc, err := time.LoadLocation(cfg.DailyDigestTimezone)
			if err != nil {
				return fmt.Errorf("cron: load location %q: %w", cfg.DailyDigestTimezone, err)
			}
			cronS = cron.New(cron.WithLocation(loc))
			_, err = cronS.AddFunc("0 8 * * *", func() {
				now := time.Now().In(loc)
				payload, _ := json.Marshal(map[string]string{
					"target_date": now.AddDate(0, 0, -1).Format("2006-01-02"),
				})
				if pubErr := producer.Publish(context.Background(), queue.TopicDigestRun, queue.NewMessage("digest.run", payload)); pubErr != nil {
					logging.L().Error("cron publish digest error", zap.Error(pubErr))
				}
			})
			if err != nil {
				return fmt.Errorf("cron: add func: %w", err)
			}
			cronS.Start()
			logging.L().Info("worker started (cron + kafka)")

			go func() {
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					logging.L().Error("http server error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if cronS != nil {
				cronS.Stop()
			}
			if consumer != nil {
				if err := consumer.Close(); err != nil {
					logging.L().Error("consumer close error", zap.Error(err))
				}
			}
			if producer != nil {
				if err := producer.Close(); err != nil {
					logging.L().Error("producer close error", zap.Error(err))
				}
			}
			if rdb != nil {
				if err := rdb.Close(); err != nil {
					logging.L().Error("redis close error", zap.Error(err))
				}
			}
			if err := srv.Shutdown(ctx); err != nil {
				logging.L().Error("http server shutdown error", zap.Error(err))
			}
			if db != nil {
				sqlDB, err := db.DB()
				if err == nil && sqlDB != nil {
					if err := sqlDB.Close(); err != nil {
						logging.L().Error("db close error", zap.Error(err))
					}
				}
			}
			return nil
		},
	})
}
