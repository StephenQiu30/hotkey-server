package runtime

import "context"

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
