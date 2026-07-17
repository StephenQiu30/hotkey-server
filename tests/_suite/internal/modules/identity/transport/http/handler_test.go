package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	identityapplication "github.com/StephenQiu30/hotkey-server/internal/modules/identity/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestAuthRoutesExposeDesignContractAndSafeCookies(t *testing.T) {
	t.Parallel()

	service := successfulService()
	router := newIdentityRouter(service, httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}, false)

	requests := []struct {
		name       string
		method     string
		path       string
		body       string
		auth       bool
		cookie     bool
		origin     bool
		wantStatus int
	}{
		{"request verification", http.MethodPost, "/api/v1/auth/email-verifications", `{"email":"reader@example.test","purpose":"registration"}`, false, false, false, http.StatusOK},
		{"confirm verification", http.MethodPost, "/api/v1/auth/email-verifications/confirm", `{"email":"reader@example.test","purpose":"registration","code":"123456"}`, false, false, false, http.StatusOK},
		{"registration", http.MethodPost, "/api/v1/auth/registrations", `{"verification_ticket":"ticket","password":"correct horse battery staple","display_name":"Reader"}`, false, false, false, http.StatusCreated},
		{"login", http.MethodPost, "/api/v1/auth/login", `{"email":"reader@example.test","password":"correct horse battery staple"}`, false, false, false, http.StatusOK},
		{"refresh", http.MethodPost, "/api/v1/auth/refresh", `{}`, false, true, true, http.StatusOK},
		{"logout", http.MethodPost, "/api/v1/auth/logout", `{}`, true, true, true, http.StatusOK},
		{"me", http.MethodGet, "/api/v1/auth/me", ``, true, false, false, http.StatusOK},
		{"change password", http.MethodPost, "/api/v1/auth/password", `{"current_password":"old password","new_password":"new password"}`, true, false, false, http.StatusOK},
		{"password reset", http.MethodPost, "/api/v1/auth/password-resets/confirm", `{"verification_ticket":"ticket","password":"new password"}`, false, false, false, http.StatusOK},
		{"list users", http.MethodGet, "/api/v1/users", ``, true, false, false, http.StatusOK},
		{"patch user", http.MethodPatch, "/api/v1/users/3", `{"role":"editor"}`, true, false, false, http.StatusOK},
		{"delete user", http.MethodDelete, "/api/v1/users/3", ``, true, false, false, http.StatusOK},
		{"restore user", http.MethodPost, "/api/v1/users/3/restore", `{}`, true, false, false, http.StatusOK},
	}

	for _, tt := range requests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if tt.auth {
				request.Header.Set("Authorization", "Bearer access-token")
			}
			if tt.cookie {
				request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token", Path: refreshCookiePath})
			}
			if tt.origin {
				request.Header.Set("Origin", "https://app.example.test")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			assertResultEnvelope(t, response, 0)
			assertNoSensitiveResponseFields(t, response.Body.String())
		})
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"reader@example.test","password":"correct horse battery staple"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, login)
	assertRefreshCookie(t, loginResponse, false, false)

	refresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refresh.Header.Set("Origin", "https://app.example.test")
	refresh.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token", Path: refreshCookiePath})
	refreshResponse := httptest.NewRecorder()
	router.ServeHTTP(refreshResponse, refresh)
	assertRefreshCookie(t, refreshResponse, false, false)

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.Header.Set("Origin", "https://app.example.test")
	logout.Header.Set("Authorization", "Bearer access-token")
	logout.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token", Path: refreshCookiePath})
	logoutResponse := httptest.NewRecorder()
	router.ServeHTTP(logoutResponse, logout)
	assertRefreshCookie(t, logoutResponse, false, true)
}

