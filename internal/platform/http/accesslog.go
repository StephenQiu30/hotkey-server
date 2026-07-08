package http

import (
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AccessLogMiddleware logs each HTTP request with structured fields.
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		method := c.Request.Method

		logging.Ctx(c.Request.Context()).Info("access",
			zap.String("method", method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.Int("size", c.Writer.Size()),
			zap.String("client_ip", c.ClientIP()),
		)
	}
}
