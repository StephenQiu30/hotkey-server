package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

func RegisterHealthRoutes(r gin.IRouter) {
	r.GET("/healthz", healthHandler())
}

// healthHandler godoc
// @Summary Health check
// @ID health-check
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /healthz [get]
func healthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		platformhttp.RespondOK(c, vo.HealthBody{Status: "ok"})
	}
}
