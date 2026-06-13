package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"github.com/StephenQiu30/hotkey-server/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <api|worker>\n", os.Args[0])
		os.Exit(1)
	}

	switch os.Args[1] {
	case "api":
		runAPI()
	case "worker":
		runWorker()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runAPI() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("api", "starting"))

	// Connect to database.
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// Wire auth with real Postgres repository.
	authRepo := database.NewAuthRepo(db)
	authSvc := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authSvc, cfg.JWTSecret)

	// Wire monitor with real Postgres repository.
	monitorRepo := database.NewMonitorRepo(db)
	monitorSvc := monitor.NewService(monitorRepo)
	monitorHandler := monitor.NewHandler(monitorSvc)

	// Wire notification with real Postgres repository.
	notifyRepo := database.NewNotifyRepo(db)
	notifySvc := notify.NewService(notifyRepo)
	notifyHandler := notify.NewHandler(notifySvc)

	// Auth middleware: validates JWT token and injects user ID into context.
	authMiddleware := server.AuthMiddleware(cfg.JWTSecret)

	router := server.NewRouter(server.Dependencies{
		AuthHandler:         authHandler,
		MonitorHandler:      monitorHandler,
		NotificationHandler: notifyHandler,
		AuthMiddleware:      authMiddleware,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Print(observability.RenderLog("api", "shutting down"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}()

	log.Print(observability.RenderLog("api", "listening on "+cfg.HTTPAddr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func runWorker() {
	_, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Print(observability.RenderLog("worker", "starting"))

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// TODO: Register background jobs (poll_monitor, aggregate_topics, build_snapshots, dispatch_notifications)
	// when the jobs module is wired with real repositories. See internal/jobs/.
	log.Print(observability.RenderLog("worker", "ready, waiting for jobs"))

	<-sigCh
	log.Print(observability.RenderLog("worker", "shutting down"))
}
