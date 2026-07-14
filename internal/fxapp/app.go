package fxapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/controller"
	"github.com/StephenQiu30/hotkey-server/internal/module"
	"github.com/StephenQiu30/hotkey-server/internal/platform/email"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/queue"
	"github.com/StephenQiu30/hotkey-server/internal/repository"
	"github.com/StephenQiu30/hotkey-server/internal/service"
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
		fx.Provide(fx.Annotate(repository.NewUserRepo,
			fx.As(new(service.UserRepository)),
			fx.As(new(service.UserLookup)),
		)),
		fx.Provide(fx.Annotate(repository.NewMonitorRepo, fx.As(new(service.MonitorRepository)))),
		fx.Provide(fx.Annotate(repository.NewNotifyRepo, fx.As(new(service.NotifyRepository)))),
		fx.Provide(fx.Annotate(repository.NewHotEventRepo, fx.As(new(service.HotEventRepository)))),
		fx.Provide(fx.Annotate(repository.NewReportRepo, fx.As(new(service.ReportRepository)))),
		fx.Provide(fx.Annotate(repository.NewReportExportRepo, fx.As(new(service.ExportRepository)))),
		fx.Provide(fx.Annotate(repository.NewKnowledgeRunRepo, fx.As(new(worker.RunRepository)))),

		// Query services — annotate concrete -> interface for DI
		fx.Provide(fx.Annotate(repository.NewContentQueryService, fx.As(new(content.PostQueryService)))),
		fx.Provide(fx.Annotate(repository.NewTopicQueryService, fx.As(new(service.TopicQueryService)))),
		fx.Provide(fx.Annotate(repository.NewTrendQueryService, fx.As(new(service.TrendQueryService)))),

		// Auth session repository
		fx.Provide(fx.Annotate(repository.NewRedisAuthSessionRepository, fx.As(new(service.AuthSessionRepository)))),

		// Auth dependencies
		fx.Provide(func(cfg *config.Config) service.TokenManager {
			return service.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience)
		}),
		fx.Provide(fx.Annotate(service.NewSessionService, fx.As(new(service.SessionManager)))),
		fx.Provide(func(cfg *config.Config) email.Mailer {
			if cfg.SMTPHost == "" {
				return nil
			}
			return email.NewSMTPMailer(email.SMTPConfig{
				Host:     cfg.SMTPHost,
				Port:     cfg.SMTPPort,
				Username: cfg.SMTPUsername,
				AuthCode: cfg.SMTPAuthCode,
				From:     cfg.SMTPFromEmail,
				FromName: cfg.SMTPFromName,
			})
		}),
		fx.Provide(service.NewEmailMailerAdapter),
		fx.Provide(func() service.Clock { return service.RealClock{} }),
		fx.Provide(func() service.CodeGenerator { return service.RealCodeGenerator{} }),
		fx.Provide(fx.Annotate(func(cfg *config.Config, rdb *redis.Client, mailer service.Mailer, clock service.Clock, codeGen service.CodeGenerator, userLook service.UserLookup) *service.VerificationService {
			return service.NewVerificationService(rdb, cfg.VerificationPepper, mailer, clock, codeGen, userLook)
		}, fx.As(new(service.VerificationManager)), fx.As(new(service.TicketVerifier)))),

		// Business services
		fx.Provide(service.NewAuthServiceV2),
		fx.Provide(newMonitorService),
		fx.Provide(service.NewNotifyService),
		fx.Provide(newReportService),
		fx.Provide(fx.Annotate(service.NewHotEventQueryService, fx.As(new(controller.HotEventManager)))),

		// HTTP server
		fx.Provide(NewHTTPServer),

		// LLM 内容聚合
		fx.Provide(fx.Annotate(service.NewLLMProvider, fx.As(new(service.LLMProvider)))),
		fx.Provide(fx.Annotate(service.NewLLMService, fx.As(new(service.LLMService)))),
		fx.Provide(service.NewLLMChain),

		// Daily obsidian publish worker
		fx.Provide(newDailyObsidianPublishJob),

		// Collection and aggregation repositories
		fx.Provide(repository.NewCollectRepo),
		fx.Provide(repository.NewTopicWriteRepo),
		fx.Provide(repository.NewSnapshotRepo),

		// Hourly aggregate worker
		fx.Provide(newHourlyAggregateJob),

		// Lifecycle hooks
		fx.Invoke(registerHooks),
	)
}

