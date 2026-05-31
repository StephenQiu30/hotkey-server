package http

import (
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	servicesource "github.com/StephenQiu30/hotkey-server/internal/service/source"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	channelhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/channel"
	sourcehandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/source"
	"github.com/gin-gonic/gin"
)

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
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)

	auth := authhandler.New(authService)
	channels := channelhandler.New(channelService)
	sourceService := servicesource.NewService(servicesource.NewMemoryRepository())
	if len(sourceServices) > 0 && sourceServices[0] != nil {
		sourceService = sourceServices[0]
	}
	sources := sourcehandler.New(sourceService)
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

	admin := v1.Group("/admin", auth.AdminRequired())
	admin.GET("/healthz", auth.AdminHealthz)
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
