// Package requestcontext carries safe HTTP correlation identifiers through
// standard context.Context values without coupling application code to Gin.
package requestcontext

import "context"

type key uint8

const (
	requestIDKey key = iota
	traceIDKey
	jobIDKey
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

// WithJobID records the durable queue job that owns work started from ctx.
// Non-positive identifiers are ignored so callers cannot persist an invalid
// foreign-key identity by accident.
func WithJobID(ctx context.Context, jobID int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if jobID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, jobIDKey, jobID)
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

// JobID returns the owning durable job identifier, or zero when the current
// operation was not started by the queue worker.
func JobID(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	value, _ := ctx.Value(jobIDKey).(int64)
	if value <= 0 {
		return 0
	}
	return value
}
