package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterHealthRoutes registers the /healthz endpoint.
func RegisterHealthRoutes(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthBody{Status: "ok"})
	})
}

// HealthBody is the health check response body.
type HealthBody struct {
	Status string `json:"status"`
}
