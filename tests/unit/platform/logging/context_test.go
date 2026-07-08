package logging_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
)

func TestFieldsFromContextEmpty(t *testing.T) {
	fields := logging.FieldsFromContext(context.Background())
	if len(fields) != 0 {
		t.Fatalf("expected 0 fields from empty context, got %d", len(fields))
	}
}

func TestFieldsFromContextAllFields(t *testing.T) {
	ctx := context.Background()
	ctx = runtime.WithRequestID(ctx, "req-123")
	ctx = runtime.WithTraceID(ctx, "trace-abc")
	ctx = runtime.WithUserID(ctx, int64(42))
	ctx = runtime.WithModule(ctx, "http")

	fields := logging.FieldsFromContext(ctx)
	if len(fields) != 4 {
		t.Fatalf("expected 4 fields, got %d: %v", len(fields), fields)
	}
}

func TestCtxReturnsNonNil(t *testing.T) {
	logging.Init("info", "json", "stdout")
	logger := logging.Ctx(context.Background())
	if logger == nil {
		t.Fatal("Ctx() returned nil")
	}
}
