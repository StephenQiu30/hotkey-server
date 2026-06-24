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

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/golang-jwt/jwt/v5"
)

// contextKey is a private type for context keys in this package.
type contextKey string

// UserIDKey is the context key for the authenticated user ID.
// Value matches internal/server.UserIDKey for backward compatibility.
const UserIDKey contextKey = "userID"

// globalAPI holds the Huma API instance for use in middleware error responses.
// Set once during NewAPI construction.
var globalAPI huma.API

// RequestIDMiddleware returns a Huma middleware that adds a request ID to the
// response header. If the request already has an X-Request-Id header, it is
// preserved; otherwise a new one is generated.
func RequestIDMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		reqID := ctx.Header("X-Request-Id")
		if reqID == "" {
			reqID = generateRequestID()
		}
		ctx.SetHeader("X-Request-Id", reqID)
		next(ctx)
	}
}

// RecoverMiddleware returns a Huma middleware that recovers from panics and
// returns a 500 error instead of crashing the server.
func RecoverMiddleware() func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic recovered: %v", r)
				_, w := humago.Unwrap(ctx)
				body, err := json.Marshal(newInternalErrorBody())
				if err != nil {
					body = []byte(`{"error":"internal server error","code":"internal_error"}`)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(body)
			}
		}()
		next(ctx)
	}
}

// AuthMiddleware returns a Huma middleware that validates JWT tokens and injects
// the user ID into the request context. For smoke test mode, bypasses JWT
// validation and injects a default user ID.
func AuthMiddleware(jwtSecret string, smokeTest bool) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if smokeTest {
			newCtx := context.WithValue(ctx.Context(), UserIDKey, int64(1))
			next(huma.WithContext(ctx, newCtx))
			return
		}

		newCtx, err := validateJWT(ctx, jwtSecret)
		if err != nil {
			huma.WriteErr(globalAPI, ctx, http.StatusUnauthorized, err.Error())
			return
		}

		next(huma.WithContext(ctx, newCtx))
	}
}

// validateJWT extracts and validates the JWT from the Authorization header.
// On success, returns a new context with the user ID injected. Returns an error
// if validation fails.
func validateJWT(ctx huma.Context, jwtSecret string) (context.Context, error) {
	authHeader := ctx.Header("Authorization")
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

	// Inject user ID into context (same key as internal/server.UserIDKey).
	newCtx := context.WithValue(ctx.Context(), UserIDKey, int64(sub))
	return newCtx, nil
}

// authError is a simple error type for authentication failures.
type authError struct {
	msg string
}

func (e *authError) Error() string { return e.msg }

// generateRequestID creates a simple request ID.
func generateRequestID() string {
	return "req-" + randomHex(8)
}

// randomHex generates a random hex string of the given byte length using
// crypto/rand. Returns a string of length 2*n.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if rand fails (should never happen).
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
