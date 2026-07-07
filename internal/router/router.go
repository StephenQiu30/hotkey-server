package router

import (
	"github.com/gin-gonic/gin"
	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// Config holds all dependencies for route registration.
type Config struct {
	JWTSecret       string
	SmokeTest       bool
	SwaggerEnabled  bool
	AuthService     *auth.Service
	MonitorSvc      *monitor.Service
	NotifySvc       *notify.Service
	PostQuerySvc    content.PostQueryService
	TopicQuerySvc   topic.TopicQueryService
	TrendQuerySvc   trend.TrendQueryService
	HotEventManager platformhttp.HotEventManager
}

// New creates a Gin engine with all routes registered.
func New(cfg Config) *gin.Engine {
	return platformhttp.NewRouter(platformhttp.Config{
		JWTSecret:       cfg.JWTSecret,
		SmokeTest:       cfg.SmokeTest,
		SwaggerEnabled:  cfg.SwaggerEnabled,
		AuthService:     cfg.AuthService,
		MonitorSvc:      cfg.MonitorSvc,
		NotifySvc:       cfg.NotifySvc,
		PostQuerySvc:    cfg.PostQuerySvc,
		TopicQuerySvc:   cfg.TopicQuerySvc,
		TrendQuerySvc:   cfg.TrendQuerySvc,
		HotEventManager: cfg.HotEventManager,
	})
}
