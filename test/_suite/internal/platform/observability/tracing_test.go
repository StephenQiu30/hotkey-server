package observability

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

func TestTelemetryUsesHotKeyResource(t *testing.T) {
	telemetry, err := NewTelemetry(config.Default())
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}
	defer func() { _ = telemetry.Shutdown(context.Background()) }()

	if got := telemetry.ServiceName(); got != "hotkey-server" {
		t.Errorf("service name = %q, want hotkey-server", got)
	}
}

func TestTelemetryShutsDownExporterThroughFxLifecycle(t *testing.T) {
	exporter := &shutdownExporter{}
	telemetry, err := NewTelemetryWithExporter(config.Default(), exporter)
	if err != nil {
		t.Fatalf("NewTelemetryWithExporter() error = %v", err)
	}
	app := fx.New(fx.Supply(telemetry), fx.Invoke(RegisterLifecycle))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("app.Start() error = %v", err)
	}
	if err := app.Stop(ctx); err != nil {
		t.Fatalf("app.Stop() error = %v", err)
	}
	if !exporter.shutdown.Load() {
		t.Fatal("exporter was not shut down")
	}
}

type shutdownExporter struct {
	shutdown atomic.Bool
}

func (*shutdownExporter) ExportSpans(context.Context, []trace.ReadOnlySpan) error {
	return nil
}

func (exporter *shutdownExporter) Shutdown(context.Context) error {
	exporter.shutdown.Store(true)
	return nil
}
