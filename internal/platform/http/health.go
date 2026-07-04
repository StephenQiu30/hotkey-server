package http

import "github.com/gin-gonic/gin"

// RegisterHealthRoutes registers the /healthz endpoint.
func RegisterHealthRoutes(r *gin.Engine) {
	r.GET("/healthz", healthHandler())
}

// HealthBody is the health check response body.
type HealthBody struct {
	Status string `json:"status"`
}

// healthHandler godoc
// @Summary Health check
// @ID health-check
// @Tags health
// @Produce json
// @Success 200 {object} HealthEnvelope
// @Router /healthz [get]
func healthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondOK(c, HealthBody{Status: "ok"})
	}
}
