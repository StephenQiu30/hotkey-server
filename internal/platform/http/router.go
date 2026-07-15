package http

import (
	"context"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Readiness interface {
	Check(context.Context) error
}

type ReadinessFunc func(context.Context) error

func (fn ReadinessFunc) Check(ctx context.Context) error {
	return fn(ctx)
}

type healthData struct {
	Status string `json:"status"`
}

func NewRouter(readiness Readiness) *gin.Engine {
	router := gin.New()
	router.Use(requestID(), recovery())
	router.GET("/healthz", func(c *gin.Context) {
		OK(c, healthData{Status: "ok"})
	})
	router.GET("/readyz", func(c *gin.Context) {
		if err := readiness.Check(c.Request.Context()); err != nil {
			Fail(c, stdhttp.StatusServiceUnavailable, 90001, "service not ready")
			return
		}
		OK(c, healthData{Status: "ok"})
	})
	return router
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, _ any) {
		Fail(c, stdhttp.StatusInternalServerError, 90000, "internal server error")
	})
}
