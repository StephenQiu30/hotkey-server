package observability

import (
	"context"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.uber.org/fx"
)

const serviceName = "hotkey-server"

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
	resource       *resource.Resource
}

func NewTelemetry(cfg config.Config) (*Telemetry, error) {
	endpoint := strings.TrimSpace(cfg.OTLPHTTPEndpoint)
	if endpoint == "" {
		return newTelemetry(nil)
	}
	exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, err
	}
	return newTelemetry(exporter)
}

// NewTelemetryWithExporter makes the lifecycle boundary testable without
// changing runtime configuration. Production callers use NewTelemetry.
func NewTelemetryWithExporter(_ config.Config, exporter sdktrace.SpanExporter) (*Telemetry, error) {
	return newTelemetry(exporter)
}

func newTelemetry(exporter sdktrace.SpanExporter) (*Telemetry, error) {
	resource, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource)}
	if exporter != nil {
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(provider)
	return &Telemetry{TracerProvider: provider, resource: resource}, nil
}

func (telemetry *Telemetry) ServiceName() string {
	if telemetry == nil || telemetry.resource == nil {
		return ""
	}
	value, ok := telemetry.resource.Set().Value(semconv.ServiceNameKey)
	if !ok {
		return ""
	}
	return value.AsString()
}

func (telemetry *Telemetry) Shutdown(ctx context.Context) error {
	if telemetry == nil || telemetry.TracerProvider == nil {
		return nil
	}
	if err := telemetry.TracerProvider.ForceFlush(ctx); err != nil {
		return err
	}
	return telemetry.TracerProvider.Shutdown(ctx)
}

func RegisterLifecycle(lifecycle fx.Lifecycle, telemetry *Telemetry) {
	lifecycle.Append(fx.Hook{
		OnStop: telemetry.Shutdown,
	})
}