// HTTPServerIn groups dependencies for the HTTP server.
type HTTPServerIn struct {
	fx.In

	Config      *config.Config
	AuthService *service.AuthService
	UserRepo    service.UserRepository
	Rdb         *redis.Client
	MonitorSvc  *service.MonitorService
	NotifySvc   *service.NotifyService
	ReportSvc   *service.ReportService

	PostQuerySvc  content.PostQueryService
	TopicQuerySvc service.TopicQueryService
	TrendQuerySvc service.TrendQueryService
	HotEventMgr   controller.HotEventManager
}

func NewHTTPServer(in HTTPServerIn) *http.Server {
	smokeTest := os.Getenv("SMOKE_TEST") == "1"

	router := controller.NewRouter(controller.Config{
		JWTSecret:          in.Config.JWTSecret,
		JWTIssuer:          in.Config.JWTIssuer,
		JWTAudience:        in.Config.JWTAudience,
		SmokeTest:          smokeTest,
		SwaggerEnabled:     in.Config.SwaggerEnabled,
		WebAllowedOrigins:  in.Config.WebAllowedOrigins,
		AuthService:        in.AuthService,
		MonitorSvc:         in.MonitorSvc,
		NotifySvc:          in.NotifySvc,
		ReportSvc:          in.ReportSvc,
		PostQuerySvc:       in.PostQuerySvc,
		TopicQuerySvc:      in.TopicQuerySvc,
		TrendQuerySvc:      in.TrendQuerySvc,
		HotEventManager:    in.HotEventMgr,
		CookieDomain:       in.Config.CookieDomain,
		CookieSecure:       in.Config.CookieSecure,
	})

	return &http.Server{
		Addr:         in.Config.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func newReportService(repo service.ReportRepository) *service.ReportService {
	return service.NewReportService(repo, time.Now)
}

func newMonitorService(repo service.MonitorRepository) *service.MonitorService {
	return service.NewMonitorService(repo, nil)
}

func newHourlyAggregateJob(db *gorm.DB, collectRepo *repository.CollectRepo, topicWriteRepo *repository.TopicWriteRepo, snapshotRepo *repository.SnapshotRepo, runRepo worker.RunRepository) *worker.HourlyAggregateJob {
	return worker.NewHourlyAggregateJob(worker.HourlyAggregateDeps{
		DB:             db,
		CollectRepo:    collectRepo,
		TopicWriteRepo: topicWriteRepo,
		SnapshotRepo:   snapshotRepo,
		RunRepo:        runRepo,
		Now:            time.Now,
	})
}

func newDailyObsidianPublishJob(cfg *config.Config, monitorSvc *service.MonitorService, reportSvc *service.ReportService, exportRepo service.ExportRepository, runRepo worker.RunRepository) *worker.DailyObsidianPublishJob {
	return worker.NewDailyObsidianPublishJob(worker.DailyObsidianPublishDeps{
		VaultRoot: cfg.ObsidianVaultPath,
		Monitors:  monitorSvc,
		Reports:   reportSvc,
		Exports:   exportRepo,
		Runs:      runRepo,
		Now:       time.Now,
	})
}

func registerHooks(lc fx.Lifecycle, srv *http.Server, db *gorm.DB, cfg *config.Config, dailyJob *worker.DailyObsidianPublishJob, hourlyJob *worker.HourlyAggregateJob) {
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
			dispatcher.Register(hourlyJob)

			// --- DLQ recorder ---
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
			// Hourly aggregate cron
			_, err = cronS.AddFunc("0 * * * *", func() {
				now := time.Now().In(loc)
				payload, _ := json.Marshal(map[string]string{
					"target_hour": now.Format("2006-01-02T15:00"),
				})
				if pubErr := producer.Publish(context.Background(), queue.TopicHourlyRun, queue.NewMessage("hourly.run", payload)); pubErr != nil {
					logging.L().Error("cron publish hourly error", zap.Error(pubErr))
				}
			})
			if err != nil {
				return fmt.Errorf("cron: add hourly func: %w", err)
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
