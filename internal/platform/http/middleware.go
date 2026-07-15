package http

import (
	"context"
	"runtime/debug"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const requestIDContextKey = "hotkey.request_id"

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		value := c.GetHeader("X-Request-ID")
		if !validRequestID(value) {
			value = uuid.NewString()
		}
		c.Set(requestIDContextKey, value)
		c.Header("X-Request-ID", value)
		c.Next()
	}
}

func traceContext(telemetry *observability.Telemetry) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		tracer := trace.NewNoopTracerProvider().Tracer("hotkey-server/http")
		if telemetry != nil && telemetry.TracerProvider != nil {
			tracer = telemetry.TracerProvider.Tracer("hotkey-server/http")
		}
		requestContext, span := tracer.Start(requestContext, c.Request.Method)
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
		span.SetAttributes(
			attribute.String("http.request_id", RequestID(c)),
			attribute.String("http.request.method", c.Request.Method),
			attribute.String("http.route", requestRoute(c)),
			attribute.Int("http.response.status_code", c.Writer.Status()),
		)
		span.End()
	}
}

func accessLog(logger *zap.Logger, metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		route := requestRoute(c)
		status := c.Writer.Status()
		duration := time.Since(started)
		metrics.RecordHTTPRequest(c.Request.Method, route, status, duration)
		logger.Info("HTTP request completed",
			zap.String("request_id", RequestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", status),
			zap.Duration("duration", duration),
		)
	}
}

func recovery(logger *zap.Logger, metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			route := requestRoute(c)
			metrics.RecordPanic(route)
			logger.Error("HTTP panic recovered",
				zap.String("request_id", RequestID(c)),
				zap.String("route", route),
				zap.ByteString("stack", debug.Stack()),
			)
			if !c.Writer.Written() {
				WriteError(c, nil)
			}
		}()
		c.Next()
	}
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func requestContextTimeout(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestContext, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(requestContext)
		c.Next()
	}
}

func authenticationPassthrough() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}

func authorizationPassthrough() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}

func RequestID(c *gin.Context) string {
	value, _ := c.Get(requestIDContextKey)
	requestID, _ := value.(string)
	return requestID
}

func requestRoute(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "unmatched"
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 33 || character > 126 {
			return false
		}
	}
	return true
}
