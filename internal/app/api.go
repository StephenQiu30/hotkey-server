package app

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
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/server"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"go.uber.org/fx"
)

// NewAPIApp constructs the Fx application for the API server.
func NewAPIApp(cfg config.Config) *fx.App {
	return fx.New(
		fx.Supply(cfg),
		fx.Invoke(startAPIServer),
	)
}

func startAPIServer(lc fx.Lifecycle, cfg config.Config) {
	log.Print(observability.RenderLog("api", "starting"))

	smokeTest := os.Getenv("SMOKE_TEST") == "1"

	var authRepo auth.Repository
	var monitorRepo monitor.Repository
	var notifyRepo notify.Repository

	if smokeTest {
		authRepo = &smokeAuthRepo{}
		monitorRepo = &smokeMonitorRepo{}
		notifyRepo = &smokeNotifyRepo{}
	} else {
		db, err := database.Open(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("failed to connect to database: %v", err)
		}
		authRepo = database.NewAuthRepo(db)
		monitorRepo = database.NewMonitorRepo(db)
		notifyRepo = database.NewNotifyRepo(db)

		lc.Append(fx.Hook{
			OnStop: func(context.Context) error {
				return db.Close()
			},
		})
	}

	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc, cfg.JWTSecret)
	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)
	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)
	postHandler := content.NewPostHandler(&stubPostQueryService{})
	topicHandler := topic.NewTopicHandler(&stubTopicQueryService{})
	trendHandler := trend.NewTrendHandler(&stubTrendQueryService{})

	authMiddleware := func(next http.Handler) http.Handler {
		if smokeTest {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := monitor.ContextWithUserID(r.Context(), 1)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		}
		return server.AuthMiddleware(cfg.JWTSecret)(next)
	}

	router := server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		TopicHandler:        topicHandler,
		TrendHandler:        trendHandler,
		PostHandler:         postHandler,
		NotificationHandler: notifyHandler,
		AuthMiddleware:      authMiddleware,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				log.Print(observability.RenderLog("api", "listening on "+cfg.HTTPAddr))
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Print(observability.RenderLog("api", "shutting down"))
			return srv.Shutdown(ctx)
		},
	})
}
