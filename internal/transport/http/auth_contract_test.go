package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestAuthHTTPFlowAndAdminDenial(t *testing.T) {
	router := transporthttp.NewRouter()

	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "flow@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d with body %s", register.Code, register.Body.String())
	}
	assertJSONField(t, register.Body.Bytes(), "user.role", "user")

	duplicate := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "flow@example.com",
		"password": "correct horse battery staple",
	})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected duplicate 409, got %d with body %s", duplicate.Code, duplicate.Body.String())
	}
	assertJSONField(t, duplicate.Body.Bytes(), "error.code", "email_already_exists")

	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "flow@example.com",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d with body %s", login.Code, login.Body.String())
	}
	accessToken := jsonStringAt(t, login.Body.Bytes(), "accessToken")
	refreshToken := jsonStringAt(t, login.Body.Bytes(), "refreshToken")
	if accessToken == "" || refreshToken == "" {
		t.Fatalf("expected tokens in login response: %s", login.Body.String())
	}

	wrongPassword := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "flow@example.com",
		"password": "wrong password",
	})
	if wrongPassword.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong password 401, got %d with body %s", wrongPassword.Code, wrongPassword.Body.String())
	}
	assertJSONField(t, wrongPassword.Body.Bytes(), "error.code", "invalid_credentials")

	me := getWithBearer(router, "/api/v1/me", accessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d with body %s", me.Code, me.Body.String())
	}
	assertJSONField(t, me.Body.Bytes(), "user.email", "flow@example.com")

	admin := getWithBearer(router, "/api/v1/admin/healthz", accessToken)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("expected admin endpoint 403 for user role, got %d with body %s", admin.Code, admin.Body.String())
	}

	refresh := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	})
	if refresh.Code != http.StatusOK {
		t.Fatalf("expected refresh 200, got %d with body %s", refresh.Code, refresh.Body.String())
	}
	if jsonStringAt(t, refresh.Body.Bytes(), "accessToken") == "" {
		t.Fatalf("expected access token in refresh response: %s", refresh.Body.String())
	}
	rotatedRefreshToken := jsonStringAt(t, refresh.Body.Bytes(), "refreshToken")
	if rotatedRefreshToken == "" || rotatedRefreshToken == refreshToken {
		t.Fatalf("expected rotated refresh token in refresh response: %s", refresh.Body.String())
	}

	logout := postJSON(t, router, "/api/v1/auth/logout", map[string]string{
		"refreshToken": rotatedRefreshToken,
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("expected logout 204, got %d with body %s", logout.Code, logout.Body.String())
	}

	revoked := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{
		"refreshToken": refreshToken,
	})
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked refresh 401, got %d with body %s", revoked.Code, revoked.Body.String())
	}
}

func TestAdminDisableUserHTTP(t *testing.T) {
	router := transportRouterForTest()

	// Register a regular user
	userToken := registerAndLogin(t, router, "disable-target@example.com")
	me := getWithBearer(router, "/api/v1/me", userToken)
	if me.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d: %s", me.Code, me.Body.String())
	}
	userID := jsonStringAt(t, me.Body.Bytes(), "user.id")

	// Regular user cannot access admin disable endpoint
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")
	denied := postJSONWithBearer(t, router, "/api/v1/admin/users/"+userID+"/disable", userToken, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin disable 403, got %d: %s", denied.Code, denied.Body.String())
	}

	// Admin can disable user
	disable := postJSONWithBearer(t, router, "/api/v1/admin/users/"+userID+"/disable", adminToken, nil)
	if disable.Code != http.StatusNoContent {
		t.Fatalf("expected admin disable 204, got %d: %s", disable.Code, disable.Body.String())
	}

	// Disabled user cannot login
	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "disable-target@example.com",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled user login 401, got %d: %s", login.Code, login.Body.String())
	}

	// Disabled user's existing token no longer works
	meAfter := getWithBearer(router, "/api/v1/me", userToken)
	if meAfter.Code != http.StatusUnauthorized {
		t.Fatalf("expected disabled user me 401, got %d: %s", meAfter.Code, meAfter.Body.String())
	}
}

