package http

import (
	"context"
)

// userIDFromCtx extracts the user ID from the context set by auth middleware.
func userIDFromCtx(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
	return id, ok
}
