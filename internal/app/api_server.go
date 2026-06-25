package app

import (
	"net/http"
	"os"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"gorm.io/gorm"
)

func newAPIServer(cfg config.Config, db *gorm.DB) (*http.Server, error) {
	smokeTest := os.Getenv("SMOKE_TEST") == "1"

	var authRepo auth.Repository
	var monitorRepo monitor.Repository
	var notifyRepo notify.Repository
	var postQuerySvc content.PostQueryService
	var topicQuerySvc topic.TopicQueryService
	var trendQuerySvc trend.TrendQueryService

	if smokeTest {
		authRepo = &smokeAuthRepo{}
		monitorRepo = &smokeMonitorRepo{}
		notifyRepo = &smokeNotifyRepo{}
		postQuerySvc = &smokePostQueryService{}
		topicQuerySvc = &smokeTopicQueryService{}
		trendQuerySvc = &smokeTrendQueryService{}
	} else {
		authRepo = database.NewAuthRepo(db)
		monitorRepo = database.NewMonitorRepo(db)
		notifyRepo = database.NewNotifyRepo(db)
		postQuerySvc = database.NewContentQueryService(db)
		topicQuerySvc = database.NewTopicQueryService(db)
		trendQuerySvc = database.NewTrendQueryService(db)
	}

	router := platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:     cfg.JWTSecret,
		SmokeTest:     smokeTest,
		AuthService:   auth.NewService(authRepo),
		MonitorSvc:    monitor.NewService(monitorRepo),
		NotifySvc:     notify.NewService(notifyRepo),
		PostQuerySvc:  postQuerySvc,
		TopicQuerySvc: topicQuerySvc,
		TrendQuerySvc: trendQuerySvc,
	})

	return &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}, nil
}
