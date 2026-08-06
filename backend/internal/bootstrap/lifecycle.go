package bootstrap

import (
	"context"
	"fmt"

	"go.uber.org/fx"
)

func startApp(ctx context.Context, app *fx.App) error {
	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start application: %w", err)
	}
	return nil
}

func stopApp(ctx context.Context, app *fx.App) error {
	if err := app.Stop(ctx); err != nil {
		return fmt.Errorf("stop application: %w", err)
	}
	return nil
}
