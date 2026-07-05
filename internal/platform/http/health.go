package http

import "github.com/gin-gonic/gin"

func RegisterHealthRoutes(r *gin.Engine) {
	r.GET("/healthz", healthHandler())
}

type HealthBody struct {
	Status string `json:"status"`
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
		RespondOK(c, HealthBody{Status: "ok"})
	}
}
