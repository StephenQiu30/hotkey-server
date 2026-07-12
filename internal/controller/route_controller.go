package controller

import (
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/StephenQiu30/hotkey-server/docs"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// Config holds all dependencies for the Gin HTTP API.
type Config struct {
	JWTSecret          string
	JWTIssuer          string
	JWTAudience        string
	SmokeTest          bool
	SwaggerEnabled     bool
	WebAllowedOrigins  []string
	AuthService       *service.AuthService
	CookieDomain       string
	CookieSecure       bool
	MonitorSvc        *service.MonitorService
	NotifySvc         *service.NotifyService
	ReportSvc         ReportService
	PostQuerySvc      content.PostQueryService
	TopicQuerySvc     service.TopicQueryService
	TrendQuerySvc     service.TrendQueryService
	HotEventManager   HotEventManager
}

func NewRouter(cfg Config) *gin.Engine {
	if !cfg.SmokeTest {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.HandleMethodNotAllowed = true

	// Global middleware stack (applied to all routes).
	r.Use(platformhttp.RecoverMiddleware())
	r.Use(platformhttp.CORSMiddleware(cfg.WebAllowedOrigins, cfg.SmokeTest))
	r.Use(platformhttp.SecurityHeadersMiddleware())
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.AccessLogMiddleware())
	r.Use(platformhttp.ContextMetadataMiddleware("http"))
	r.Use(platformhttp.ErrorHandlerMiddleware())

	// Public routes (no auth required).
	RegisterHealthRoutes(r)
	RegisterAuthRoutes(r, cfg.AuthService, cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience, cfg.CookieDomain, cfg.CookieSecure)
	if cfg.SwaggerEnabled {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	// Protected routes (auth required).
	protected := r.Group("")
	protected.Use(platformhttp.AuthMiddleware(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience, cfg.SmokeTest))

	RegisterMonitorRoutes(protected, cfg.MonitorSvc)
	RegisterContentRoutes(protected, cfg.PostQuerySvc, cfg.MonitorSvc)
	RegisterTopicRoutes(protected, cfg.TopicQuerySvc, cfg.MonitorSvc)
	RegisterTrendRoutes(protected, cfg.TrendQuerySvc, cfg.MonitorSvc, cfg.TopicQuerySvc)
	RegisterNotifyRoutes(protected, cfg.NotifySvc)
	RegisterReportRoutes(protected, cfg.ReportSvc)

	// HotEvent routes (optional — nil-safe).
	if cfg.HotEventManager != nil {
		RegisterTrendingRoutes(protected, cfg.HotEventManager)
	}

	// NoRoute / NoMethod handlers return unified envelope.
	r.NoRoute(platformhttp.NoRouteHandler())
	r.NoMethod(platformhttp.NoMethodHandler())

	// Global error handler catches unhandled AppError and panics.

	return r
}
