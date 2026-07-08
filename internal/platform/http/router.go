package http

import (
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/StephenQiu30/hotkey-server/docs"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// Config holds all dependencies for the Gin HTTP API.
type Config struct {
	JWTSecret       string
	SmokeTest       bool
	SwaggerEnabled  bool
	AuthService     *service.AuthService
	MonitorSvc      *service.MonitorService
	NotifySvc       *service.NotifyService
	ReportSvc       ReportService
	PostQuerySvc    content.PostQueryService
	TopicQuerySvc   service.TopicQueryService
	TrendQuerySvc   service.TrendQueryService
	HotEventManager HotEventManager
}

func NewRouter(cfg Config) *gin.Engine {
	if !cfg.SmokeTest {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()

	r.Use(RecoverMiddleware())
	r.Use(CORSMiddleware())
	r.Use(SecurityHeadersMiddleware())
	r.Use(RequestIDMiddleware())
	r.Use(AccessLogMiddleware())
	r.Use(ContextMetadataMiddleware("http"))
	r.Use(AuthMiddleware(cfg.JWTSecret, cfg.SmokeTest))

	if cfg.SwaggerEnabled {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	RegisterHealthRoutes(r)
	RegisterAuthRoutes(r, cfg.AuthService, cfg.JWTSecret)
	RegisterMonitorRoutes(r, cfg.MonitorSvc)
	RegisterContentRoutes(r, cfg.PostQuerySvc, cfg.MonitorSvc)
	RegisterTopicRoutes(r, cfg.TopicQuerySvc, cfg.MonitorSvc)
	RegisterTrendRoutes(r, cfg.TrendQuerySvc, cfg.MonitorSvc, cfg.TopicQuerySvc)
	RegisterNotifyRoutes(r, cfg.NotifySvc)
	RegisterReportRoutes(r, cfg.ReportSvc)

	// HotEvent routes (optional — nil-safe)
	if cfg.HotEventManager != nil {
		RegisterTrendingRoutes(r, cfg.HotEventManager)
	}

	return r
}
