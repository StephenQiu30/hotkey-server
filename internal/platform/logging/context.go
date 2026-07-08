package logging

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"go.uber.org/zap"
)

// FieldsFromContext extracts runtime metadata fields from context.
func FieldsFromContext(ctx context.Context) []zap.Field {
	var fields []zap.Field
	if reqID := runtime.RequestIDFromContext(ctx); reqID != "" {
		fields = append(fields, zap.String("request_id", reqID))
	}
	if traceID := runtime.TraceIDFromContext(ctx); traceID != "" {
		fields = append(fields, zap.String("trace_id", traceID))
	}
	if userID := runtime.UserIDFromContext(ctx); userID != 0 {
		fields = append(fields, zap.Int64("user_id", userID))
	}
	if mod := runtime.ModuleFromContext(ctx); mod != "" {
		fields = append(fields, zap.String("module", mod))
	}
	return fields
}

// Ctx returns a contextualized Logger with request metadata from ctx.
func Ctx(ctx context.Context) *zap.Logger {
	return L().With(FieldsFromContext(ctx)...)
}
