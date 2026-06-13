package monitor

import (
	"context"

	"github.com/StephenQiu30/hotkey-server/internal/server"
)

// ContextWithUserID returns a new context with the given user ID.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, server.UserIDKey, userID)
}

// userIDFromContext extracts the user ID from the context.
func userIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(server.UserIDKey).(int64)
	return id, ok
}
