package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type authenticatorStub struct {
	called atomic.Int64
	auth   func(context.Context, string) (Subject, error)
}

func (stub *authenticatorStub) Authenticate(ctx context.Context, token string) (Subject, error) {
	stub.called.Add(1)
	return stub.auth(ctx, token)
}

func TestAuthenticationRejectsMissingMalformedAndInvalidBearer(t *testing.T) {
	t.Parallel()

	invalid := sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "")
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing bearer"},
		{name: "malformed scheme", header: "Token access"},
		{name: "empty bearer", header: "Bearer   "},
		{name: "non-HS256 token", header: "Bearer non-hs256"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := &authenticatorStub{auth: func(_ context.Context, _ string) (Subject, error) {
				return Subject{}, invalid
			}}
			router := gin.New()
			called := false
			router.GET("/protected", RequireAuthentication(authenticator), func(c *gin.Context) {
				called = true
				OK[any](c, nil)
			})

			request := httptest.NewRequest(stdhttp.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", tt.header)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != stdhttp.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusUnauthorized)
			}
			assertResult(t, response, sharederrors.CodeUnauthenticated, "unauthenticated", nil)
			assertThreeFieldResult(t, response)
			if called {
				t.Fatal("protected handler was called")
			}
			if tt.header == "Bearer non-hs256" && authenticator.called.Load() != 1 {
				t.Fatalf("authenticator calls = %d, want 1", authenticator.called.Load())
			}
			if tt.header != "Bearer non-hs256" && authenticator.called.Load() != 0 {
				t.Fatalf("authenticator calls = %d, want 0", authenticator.called.Load())
			}
		})
	}
}

func TestAuthenticationRejectsInvalidatedSessions(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"revoked", "disabled", "deleted"} {
		state := state
		t.Run(state, func(t *testing.T) {
			authenticator := &authenticatorStub{auth: func(_ context.Context, token string) (Subject, error) {
				if token != state {
					t.Fatalf("token = %q, want %q", token, state)
				}
				return Subject{}, sharederrors.New(sharederrors.CodeSessionInvalid, stdhttp.StatusUnauthorized, "")
			}}
			router := gin.New()
			router.GET("/protected", RequireAuthentication(authenticator), func(c *gin.Context) { OK[any](c, nil) })

			request := httptest.NewRequest(stdhttp.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", "Bearer "+state)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != stdhttp.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusUnauthorized)
			}
			assertResult(t, response, sharederrors.CodeSessionInvalid, "session invalid", nil)
			assertThreeFieldResult(t, response)
		})
	}
}

func assertThreeFieldResult(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode Result fields: %v", err)
	}
	if len(result) != 3 || result["code"] == nil || result["message"] == nil || result["data"] == nil {
		t.Fatalf("Result fields = %#v, want only code, message, data", result)
	}
}

func TestAuthenticationStoresUnforgeableSubject(t *testing.T) {
	t.Parallel()

	expected := Subject{UserID: 11, SessionID: 22, Role: RoleEditor}
	authenticator := &authenticatorStub{auth: func(_ context.Context, _ string) (Subject, error) {
		return expected, nil
	}}
	router := gin.New()
	router.GET("/protected", RequireAuthentication(authenticator), func(c *gin.Context) {
		subject, ok := SubjectFromContext(c)
		if !ok || subject != expected {
			t.Fatalf("SubjectFromContext() = %#v, %t; want %#v, true", subject, ok, expected)
		}
		OK[any](c, nil)
	})
	router.GET("/forged", func(c *gin.Context) {
		c.Set("hotkey.subject", expected)
		if _, ok := SubjectFromContext(c); ok {
			t.Fatal("string context key forged authentication subject")
		}
		OK[any](c, nil)
	})

	for _, path := range []string{"/protected", "/forged"} {
		request := httptest.NewRequest(stdhttp.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer valid")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != stdhttp.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, response.Code, stdhttp.StatusOK)
		}
	}
}

func TestAuthorizationRejectsViewerForAdminRoute(t *testing.T) {
	t.Parallel()

	authenticator := &authenticatorStub{auth: func(_ context.Context, _ string) (Subject, error) {
		return Subject{UserID: 11, SessionID: 22, Role: RoleViewer}, nil
	}}
	router := gin.New()
	admin := router.Group("/admin", RequireAuthentication(authenticator), RequireRoles(RoleAdmin))
	admin.GET("/users", func(c *gin.Context) { OK[any](c, nil) })

	request := httptest.NewRequest(stdhttp.MethodGet, "/admin/users", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusForbidden)
	}
	assertResult(t, response, sharederrors.CodeForbidden, "forbidden", nil)
}

func TestPublicAuthGroupNeverParsesBearer(t *testing.T) {
	t.Parallel()

	authenticator := &authenticatorStub{auth: func(_ context.Context, _ string) (Subject, error) {
		return Subject{}, errors.New("public route must not authenticate")
	}}
	router := gin.New()
	protected := router.Group("/api/v1", RequireAuthentication(authenticator))
	protected.GET("/protected", func(c *gin.Context) { OK[any](c, nil) })
	public := PublicAuthGroup(router.Group("/api/v1"))
	public.GET("/login", func(c *gin.Context) { OK[any](c, nil) })

	request := httptest.NewRequest(stdhttp.MethodGet, "/api/v1/auth/login", nil)
	request.Header.Set("Authorization", "Bearer ignored")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusOK)
	}
	if authenticator.called.Load() != 0 {
		t.Fatalf("authenticator calls = %d, want 0", authenticator.called.Load())
	}
}

func TestCORSAndCookieOriginUseExplicitAllowlist(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Authentication.AllowedOrigins = []string{"https://app.example.test"}
	router := gin.New()
	router.Use(cors(cfg.Authentication.AllowedOrigins))
	router.GET("/public", func(c *gin.Context) { OK[any](c, nil) })
	cookies := router.Group("/api/v1/auth", RequireCookieOrigin(cfg.Authentication.AllowedOrigins))
	cookies.POST("/refresh", func(c *gin.Context) { OK[any](c, nil) })

	allowed := httptest.NewRequest(stdhttp.MethodGet, "/public", nil)
	allowed.Header.Set("Origin", "https://app.example.test")
	allowedResponse := httptest.NewRecorder()
	router.ServeHTTP(allowedResponse, allowed)
	if got := allowedResponse.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.test" {
		t.Errorf("allowed origin = %q, want explicit origin", got)
	}
	if got := allowedResponse.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("credentials = %q, want true", got)
	}
	if got := allowedResponse.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("credentialed CORS used wildcard origin")
	}

	disallowed := httptest.NewRequest(stdhttp.MethodGet, "/public", nil)
	disallowed.Header.Set("Origin", "https://evil.example.test")
	disallowedResponse := httptest.NewRecorder()
	router.ServeHTTP(disallowedResponse, disallowed)
	if got := disallowedResponse.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin = %q, want no CORS grant", got)
	}

	for _, origin := range []string{"https://app.example.test", "https://evil.example.test", ""} {
		request := httptest.NewRequest(stdhttp.MethodPost, "/api/v1/auth/refresh", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if origin == "https://app.example.test" {
			if response.Code != stdhttp.StatusOK {
				t.Errorf("allowed cookie origin status = %d, want %d", response.Code, stdhttp.StatusOK)
			}
			continue
		}
		if response.Code != stdhttp.StatusForbidden {
			t.Errorf("cookie origin %q status = %d, want %d", origin, response.Code, stdhttp.StatusForbidden)
		}
		assertResult(t, response, sharederrors.CodeForbidden, "forbidden", nil)
	}
}
