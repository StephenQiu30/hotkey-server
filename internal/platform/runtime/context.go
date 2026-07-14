package runtime

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	traceIDKey   contextKey = "trace_id"
	userIDKey    contextKey = "user_id"
	moduleKey    contextKey = "module"
	operatorKey  contextKey = "operator"
)

// WithRequestID stores the request ID in context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext returns the request ID stored in context.
func RequestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

// WithTraceID stores the trace ID in context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext returns the trace ID stored in context.
func TraceIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(traceIDKey).(string); ok {
		return value
	}
	return ""
}

// WithUserID stores the authenticated user ID in context.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext returns the authenticated user ID stored in context.
func UserIDFromContext(ctx context.Context) int64 {
	if value, ok := ctx.Value(userIDKey).(int64); ok {
		return value
	}
	return 0
}

const userContextKey contextKey = "user_context"

// WithUserContext stores the UserContext in context.
func WithUserContext(ctx context.Context, uc *dto.UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, uc)
}

// UserFromContext returns the UserContext stored in context.
// Returns nil when no authenticated user context exists.
func UserFromContext(ctx context.Context) *dto.UserContext {
	if uc, ok := ctx.Value(userContextKey).(*dto.UserContext); ok {
		return uc
	}
	return nil
}

// WithModule stores the current application module in context.
func WithModule(ctx context.Context, module string) context.Context {
	return context.WithValue(ctx, moduleKey, module)
}

// ModuleFromContext returns the current application module stored in context.
func ModuleFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(moduleKey).(string); ok {
		return value
	}
	return ""
}

// WithOperator stores the operator identifier in context.
func WithOperator(ctx context.Context, operator string) context.Context {
	return context.WithValue(ctx, operatorKey, operator)
}

// OperatorFromContext returns the operator identifier stored in context.
func OperatorFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(operatorKey).(string); ok {
		return value
	}
	return ""
}
