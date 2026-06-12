package monitor

import "context"

type contextKey string

const userIDKey contextKey = "user_id"

// ContextWithUserID returns a new context with the given user ID.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// userIDFromContext extracts the user ID from the context.
func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}
