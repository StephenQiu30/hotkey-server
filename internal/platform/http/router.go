package http

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// Config holds all dependencies for the Gin HTTP API.
type Config struct {
	JWTSecret     string
	SmokeTest     bool
	AuthService   *auth.Service
	MonitorSvc    *monitor.Service
	NotifySvc     *notify.Service
	PostQuerySvc  content.PostQueryService
	TopicQuerySvc topic.TopicQueryService
	TrendQuerySvc trend.TrendQueryService
}

// NewRouter creates a Gin engine with middleware and all routes registered.
func NewRouter(cfg Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(RecoverMiddleware())
	r.Use(RequestIDMiddleware())
	r.Use(AuthMiddleware(cfg.JWTSecret, cfg.SmokeTest))

	RegisterHealthRoutes(r)
	RegisterAuthRoutes(r, cfg.AuthService, cfg.JWTSecret)
	RegisterMonitorRoutes(r, cfg.MonitorSvc)
	RegisterContentRoutes(r, cfg.PostQuerySvc)
	RegisterTopicRoutes(r, cfg.TopicQuerySvc)
	RegisterTrendRoutes(r, cfg.TrendQuerySvc)
	RegisterNotifyRoutes(r, cfg.NotifySvc)

	r.GET("/openapi.json", func(c *gin.Context) {
		c.JSON(200, BuildOpenAPISpec())
	})

	return r
}

// NewAPI is an alias for NewRouter for backward compatibility in tests.
func NewAPI(cfg Config) (*gin.Engine, *gin.Engine) {
	r := NewRouter(cfg)
	return r, r
}
