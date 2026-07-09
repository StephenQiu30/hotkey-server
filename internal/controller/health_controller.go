package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

func RegisterHealthRoutes(r *gin.Engine) {
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
		RespondOK(c, vo.HealthBody{Status: "ok"})
	}
}
