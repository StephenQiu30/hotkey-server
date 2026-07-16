// Package requestcontext carries safe HTTP correlation identifiers through
// standard context.Context values without coupling application code to Gin.
package requestcontext

import "context"

type key uint8

const (
	requestIDKey key = iota
	traceIDKey
)

// WithRequestID adds the platform request identifier to a standard context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithTraceID adds the active trace identifier to a standard context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// RequestID returns the request identifier, or an empty string when absent.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// TraceID returns the trace identifier, or an empty string when absent.
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceIDKey).(string)
	return value
}
