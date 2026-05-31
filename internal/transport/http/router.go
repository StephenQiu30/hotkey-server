package http

import (
	"context"

	domainhotspot "github.com/StephenQiu30/hotkey-server/internal/domain/hotspot"
	serviceadmin "github.com/StephenQiu30/hotkey-server/internal/service/admin"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	servicehotspot "github.com/StephenQiu30/hotkey-server/internal/service/hotspot"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	adminhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/admin"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	channelhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/channel"
	hotspothandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/hotspot"
	sourcehandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/source"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthService    *serviceauth.Service
	ChannelService *servicechannel.Service
	SourceService  *servicesource.Service
	AdminService   *serviceadmin.Service
	ScoringService *servicehotspot.ScoringService
}

func NewRouter() *gin.Engine {
	authService, err := serviceauth.NewService(serviceauth.NewMemoryRepository(), serviceauth.Config{
		AccessTokenSecret: "test-router-secret",
	})
	if err != nil {
		panic(err)
	}
	return NewRouterWithServices(authService, servicechannel.NewService(servicechannel.NewMemoryRepository()))
}

func NewRouterWithAuth(authService *serviceauth.Service) *gin.Engine {
	return NewRouterWithServices(authService, servicechannel.NewService(servicechannel.NewMemoryRepository()))
}

func NewRouterWithServices(authService *serviceauth.Service, channelService *servicechannel.Service, sourceServices ...*servicesource.Service) *gin.Engine {
	deps := Dependencies{AuthService: authService, ChannelService: channelService}
	if len(sourceServices) > 0 {
		deps.SourceService = sourceServices[0]
	}
	return NewRouterWithDependencies(deps)
}

func NewRouterWithDependencies(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)

	if deps.AuthService == nil {
		authService, err := serviceauth.NewService(serviceauth.NewMemoryRepository(), serviceauth.Config{
			AccessTokenSecret: "test-router-secret",
		})
		if err != nil {
			panic(err)
		}
		deps.AuthService = authService
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

	auth := authhandler.New(deps.AuthService)
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

	adminObservability := adminhandler.New(deps.AdminService)
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

	return router
}
