package controller

import (
	"context"

	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

// userIDFromCtx extracts the user ID from the context set by auth middleware.
func userIDFromCtx(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(platformhttp.UserIDKey).(int64)
	return id, ok
}
