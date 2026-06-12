package main

import (
	"log"
	"net/http"

	"github.com/stephenqiu/hotkey-server/internal/auth"
	"github.com/stephenqiu/hotkey-server/internal/config"
	"github.com/stephenqiu/hotkey-server/internal/monitor"
	"github.com/stephenqiu/hotkey-server/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// TODO: wire real PostgreSQL repository implementations
	// For now, the server starts but requires database-backed repos for full functionality
	authSvc := auth.NewService(nil, cfg.JWTSecret)
	authHandler := auth.NewHTTPHandler(authSvc)

	monitorSvc := monitor.NewService(nil)
	monitorHandler := monitor.NewHTTPHandler(monitorSvc)

	router := server.NewRouter(server.Dependencies{
		AuthHandler:    authHandler,
		MonitorHandler: monitorHandler,
		JWTSecret:      cfg.JWTSecret,
	})

	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, router); err != nil {
		log.Fatalf("server: %v", err)
	}
}
