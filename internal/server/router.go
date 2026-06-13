// Package server provides the legacy HTTP router and middleware.
//
// Deprecated: This package is superseded by internal/platform/http which uses
// Huma v2 for route registration, middleware, and OpenAPI generation. Retained
// only for backward compatibility with existing tests and domain packages that
// import server.UserIDKey.
package server

import (
	"net/http"
)

// Dependencies holds injected handlers and middleware for the router.
//
// Deprecated: Use internal/platform/http.Config with huma.API instead.
type Dependencies struct {
	AuthHandler         http.Handler
	MonitorHandler      http.Handler
	TopicHandler        http.Handler
	TrendHandler        http.Handler
	PostHandler         http.Handler
	NotificationHandler http.Handler
	AuthMiddleware      func(http.Handler) http.Handler
}

// NewRouter creates the application HTTP router with all routes mounted.
func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", Health)

	if deps.AuthHandler != nil {
		mux.Handle("POST /api/v1/auth/register", deps.AuthHandler)
		mux.Handle("POST /api/v1/auth/login", deps.AuthHandler)
	}

	if deps.MonitorHandler != nil && deps.AuthMiddleware != nil {
		mux.Handle("GET /api/v1/monitors", deps.AuthMiddleware(deps.MonitorHandler))
		mux.Handle("POST /api/v1/monitors", deps.AuthMiddleware(deps.MonitorHandler))
		mux.Handle("GET /api/v1/monitors/{id}", deps.AuthMiddleware(deps.MonitorHandler))
		mux.Handle("PATCH /api/v1/monitors/{id}", deps.AuthMiddleware(deps.MonitorHandler))
	}

	// Content flow: GET /api/v1/monitors/{id}/posts
	if deps.PostHandler != nil && deps.AuthMiddleware != nil {
		mux.Handle("GET /api/v1/monitors/{id}/posts", deps.AuthMiddleware(deps.PostHandler))
	}

	// Topic list: GET /api/v1/monitors/{id}/topics
	if deps.TopicHandler != nil && deps.AuthMiddleware != nil {
		mux.Handle("GET /api/v1/monitors/{id}/topics", deps.AuthMiddleware(deps.TopicHandler))
	}

	// Trend endpoints
	if deps.TrendHandler != nil && deps.AuthMiddleware != nil {
		mux.Handle("GET /api/v1/monitors/{id}/trends", deps.AuthMiddleware(deps.TrendHandler))
		mux.Handle("GET /api/v1/topics/{id}/trends", deps.AuthMiddleware(deps.TrendHandler))
	}

	// Notification endpoints
	if deps.NotificationHandler != nil && deps.AuthMiddleware != nil {
		mux.Handle("GET /api/v1/notifications", deps.AuthMiddleware(deps.NotificationHandler))
		mux.Handle("POST /api/v1/notifications/{id}/read", deps.AuthMiddleware(deps.NotificationHandler))
	}

	return mux
}
