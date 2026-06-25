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
)

type contextKey string

// UserIDKey is the context key for the authenticated user ID.
const UserIDKey contextKey = "userID"

// RequestIDMiddleware adds a request ID to the response header.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-Id")
		if reqID == "" {
			reqID = generateRequestID()
		}
		c.Header("X-Request-Id", reqID)
		c.Next()
	}
}

// RecoverMiddleware recovers from panics and returns a 500 error.
func RecoverMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic recovered: %v", r)
				body, err := json.Marshal(newInternalErrorBody())
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
		if smokeTest || isPublicPath(c.Request.URL.Path) {
			ctx := context.WithValue(c.Request.Context(), UserIDKey, int64(1))
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

	return context.WithValue(c.Request.Context(), UserIDKey, int64(sub)), nil
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
	case "/healthz", "/openapi.json":
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
