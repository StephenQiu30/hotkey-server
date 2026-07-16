package http

import (
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

const optionalSubjectContextKey = "hotkey.identity.optional_subject"

// optionalAuthentication is deliberately restricted to logout. A valid bearer
// token takes precedence there, while missing or stale bearer credentials can
// still be handled by the Refresh cookie in the application workflow.
func optionalAuthentication(authenticator httptransport.Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if authenticator != nil {
			if token, ok := bearerToken(c.GetHeader("Authorization")); ok {
				if subject, err := authenticator.Authenticate(c.Request.Context(), token); err == nil && subject.UserID > 0 && subject.SessionID > 0 {
					c.Set(optionalSubjectContextKey, domain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: domain.Role(subject.Role)})
				}
			}
		}
		c.Next()
	}
}

func optionalSubject(c *gin.Context) *domain.Subject {
	if c == nil {
		return nil
	}
	subject, ok := c.Get(optionalSubjectContextKey)
	if !ok {
		return nil
	}
	value, ok := subject.(domain.Subject)
	if !ok || value.UserID <= 0 || value.SessionID <= 0 || !value.Role.Valid() {
		return nil
	}
	return &value
}

func protectedSubject(c *gin.Context) (domain.Subject, bool) {
	subject, ok := httptransport.SubjectFromContext(c)
	if !ok {
		return domain.Subject{}, false
	}
	value := domain.Subject{UserID: subject.UserID, SessionID: subject.SessionID, Role: domain.Role(subject.Role)}
	if value.UserID <= 0 || value.SessionID <= 0 || !value.Role.Valid() {
		return domain.Subject{}, false
	}
	return value, true
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}
