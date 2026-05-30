package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)
	return router
}
