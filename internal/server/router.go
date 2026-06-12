package server

import (
	"encoding/json"
	"net/http"

	"github.com/stephenqiu/hotkey-server/internal/auth"
	"github.com/stephenqiu/hotkey-server/internal/monitor"
)

type Dependencies struct {
	AuthHandler    *auth.HTTPHandler
	MonitorHandler *monitor.HTTPHandler
	JWTSecret      string
}

func NewRouter(deps Dependencies) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Health)

	// Auth routes (public)
	mux.HandleFunc("POST /api/v1/auth/register", deps.AuthHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", deps.AuthHandler.Login)

	// Monitor routes (protected)
	authMw := AuthMiddleware(deps.JWTSecret)
	mux.Handle("POST /api/v1/monitors", authMw(http.HandlerFunc(deps.MonitorHandler.Create)))
	mux.Handle("GET /api/v1/monitors", authMw(http.HandlerFunc(deps.MonitorHandler.List)))
	mux.Handle("GET /api/v1/monitors/{id}", authMw(http.HandlerFunc(deps.MonitorHandler.Get)))
	mux.Handle("PATCH /api/v1/monitors/{id}", authMw(http.HandlerFunc(deps.MonitorHandler.Update)))
	mux.Handle("DELETE /api/v1/monitors/{id}", authMw(http.HandlerFunc(deps.MonitorHandler.Deactivate)))

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
