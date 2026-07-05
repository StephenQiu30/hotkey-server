package http

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	platformruntime "github.com/StephenQiu30/hotkey-server/internal/platform/runtime"
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
				log.Printf("panic recovered: %v", r)
				body, err := json.Marshal(newInternalErrorBody(requestIDFromContext(c)))
				if err != nil {
					body = []byte(`{"error":"internal server error","code":"internal_error"}`)
				}
				c.Data(http.StatusInternalServerError, "application/json", body)
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
			respondError(c, http.StatusUnauthorized, err.Error())
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
		return []byte(jwtSecret), nil
	})
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

func requestIDFromContext(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return c.GetHeader("X-Request-Id")
}

func isPublicPath(path string) bool {
	switch path {
	case "/healthz", "/api/v1/auth/register", "/api/v1/auth/login":
		return true
	default:
		return strings.HasPrefix(path, "/schemas/")
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
