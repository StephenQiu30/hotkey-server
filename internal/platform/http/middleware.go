package http

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
)

type contextKey string

const UserIDKey contextKey = "userID"

// RequestIDMiddleware injects a request ID into the response header and context.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-Id")
		if reqID == "" {
			reqID = generateRequestID()
		}
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = reqID
		}
		c.Header("X-Request-Id", reqID)
		c.Header("X-Trace-Id", traceID)
		c.Set("request_id", reqID)
		c.Set("trace_id", traceID)
		ctx := platformruntime.WithRequestID(c.Request.Context(), reqID)
		ctx = platformruntime.WithTraceID(ctx, traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ContextMetadataMiddleware injects module and operator into request context.
func ContextMetadataMiddleware(module string) gin.HandlerFunc {
	return func(c *gin.Context) {
		operator := c.GetHeader("X-Operator")
		ctx := platformruntime.WithModule(c.Request.Context(), module)
		ctx = platformruntime.WithOperator(ctx, operator)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RecoverMiddleware recovers from panics and returns a 500 with unified envelope.
func RecoverMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logging.Ctx(c.Request.Context()).Error("panic recovered",
					zap.Any("panic", r),
				)
				RespondInternalError(c)
				c.Abort()
			}
		}()
		c.Next()
	}
}

// AuthMiddleware validates JWT tokens using typed AccessClaims and injects the
// user ID into context. It expects tokens signed and parsed by the security package,
// which enforces HS256, the configured issuer, audience, exp, and nbf.
func AuthMiddleware(jwtSecret string, isSmokeTest bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSmokeTest {
			ctx := context.WithValue(c.Request.Context(), UserIDKey, int64(1))
			ctx = platformruntime.WithUserID(ctx, int64(1))
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}

		// Public path bypass: these paths are registered unprotected at the
		// routing level, but an extra check here keeps the middleware safe
		// when used standalone (e.g. in unit tests).
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			RespondError(c, enum.ErrorCodeUnauthorized, "缺少认证令牌")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			RespondError(c, enum.ErrorCodeUnauthorized, "认证令牌格式错误")
			c.Abort()
			return
		}

		claims, err := security.ParseAccessToken(parts[1], jwtSecret)
		if err != nil {
			// Map specific JWT error types to appropriate error codes.
			if isTokenExpiredError(err) {
				RespondError(c, enum.ErrorCodeTokenExpired, "")
			} else {
				RespondError(c, enum.ErrorCodeUnauthorized, "认证令牌无效")
			}
			c.Abort()
			return
		}

		// Extract user ID from Subject claim.
		userID := parseSubjectAsInt64(claims.Subject)
		ctx := context.WithValue(c.Request.Context(), UserIDKey, userID)
		ctx = platformruntime.WithUserID(ctx, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// isTokenExpiredError checks if the error is specifically a token expiry error.
func isTokenExpiredError(err error) bool {
	if err == nil {
		return false
	}
	// jwt.ErrTokenExpired is a sentinel from golang-jwt.
	return strings.Contains(err.Error(), jwt.ErrTokenExpired.Error())
}

// parseSubjectAsInt64 converts a JWT subject string to an int64.
// Defaults to 0 if parsing fails.
func parseSubjectAsInt64(subject string) int64 {
	var id int64
	fmt.Sscanf(subject, "%d", &id)
	return id
}

// CORSMiddleware returns a Gin middleware that applies origin-allowlist checks.
// It replaces the permissive wildcard approach with exact origin matching.
// When the allowlist contains "*", all origins are allowed but the specific
// origin is echoed (required for credentialed requests).
// Credentialed requests are handled properly: Access-Control-Allow-Origin
// echoes the request Origin when matched, and the Vary header is set.
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	// Build a set for O(1) lookup and check for wildcard.
	hasWildcard := false
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			hasWildcard = true
		}
		originSet[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// Always set Vary: Origin so caches know the response varies on this header.
		c.Header("Vary", "Origin")

		if origin == "" {
			// Non-browser request (no Origin header) — skip CORS processing.
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
			c.Next()
			return
		}

		// Determine if the origin is allowed.
		originAllowed := hasWildcard
		if !originAllowed {
			_, originAllowed = originSet[origin]
		}

		if !originAllowed {
			// Origin is not allowed. For preflight, return forbidden.
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			// For actual requests, proceed without CORS headers (browser will reject).
			c.Next()
			return
		}

		// Echo the specific origin (never "*" with credentials).
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// NoRouteHandler returns a 404 response using the unified envelope.
func NoRouteHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondError(c, enum.ErrorCodeNotFound, "")
	}
}

// NoMethodHandler returns a 405 response using the unified envelope.
func NoMethodHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		RespondError(c, enum.ErrorCodeMethodNotAllowed, "")
	}
}

// SecurityHeadersMiddleware adds recommended security response headers.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Next()
	}
}

type authError struct {
	msg string
}

func (e *authError) Error() string { return e.msg }

func generateRequestID() string {
	return "req-" + randomHex(8)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

// isPublicPath returns true if the request path does not require authentication.
// The authoritative public-path list is at the routing level (route_controller.go),
// but this helper provides defense-in-depth for standalone middleware usage.
func isPublicPath(path string) bool {
	switch path {
	case "/healthz", "/api/v1/auth/register", "/api/v1/auth/login":
		return true
	default:
		return strings.HasPrefix(path, "/schemas/") || strings.HasPrefix(path, "/swagger/")
	}
}