func TestAdminRevokeAllTokensHTTP(t *testing.T) {
	router := transportRouterForTest()

	// Register a user and create two sessions
	registerAndLogin(t, router, "revoke-target@example.com")
	login2 := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "revoke-target@example.com",
		"password": "correct horse battery staple",
	})
	if login2.Code != http.StatusOK {
		t.Fatalf("expected second login 200, got %d: %s", login2.Code, login2.Body.String())
	}
	refreshToken1 := jsonStringAt(t, login2.Body.Bytes(), "refreshToken")

	// Create a third session to get its refresh token
	login3 := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "revoke-target@example.com",
		"password": "correct horse battery staple",
	})
	if login3.Code != http.StatusOK {
		t.Fatalf("expected third login 200, got %d: %s", login3.Code, login3.Body.String())
	}
	refreshToken2 := jsonStringAt(t, login3.Body.Bytes(), "refreshToken")

	// Get user ID
	userToken := jsonStringAt(t, login2.Body.Bytes(), "accessToken")
	me := getWithBearer(router, "/api/v1/me", userToken)
	userID := jsonStringAt(t, me.Body.Bytes(), "user.id")

	// Both refresh tokens work initially; save rotated tokens
	refresh1Resp := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{"refreshToken": refreshToken1})
	if refresh1Resp.Code != http.StatusOK {
		t.Fatalf("expected refresh1 200, got %d", refresh1Resp.Code)
	}
	rotatedToken1 := jsonStringAt(t, refresh1Resp.Body.Bytes(), "refreshToken")

	refresh2Resp := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{"refreshToken": refreshToken2})
	if refresh2Resp.Code != http.StatusOK {
		t.Fatalf("expected refresh2 200, got %d", refresh2Resp.Code)
	}
	rotatedToken2 := jsonStringAt(t, refresh2Resp.Body.Bytes(), "refreshToken")

	// Admin revokes all tokens
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")
	revoke := postJSONWithBearer(t, router, "/api/v1/admin/users/"+userID+"/revoke-tokens", adminToken, nil)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("expected admin revoke-tokens 204, got %d: %s", revoke.Code, revoke.Body.String())
	}

	// Rotated refresh tokens should no longer work
	if got := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{"refreshToken": rotatedToken1}); got.Code != http.StatusUnauthorized {
		t.Fatalf("expected rotated1 401 after revoke, got %d", got.Code)
	}
	if got := postJSON(t, router, "/api/v1/auth/refresh", map[string]string{"refreshToken": rotatedToken2}); got.Code != http.StatusUnauthorized {
		t.Fatalf("expected rotated2 401 after revoke, got %d", got.Code)
	}

	// User can still login again (unlike disable)
	relogin := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "revoke-target@example.com",
		"password": "correct horse battery staple",
	})
	if relogin.Code != http.StatusOK {
		t.Fatalf("expected relogin 200 after revoke, got %d: %s", relogin.Code, relogin.Body.String())
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func getWithBearer(handler http.Handler, path string, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func assertJSONField(t *testing.T, body []byte, path string, want string) {
	t.Helper()
	got := jsonStringAt(t, body, path)
	if got != want {
		t.Fatalf("expected %s=%q, got %q in %s", path, want, got, string(body))
	}
}

func jsonStringAt(t *testing.T, body []byte, path string) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("invalid JSON %s: %v", string(body), err)
	}
	var current any = decoded
	for _, key := range bytes.Split([]byte(path), []byte(".")) {
		if index, err := strconv.Atoi(string(key)); err == nil {
			array, ok := current.([]any)
			if !ok || index < 0 || index >= len(array) {
				return ""
			}
			current = array[index]
			continue
		}
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[string(key)]
	}
	value, _ := current.(string)
	return value
}