func TestAuthenticationHTTPFailuresAreSafeAndStable(t *testing.T) {
	t.Parallel()

	t.Run("verification is nonenumerating", func(t *testing.T) {
		service := successfulService()
		service.requestVerification = func(_ context.Context, _ domain.VerificationPurpose, email string) error {
			if email != "known@example.test" && email != "new@example.test" {
				t.Fatalf("email = %q", email)
			}
			return nil
		}
		router := newIdentityRouter(service, httptransport.Subject{}, false)
		var responses []string
		for _, email := range []string{"known@example.test", "new@example.test"} {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/email-verifications", strings.NewReader(`{"email":"`+email+`","purpose":"registration"}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("%s status = %d, want 200", email, response.Code)
			}
			responses = append(responses, response.Body.String())
		}
		if responses[0] != responses[1] {
			t.Fatalf("verification results differ: %q vs %q", responses[0], responses[1])
		}
	})

	t.Run("SMTP unavailable does not leak cause", func(t *testing.T) {
		service := successfulService()
		service.requestVerification = func(context.Context, domain.VerificationPurpose, string) error {
			return sharederrors.Wrap(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "", errors.New("smtp password=secret"))
		}
		response := serveJSON(t, newIdentityRouter(service, httptransport.Subject{}, false), http.MethodPost, "/api/v1/auth/email-verifications", `{"email":"reader@example.test","purpose":"registration"}`)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeUnavailable)
		if strings.Contains(response.Body.String(), "smtp") || strings.Contains(response.Body.String(), "secret") {
			t.Fatalf("response leaks cause: %s", response.Body.String())
		}
	})

	t.Run("rate limit preserves status and envelope", func(t *testing.T) {
		service := successfulService()
		service.requestVerification = func(context.Context, domain.VerificationPurpose, string) error {
			return sharederrors.New(sharederrors.CodeRateLimited, http.StatusTooManyRequests, "")
		}
		response := serveJSON(t, newIdentityRouter(service, httptransport.Subject{}, false), http.MethodPost, "/api/v1/auth/email-verifications", `{"email":"reader@example.test","purpose":"registration"}`)
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeRateLimited)
	})

	t.Run("invalid credentials are indistinguishable", func(t *testing.T) {
		service := successfulService()
		service.login = func(context.Context, identityapplication.Credentials) (identityapplication.Authentication, error) {
			return identityapplication.Authentication{}, domain.InvalidCredentials()
		}
		response := serveJSON(t, newIdentityRouter(service, httptransport.Subject{}, false), http.MethodPost, "/api/v1/auth/login", `{"email":"reader@example.test","password":"wrong"}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeInvalidCredentials)
	})

	t.Run("protected routes reject missing bearer", func(t *testing.T) {
		response := serveJSON(t, newIdentityRouter(successfulService(), httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleViewer}, false), http.MethodGet, "/api/v1/auth/me", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeUnauthenticated)
	})

	t.Run("viewer cannot access admin users", func(t *testing.T) {
		response := serveAuthorized(t, newIdentityRouter(successfulService(), httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleViewer}, false), http.MethodGet, "/api/v1/users", "", "access-token")
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeForbidden)
	})

	t.Run("refresh and logout require an allowed Origin", func(t *testing.T) {
		router := newIdentityRouter(successfulService(), httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}, false)
		for _, path := range []string{"/api/v1/auth/refresh", "/api/v1/auth/logout"} {
			request := httptest.NewRequest(http.MethodPost, path, nil)
			request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "refresh-token"})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("%s status = %d, want 403", path, response.Code)
			}
			assertResultEnvelope(t, response, sharederrors.CodeForbidden)
		}
	})

	t.Run("restore conflict does not mutate response state", func(t *testing.T) {
		service := successfulService()
		service.restoreUser = func(context.Context, domain.Subject, int64) (*domain.User, error) {
			return nil, domain.LastActiveAdmin()
		}
		response := serveAuthorized(t, newIdentityRouter(service, httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}, false), http.MethodPost, "/api/v1/users/3/restore", `{}`, "access-token")
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409", response.Code)
		}
		assertResultEnvelope(t, response, sharederrors.CodeLastActiveAdmin)
		if strings.Contains(response.Body.String(), "deleted_at") || strings.Contains(response.Body.String(), "user_id") {
			t.Fatalf("restore conflict leaks mutation state: %s", response.Body.String())
		}
	})
}

func TestAuthenticationCookiesAreSecureInProduction(t *testing.T) {
	t.Parallel()

	router := newIdentityRouter(successfulService(), httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleAdmin}, true)
	response := serveJSON(t, router, http.MethodPost, "/api/v1/auth/login", `{"email":"reader@example.test","password":"correct horse battery staple"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	assertRefreshCookie(t, response, true, false)
}

type identityServiceStub struct {
	requestVerification  func(context.Context, domain.VerificationPurpose, string) error
	confirmVerification  func(context.Context, domain.VerificationPurpose, string, string) (domain.VerificationTicket, error)
	register             func(context.Context, identityapplication.RegisterInput) (*domain.User, error)
	login                func(context.Context, identityapplication.Credentials) (identityapplication.Authentication, error)
	refresh              func(context.Context, string) (identityapplication.Authentication, error)
	logout               func(context.Context, *domain.Subject, string) error
	currentUser          func(context.Context, domain.Subject) (*domain.User, error)
	changePassword       func(context.Context, domain.Subject, string, string) error
	confirmPasswordReset func(context.Context, string, string) error
	listUsers            func(context.Context) ([]domain.User, error)
	updateUser           func(context.Context, domain.Subject, int64, identityapplication.UserUpdate) (*domain.User, error)
	deleteUser           func(context.Context, domain.Subject, int64) (*domain.User, error)
	restoreUser          func(context.Context, domain.Subject, int64) (*domain.User, error)
}

func (s *identityServiceStub) RequestVerification(ctx context.Context, purpose domain.VerificationPurpose, email string) error {
	return s.requestVerification(ctx, purpose, email)
}
func (s *identityServiceStub) ConfirmVerification(ctx context.Context, purpose domain.VerificationPurpose, email, code string) (domain.VerificationTicket, error) {
	return s.confirmVerification(ctx, purpose, email, code)
}
func (s *identityServiceStub) Register(ctx context.Context, input identityapplication.RegisterInput) (*domain.User, error) {
	return s.register(ctx, input)
}
func (s *identityServiceStub) Login(ctx context.Context, input identityapplication.Credentials) (identityapplication.Authentication, error) {
	return s.login(ctx, input)
}
func (s *identityServiceStub) Refresh(ctx context.Context, token string) (identityapplication.Authentication, error) {
	return s.refresh(ctx, token)
}
func (s *identityServiceStub) Logout(ctx context.Context, subject *domain.Subject, token string) error {
	return s.logout(ctx, subject, token)
}
func (s *identityServiceStub) CurrentUser(ctx context.Context, subject domain.Subject) (*domain.User, error) {
	return s.currentUser(ctx, subject)
}
func (s *identityServiceStub) ChangePassword(ctx context.Context, subject domain.Subject, currentPassword, nextPassword string) error {
	return s.changePassword(ctx, subject, currentPassword, nextPassword)
}
func (s *identityServiceStub) ConfirmPasswordReset(ctx context.Context, ticket, password string) error {
	return s.confirmPasswordReset(ctx, ticket, password)
}
func (s *identityServiceStub) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.listUsers(ctx)
}
func (s *identityServiceStub) UpdateUser(ctx context.Context, actor domain.Subject, id int64, input identityapplication.UserUpdate) (*domain.User, error) {
	return s.updateUser(ctx, actor, id, input)
}
func (s *identityServiceStub) DeleteUser(ctx context.Context, actor domain.Subject, id int64) (*domain.User, error) {
	return s.deleteUser(ctx, actor, id)
}
func (s *identityServiceStub) RestoreUser(ctx context.Context, actor domain.Subject, id int64) (*domain.User, error) {
	return s.restoreUser(ctx, actor, id)
}

func successfulService() *identityServiceStub {
	now := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	user := domain.User{ID: 3, Email: "reader@example.test", PasswordHash: "must-never-leak", DisplayName: "Reader", Role: domain.RoleViewer, Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now}
	authentication := identityapplication.Authentication{AccessToken: "access-token", RefreshToken: "refresh-token", User: user}
	return &identityServiceStub{
		requestVerification: func(context.Context, domain.VerificationPurpose, string) error { return nil },
		confirmVerification: func(context.Context, domain.VerificationPurpose, string, string) (domain.VerificationTicket, error) {
			return domain.VerificationTicket{Token: "ticket", Purpose: domain.VerificationPurposeRegistration}, nil
		},
		register: func(context.Context, identityapplication.RegisterInput) (*domain.User, error) { return &user, nil },
		login: func(context.Context, identityapplication.Credentials) (identityapplication.Authentication, error) {
			return authentication, nil
		},
		refresh:              func(context.Context, string) (identityapplication.Authentication, error) { return authentication, nil },
		logout:               func(context.Context, *domain.Subject, string) error { return nil },
		currentUser:          func(context.Context, domain.Subject) (*domain.User, error) { return &user, nil },
		changePassword:       func(context.Context, domain.Subject, string, string) error { return nil },
		confirmPasswordReset: func(context.Context, string, string) error { return nil },
		listUsers:            func(context.Context) ([]domain.User, error) { return []domain.User{user}, nil },
		updateUser: func(context.Context, domain.Subject, int64, identityapplication.UserUpdate) (*domain.User, error) {
			return &user, nil
		},
		deleteUser:  func(context.Context, domain.Subject, int64) (*domain.User, error) { return &user, nil },
		restoreUser: func(context.Context, domain.Subject, int64) (*domain.User, error) { return &user, nil },
	}
}

type identityAuthenticatorStub struct{ subject httptransport.Subject }

func (stub identityAuthenticatorStub) Authenticate(context.Context, string) (httptransport.Subject, error) {
	if stub.subject.UserID == 0 {
		return httptransport.Subject{}, sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
	}
	return stub.subject, nil
}

func newIdentityRouter(service identityService, subject httptransport.Subject, secureCookie bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := config.Default()
	cfg.Authentication.AllowedOrigins = []string{"https://app.example.test"}
	cfg.Authentication.RefreshCookieSecure = secureCookie
	RegisterRoutes(router, service, identityAuthenticatorStub{subject: subject}, cfg)
	return router
}

func serveJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func serveAuthorized(t *testing.T, router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertResultEnvelope(t *testing.T, response *httptest.ResponseRecorder, wantCode int) {
	t.Helper()
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v; body=%s", err, response.Body.String())
	}
	if len(result) != 3 || result["code"] == nil || result["message"] == nil || result["data"] == nil {
		t.Fatalf("Result = %#v, want only code/message/data", result)
	}
	var code int
	if err := json.Unmarshal(result["code"], &code); err != nil {
		t.Fatalf("decode result code: %v", err)
	}
	if code != wantCode {
		t.Fatalf("Result code = %d, want %d; body=%s", code, wantCode, response.Body.String())
	}
	if wantCode != 0 && string(result["data"]) != "null" {
		t.Fatalf("error data = %s, want null", result["data"])
	}
}

func assertNoSensitiveResponseFields(t *testing.T, body string) {
	t.Helper()
	for _, field := range []string{"password_hash", "refresh_token", "verification_code", "smtp password"} {
		if strings.Contains(body, field) {
			t.Fatalf("response contains sensitive field %q: %s", field, body)
		}
	}
}

func assertRefreshCookie(t *testing.T, response *httptest.ResponseRecorder, secure, cleared bool) {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	cookies := result.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count = %d, want 1 (%#v)", len(cookies), cookies)
	}
	cookie := cookies[0]
	if cookie.Name != refreshCookieName || cookie.Path != refreshCookiePath || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != secure {
		t.Fatalf("refresh cookie = %#v, want name/path/HttpOnly/SameSite=Strict/Secure=%t", cookie, secure)
	}
	if cleared && (cookie.Value != "" || cookie.MaxAge >= 0) {
		t.Fatalf("cleared cookie = %#v, want empty expired cookie", cookie)
	}
	if !cleared && cookie.Value == "" {
		t.Fatal("issued refresh cookie has empty value")
	}
}
