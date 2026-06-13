package http

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// Config holds all dependencies for the Huma-based HTTP API.
type Config struct {
	JWTSecret     string
	SmokeTest     bool
	AuthService   *auth.Service
	MonitorSvc    *monitor.Service
	NotifySvc     *notify.Service
	PostQuerySvc  content.PostQueryService
	TopicQuerySvc topic.TopicQueryService
	TrendQuerySvc trend.TrendQueryService
}

// NewAPI creates a Huma API instance with middleware and all routes registered.
// Returns both the huma.API and the underlying http.ServeMux.
func NewAPI(cfg Config) (huma.API, *http.ServeMux) {
	mux := http.NewServeMux()

	config := huma.DefaultConfig("HotKey Server", "1.0.0")
	config.Info.Description = "X (Twitter) hot-topic monitoring platform API"
	config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		},
	}

	api := humago.New(mux, config)

	// Set global API reference before registering middleware (closures capture
	// the variable, not the value, so it's available when middleware runs).
	globalAPI = api

	// Register global middleware.
	api.UseMiddleware(RecoverMiddleware())
	api.UseMiddleware(RequestIDMiddleware())
	api.UseMiddleware(AuthMiddleware(cfg.JWTSecret, cfg.SmokeTest))

	// Register routes for each domain.
	RegisterHealthRoutes(api)
	RegisterAuthRoutes(api, cfg.AuthService, cfg.JWTSecret)
	RegisterMonitorRoutes(api, cfg.MonitorSvc)
	RegisterContentRoutes(api, cfg.PostQuerySvc)
	RegisterTopicRoutes(api, cfg.TopicQuerySvc)
	RegisterTrendRoutes(api, cfg.TrendQuerySvc)
	RegisterNotifyRoutes(api, cfg.NotifySvc)

	return api, mux
}
