package http

import (
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	authhandler "github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers/auth"
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	authService, err := serviceauth.NewService(serviceauth.NewMemoryRepository(), serviceauth.Config{
		AccessTokenSecret: "test-router-secret",
	})
	if err != nil {
		panic(err)
	}
	return NewRouterWithAuth(authService)
}

func NewRouterWithAuth(authService *serviceauth.Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)

	auth := authhandler.New(authService)
	v1 := router.Group("/api/v1")
	v1.POST("/auth/register", auth.Register)
	v1.POST("/auth/login", auth.Login)
	v1.POST("/auth/refresh", auth.Refresh)
	v1.POST("/auth/logout", auth.Logout)
	v1.GET("/me", auth.AuthRequired(), auth.Me)

	admin := v1.Group("/admin", auth.AdminRequired())
	admin.GET("/healthz", auth.AdminHealthz)

	return router
}
