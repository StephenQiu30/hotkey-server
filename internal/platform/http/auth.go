package http

import (
	"context"
	"errors"
	stdhttp "net/http"
	"strings"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// Role is the small, stable authorization fact exposed by platform HTTP. It
// deliberately is not an identity-domain type, so the platform depends only
// on the narrow Authenticator port below.
type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleAdmin  Role = "admin"
)

func (role Role) valid() bool {
	return role == RoleViewer || role == RoleEditor || role == RoleAdmin
}

// Subject is the database-backed identity fact made available only after a
// successful authentication middleware invocation.
type Subject struct {
	UserID    int64
	SessionID int64
	Role      Role
}

func (subject Subject) valid() bool {
	return subject.UserID > 0 && subject.SessionID > 0 && subject.Role.valid()
}

// Authenticator validates one bearer token and returns current identity facts.
// Its implementation is intentionally owned by bootstrap, not platform HTTP.
type Authenticator interface {
	Authenticate(context.Context, string) (Subject, error)
}

type subjectContextKey struct{}

// RequireAuthentication validates a strict Bearer header through the injected
// application port. It never trusts token claims or client-supplied roles.
func RequireAuthentication(authenticator Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok || authenticator == nil {
			WriteError(c, unauthenticated())
			return
		}
		subject, err := authenticator.Authenticate(c.Request.Context(), token)
		if err != nil {
			WriteError(c, authenticationError(err))
			return
		}
		if !subject.valid() {
			WriteError(c, unauthenticated())
			return
		}
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), subjectContextKey{}, subject))
		c.Next()
	}
}

// RequireRoles rejects a request unless a preceding RequireAuthentication has
// placed a valid Subject into the unforgeable request context.
func RequireRoles(roles ...Role) gin.HandlerFunc {
	allowed := make(map[Role]struct{}, len(roles))
	for _, role := range roles {
		if role.valid() {
			allowed[role] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		subject, ok := SubjectFromContext(c)
		if !ok {
			WriteError(c, unauthenticated())
			return
		}
		if _, ok := allowed[subject.Role]; !ok {
			WriteError(c, forbidden())
			return
		}
		c.Next()
	}
}

// SubjectFromContext returns only a Subject written by RequireAuthentication.
// A Gin string key cannot forge it because the source of truth is an unexported
// typed net/http context key.
func SubjectFromContext(c *gin.Context) (Subject, bool) {
	if c == nil || c.Request == nil {
		return Subject{}, false
	}
	subject, ok := c.Request.Context().Value(subjectContextKey{}).(Subject)
	if !ok || !subject.valid() {
		return Subject{}, false
	}
	return subject, true
}

// PublicAuthGroup creates the anonymous /auth route group. It intentionally
// does not install RequireAuthentication, so malformed Bearer headers on
// registration/login endpoints are ignored rather than parsed.
func PublicAuthGroup(api *gin.RouterGroup) *gin.RouterGroup {
	return api.Group("/auth")
}

// RequireCookieOrigin protects cookie-bearing auth endpoints from requests
// whose Origin is absent or not present in the explicit configured allowlist.
func RequireCookieOrigin(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !originAllowed(c.GetHeader("Origin"), allowedOrigins) {
			WriteError(c, forbidden())
			return
		}
		c.Next()
	}
}

// NewUnavailableAuthenticator is used only by lightweight API constructor
// tests with no database runtime. Real running API processes require a
// database and receive the identity application adapter from bootstrap.
func NewUnavailableAuthenticator() Authenticator {
	return unavailableAuthenticator{}
}

type unavailableAuthenticator struct{}

func (unavailableAuthenticator) Authenticate(context.Context, string) (Subject, error) {
	return Subject{}, unauthenticated()
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}

func originAllowed(origin string, allowedOrigins []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	for _, allowed := range allowedOrigins {
		if allowed = strings.TrimSpace(allowed); allowed != "" && allowed != "*" && origin == allowed {
			return true
		}
	}
	return false
}

func authenticationError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return unauthenticated()
}

func unauthenticated() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
}

func forbidden() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "")
}
