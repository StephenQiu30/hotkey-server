package http

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"github.com/StephenQiu30/hotkey-server/internal/service"
)

// UserContextMiddleware injects full UserContext into the request context.
//
// It tries Redis cache first, then falls back to DB. On success, sets
// the UserContext on the context so handlers can use runtime.UserFromContext().
//
// The middleware MUST be placed after AuthMiddleware (which injects userID).
func UserContextMiddleware(userRepo service.UserRepository, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := platformruntime.UserIDFromContext(c.Request.Context())
		if userID == 0 {
			c.Next()
			return
		}

		uc := &dto.UserContext{UserID: userID}

		// Try Redis cache first
		var fromCache bool
		if rdb != nil {
			cached, err := rdb.Get(c.Request.Context(), userInfoKey(userID)).Bytes()
			if err == nil {
				if err := json.Unmarshal(cached, uc); err == nil && uc.Email != "" {
					fromCache = true
				}
			}
		}

		if !fromCache {
			// Fallback to DB
			user, err := userRepo.GetByID(c.Request.Context(), userID)
			if err != nil || user == nil {
				c.Next()
				return
			}
			uc.Email = user.Email
			uc.DisplayName = user.DisplayName
			uc.Status = user.Status
			uc.PlanType = user.PlanType
			uc.CreatedAt = user.CreatedAt

			// Cache in Redis (best-effort, TTL 10 min)
			if rdb != nil {
				encoded, err := json.Marshal(uc)
				if err == nil {
					rdb.SetEx(c.Request.Context(), userInfoKey(userID), encoded, 10*time.Minute)
				}
			}
		}

		ctx := platformruntime.WithUserContext(c.Request.Context(), uc)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// userInfoKey returns the Redis key for user info cache.
func userInfoKey(userID int64) string {
	return fmt.Sprintf("user_info:%d", userID)
}
