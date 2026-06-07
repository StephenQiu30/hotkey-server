package http

import (
	"context"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	"github.com/StephenQiu30/hotkey-server/internal/platform/adapter"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
	servicereport "github.com/StephenQiu30/hotkey-server/internal/service/report"
	servicerss "github.com/StephenQiu30/hotkey-server/internal/service/rss"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	adapterhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/adapter"
	adminhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/admin"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	azhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/authorization"
	channelhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/channel"
	hotspothandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/hotspot"
	reporthandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/report"
	rsshandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/rss"
	sourcehandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/source"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthService          *serviceauth.Service
	AuthorizationService *serviceauth.AuthorizationService
	ChannelService       *servicechannel.Service
	SourceService        *servicesource.Service
	AdminService         *serviceadmin.Service
	ScoringService       *servicehotspot.ScoringService
	ReportService        *servicereport.Service
	RSSService           *servicerss.Service
	AdapterRegistry      *adapter.Registry
}

func NewRouterWithDependencies(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)

	if deps.AuthService == nil {
		panic("AuthService is required")
	}
	if deps.ChannelService == nil {
		deps.ChannelService = servicechannel.NewService(servicechannel.NewMemoryRepository())
	}
	if deps.AdminService == nil {
		deps.AdminService = serviceadmin.NewService(serviceadmin.NewMemoryRepository(), serviceadmin.Config{
			PostgreSQLPing: func(context.Context) error { return nil },
			RedisPing:      func(context.Context) error { return nil },
		})
	}
	var reportRepo servicereport.ReportRepository
	if deps.ReportService == nil {
		reportRepo = servicereport.NewMemoryReportRepository()
		deps.ReportService = servicereport.NewService(reportRepo, nil, nil, nil, nil, nil)
	} else {
		reportRepo = deps.ReportService.Repository()
	}
	if deps.RSSService == nil {
		deps.RSSService = servicerss.NewService(servicerss.NewMemoryFeedRepository(), reportRepo, servicerss.Config{})
	}
	if deps.AuthorizationService == nil {
		panic("AuthorizationService is required")
	}

	auth := authhandler.New(deps.AuthService)
	authorizations := azhandler.New(deps.AuthorizationService)
	channels := channelhandler.New(deps.ChannelService)
	sourceService := servicesource.NewService(servicesource.NewMemoryRepository())
	if deps.SourceService != nil {
		sourceService = deps.SourceService
	}
	sources := sourcehandler.New(sourceService)
	if deps.ScoringService == nil {
		clusterRepo := domainhotspot.NewMemoryRepository()
		scoreRepo := servicehotspot.NewMemoryScoreRepository()
		deps.ScoringService = servicehotspot.NewScoringService(servicehotspot.ScoringConfig{}, clusterRepo, scoreRepo)
	}
	hotspots := hotspothandler.New(deps.ScoringService)
	reports := reporthandler.New(deps.ReportService)
	rss := rsshandler.New(deps.RSSService)

	if deps.AdapterRegistry == nil {
		deps.AdapterRegistry = adapter.NewRegistry()
	}
	adapterHandler := adapterhandler.New(deps.AdapterRegistry)

	adminObservability := adminhandler.New(deps.AdminService)
	router.GET("/rss/channels/:channelCode", rss.PublicChannel)
	router.GET("/rss/users/:token", rss.PrivateUser)

	v1 := router.Group("/api/v1")
	v1.POST("/auth/register", auth.Register)
	v1.POST("/auth/login", auth.Login)
	v1.POST("/auth/refresh", auth.Refresh)
	v1.POST("/auth/logout", auth.Logout)
	v1.GET("/me", auth.AuthRequired(), auth.Me)
	v1.GET("/channels", channels.ListChannels)
	v1.GET("/me/channels", auth.AuthRequired(), channels.ListSubscriptions)
	v1.POST("/me/channels/:channelID", auth.AuthRequired(), channels.Subscribe)
	v1.DELETE("/me/channels/:channelID", auth.AuthRequired(), channels.Unsubscribe)
	v1.GET("/me/keywords", auth.AuthRequired(), channels.ListKeywords)
	v1.POST("/me/keywords", auth.AuthRequired(), channels.CreateKeyword)
	v1.PATCH("/me/keywords/:keywordID", auth.AuthRequired(), channels.UpdateKeyword)
	v1.DELETE("/me/keywords/:keywordID", auth.AuthRequired(), channels.DeleteKeyword)
	v1.PUT("/me/preferences/daily-send-at", auth.AuthRequired(), channels.SetUserDailySendAt)
	v1.GET("/hotspots", auth.AuthRequired(), hotspots.ListHotspots)
	v1.GET("/hotspots/:hotspotID", auth.AuthRequired(), hotspots.GetHotspot)
	v1.GET("/reports", auth.AuthRequired(), reports.ListReports)
	v1.GET("/reports/:reportID", auth.AuthRequired(), reports.GetReport)
	v1.GET("/me/rss", auth.AuthRequired(), rss.GetUserFeed)
	v1.POST("/me/rss/reset", auth.AuthRequired(), rss.ResetUserFeed)
	v1.DELETE("/me/rss", auth.AuthRequired(), rss.DisableUserFeed)

	// Authorization endpoints
	v1.POST("/authorizations/connect", auth.AuthRequired(), authorizations.Connect)
	v1.GET("/authorizations", auth.AuthRequired(), authorizations.List)
	v1.POST("/authorizations/:authorizationID/test", auth.AuthRequired(), authorizations.Test)
	v1.DELETE("/authorizations/:authorizationID", auth.AuthRequired(), authorizations.Disconnect)
	v1.DELETE("/me", auth.AuthRequired(), authorizations.DeleteAccount)

	admin := v1.Group("/admin", auth.AdminRequired(), adminObservability.AuditMiddleware())
	admin.GET("/healthz", auth.AdminHealthz)
	admin.GET("/config/status", adminObservability.ConfigStatus)
	admin.GET("/audit-logs", adminObservability.ListAuditLogs)
	admin.GET("/jobs", adminObservability.QueueOverview)
	admin.GET("/jobs/failed", adminObservability.ListFailedJobs)
	admin.GET("/jobs/:jobID", adminObservability.JobDetail)
	admin.POST("/jobs/:jobID/retry", adminObservability.RetryJob)
	admin.POST("/daily-reports/rerun", adminObservability.RerunDailyReport)
	admin.GET("/channels", channels.ListChannels)
	admin.POST("/channels", channels.CreateChannel)
	admin.PATCH("/channels/:channelID", channels.UpdateChannel)
	admin.DELETE("/channels/:channelID", channels.DeleteChannel)
	admin.PUT("/settings/default-daily-send-at", channels.SetDefaultDailySendAt)
	admin.GET("/sources", sources.ListSources)
	admin.POST("/sources", sources.CreateSource)
	admin.PATCH("/sources/:sourceID", sources.UpdateSource)
	admin.PATCH("/sources/:sourceID/status", sources.SetSourceStatus)
	admin.GET("/sources/:sourceID/collection-runs", sources.ListCollectionRuns)
	admin.POST("/sources/:sourceID/test-fetch", sources.TestFetch)

	admin.GET("/adapters", adapterHandler.ListAdapters)
	admin.GET("/adapters/:provider/health", adapterHandler.GetAdapterHealth)
	admin.GET("/adapters/:provider/capabilities", adapterHandler.GetAdapterCapabilities)

	return router
}
