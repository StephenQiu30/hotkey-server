package http

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/observability"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

func NewRouter(readiness Readiness, metrics *observability.Metrics, telemetry *observability.Telemetry, logger *zap.Logger, cfg config.Config) *gin.Engine {
	router := gin.New()
	router.Use(
		requestID(),
		traceContext(telemetry),
		accessLog(logger, metrics),
		recovery(logger, metrics),
		cors(cfg.Authentication.AllowedOrigins),
		requestContextTimeout(cfg.RequestTimeout),
	)
	router.GET("/healthz", Wrap(func(c *gin.Context) error {
		OK(c, healthData{Status: "ok"})
		return nil
	}))
	router.GET("/readyz", Wrap(func(c *gin.Context) error {
		if err := readiness.Check(c.Request.Context()); err != nil {
			metrics.SetDependencyHealth("runtime", 0)
			return sharederrors.Wrap(sharederrors.CodeUnavailable, 503, "service not ready", err)
		}
		metrics.SetDependencyHealth("runtime", 1)
		OK(c, healthData{Status: "ok"})
		return nil
	}))
	router.GET("/metrics", gin.WrapH(metrics.Handler()))
	if cfg.Environment != "production" {
		registerAPIDocumentation(router)
	}
	api := router.Group("/api/v1")
	api.GET("/capabilities", Wrap(CapabilitiesHandler))
	return router
}
