package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"github.com/StephenQiu30/hotkey-server/internal/observability"
	"gorm.io/gorm"
)

// App manages the HotKey server lifecycle (API + worker in one process).
type App struct {
	cfg    config.Config
	db     *gorm.DB
	server *http.Server
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Run starts the HotKey server.
func Run() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if os.Getenv("SMOKE_TEST") == "1" {
		runAPISmoke(cfg)
		return
	}

	app, err := New(cfg)
	if err != nil {
		log.Fatalf("failed to create app: %v", err)
	}

	if err := app.Start(); err != nil {
		log.Fatalf("failed to start app: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Print(observability.RenderLog("app", "shutting down"))
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := app.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// New creates an App with database and HTTP server wired.
func New(cfg config.Config) (*App, error) {
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	srv, err := newAPIServer(cfg, db)
	if err != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		return nil, err
	}

	return &App{cfg: cfg, db: db, server: srv}, nil
}

// Start launches the API server and background worker.
func (a *App) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		runWorkerWithDB(ctx, a.cfg, a.db)
	}()

	go func() {
		log.Print(observability.RenderLog("api", "listening on "+a.cfg.HTTPAddr))
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	log.Print(observability.RenderLog("app", "starting"))
	return nil
}

// Shutdown stops the worker and API server gracefully.
func (a *App) Shutdown(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if err := a.server.Shutdown(ctx); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if a.db != nil {
		if sqlDB, err := a.db.DB(); err == nil && sqlDB != nil {
			sqlDB.Close()
		}
	}
	return nil
}

func runAPISmoke(cfg config.Config) {
	log.Print(observability.RenderLog("api", "smoke test mode"))

	srv, err := newAPIServer(cfg, nil)
	if err != nil {
		log.Fatalf("failed to create api server: %v", err)
	}

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
