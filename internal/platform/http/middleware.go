package http

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
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

// RecoverMiddleware recovers from panics and returns a 500.
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

// AuthMiddleware validates JWT tokens and injects the user ID into context.
func AuthMiddleware(jwtSecret string, smokeTest bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		if smokeTest {
			ctx := context.WithValue(c.Request.Context(), UserIDKey, int64(1))
			ctx = platformruntime.WithUserID(ctx, int64(1))
			c.Request = c.Request.WithContext(ctx)
			c.Next()
			return
		}

		newCtx, err := validateJWT(c, jwtSecret)
		if err != nil {
			RespondError(c, enum.ErrorCodeUnauthorized, "")
			c.Abort()
			return
		}

		c.Request = c.Request.WithContext(newCtx)
		c.Next()
	}
}

func validateJWT(c *gin.Context, jwtSecret string) (context.Context, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return nil, &authError{"missing authorization header"}
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, &authError{"invalid authorization format"}
	}

	token, err := jwt.Parse(parts[1], func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, &authError{"invalid token"}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, &authError{"invalid token claims"}
	}

	sub, ok := claims["sub"].(float64)
	if !ok {
		return nil, &authError{"invalid user id in token"}
	}

	ctx := context.WithValue(c.Request.Context(), UserIDKey, int64(sub))
	ctx = platformruntime.WithUserID(ctx, int64(sub))
	return ctx, nil
}

type authError struct {
	msg string
}

func (e *authError) Error() string { return e.msg }

func generateRequestID() string {
	return "req-" + randomHex(8)
}

func isPublicPath(path string) bool {
	switch path {
	case "/healthz", "/api/v1/auth/register", "/api/v1/auth/login":
		return true
	default:
		return strings.HasPrefix(path, "/schemas/") || strings.HasPrefix(path, "/swagger/")
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

// CORSMiddleware adds permissive CORS headers for web frontend access.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
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
