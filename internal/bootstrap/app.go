package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
)

func NewApp(cfg config.Config, logger *zap.Logger) (*fx.App, error) {
	return NewAppWithReadiness(cfg, logger, httptransport.ReadinessFunc(func(context.Context) error { return nil }))
}

// NewAppWithReadiness makes the aggregate lifecycle check injectable. Runtime
// packages register their required dependencies here as they are introduced.
func NewAppWithReadiness(cfg config.Config, logger *zap.Logger, readiness httptransport.Readiness, extra ...fx.Option) (*fx.App, error) {
	role, err := ParseRole(cfg.Role)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	options := []fx.Option{
		fx.Supply(cfg, logger),
		fx.WithLogger(func() fxevent.Logger { return &fxevent.ZapLogger{Logger: logger} }),
	}
	usesDatabase := strings.TrimSpace(cfg.DatabaseURL) != ""
	if usesDatabase {
		options = append(options, fx.Provide(database.NewRuntime), fx.Invoke(database.RegisterLifecycle))
	}
	if role.StartsAPI() {
		if readiness == nil {
			return nil, fmt.Errorf("api readiness check is required")
		}
		readinessProvider := fx.Provide(func() httptransport.Readiness { return readiness })
		if usesDatabase {
			readinessProvider = fx.Provide(func(runtime *database.Runtime) httptransport.Readiness {
				return httptransport.ReadinessFunc(func(ctx context.Context) error {
					if err := readiness.Check(ctx); err != nil {
						return err
					}
					return runtime.Ping(ctx)
				})
			})
		}
		options = append(options,
			readinessProvider,
			fx.Provide(httptransport.NewRouter, httptransport.NewServer),
			fx.Invoke(httptransport.RegisterServer),
		)
	}
	if role.StartsWorker() {
		options = append(options, fx.Invoke(registerWorkerLifecycle))
	}
	options = append(options, extra...)

	return fx.New(options...), nil
}

func Run(ctx context.Context, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if len(args) > 0 && args[0] == "db" {
		return runDatabaseCommand(ctx, cfg, args[1:])
	}
	if err := applyCommandLine(&cfg, args); err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	logger, err := logging.New(cfg.Environment)
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	app, err := NewApp(cfg, logger)
	if err != nil {
		return fmt.Errorf("build application: %w", err)
	}

	startCtx, cancelStart := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancelStart()
	if err := startApp(startCtx, app); err != nil {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelCleanup()
		_ = stopApp(cleanupCtx, app)
		return err
	}

	<-ctx.Done()
	stopCtx, cancelStop := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancelStop()
	if err := stopApp(stopCtx, app); err != nil {
		return err
	}
	return nil
}

func applyCommandLine(cfg *config.Config, args []string) error {
	if len(args) > 0 && args[0] != "serve" {
		return fmt.Errorf("unknown command %q: expected serve", args[0])
	}
	if len(args) > 0 {
		args = args[1:]
	}

	flags := flag.NewFlagSet("hotkey serve", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	flags.StringVar(&cfg.Role, "role", cfg.Role, "runtime role: all, api, or worker")
	flags.StringVar(&cfg.HTTPAddr, "http-addr", cfg.HTTPAddr, "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse serve flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	return nil
}

func registerWorkerLifecycle(lifecycle fx.Lifecycle, logger *zap.Logger) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			logger.Info("worker runtime started")
			return nil
		},
		OnStop: func(context.Context) error {
			logger.Info("worker runtime stopped")
			return nil
		},
	})
}

type discardWriter struct{}

func (*discardWriter) Write(data []byte) (int, error) {
	return len(data), nil
}
