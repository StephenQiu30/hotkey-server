package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestEmailPreferencesRequiresAuth(t *testing.T) {
	router := transporthttp.NewRouter()

	getReq := httptest.NewRequest("GET", "/api/v1/me/email", nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusUnauthorized {
		t.Fatalf("expected GET /me/email 401, got %d", getW.Code)
	}

	putBody, _ := json.Marshal(map[string]any{"weeklyEnabled": true})
	putReq := httptest.NewRequest("PUT", "/api/v1/me/email", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	router.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusUnauthorized {
		t.Fatalf("expected PUT /me/email 401, got %d", putW.Code)
	}
}

func TestEmailPreferencesGetDefaultValues(t *testing.T) {
	router := transporthttp.NewRouter()

	// Register and login
	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "email-defaults@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d", register.Code)
	}
	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "email-defaults@example.com",
		"password": "correct horse battery staple",
	})
	token := jsonStringAt(t, login.Body.Bytes(), "accessToken")

	// GET /me/email
	req := getWithBearer(router, "/api/v1/me/email", token)
	if req.Code != http.StatusOK {
		t.Fatalf("expected GET /me/email 200, got %d: %s", req.Code, req.Body.String())
	}

	dailySendAt := jsonStringAt(t, req.Body.Bytes(), "dailySendAt")
	if dailySendAt == "" {
		t.Fatal("expected dailySendAt to be set")
	}
}

func TestEmailPreferencesUpdateWeeklyEnabled(t *testing.T) {
	router := transporthttp.NewRouter()

	// Register and login
	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "email-update@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d", register.Code)
	}
	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "email-update@example.com",
		"password": "correct horse battery staple",
	})
	token := jsonStringAt(t, login.Body.Bytes(), "accessToken")

	// PUT /me/email
	putBody, _ := json.Marshal(map[string]any{
		"weeklyEnabled": true,
		"weeklySendAt":  "10:00",
	})
	putReq := httptest.NewRequest("PUT", "/api/v1/me/email", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("Authorization", "Bearer "+token)
	putW := httptest.NewRecorder()
	router.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("expected PUT /me/email 200, got %d: %s", putW.Code, putW.Body.String())
	}

	// GET /me/email to verify
	getReq := getWithBearer(router, "/api/v1/me/email", token)
	if getReq.Code != http.StatusOK {
		t.Fatalf("expected GET /me/email 200, got %d", getReq.Code)
	}

	weeklySendAt := jsonStringAt(t, getReq.Body.Bytes(), "weeklySendAt")
	if weeklySendAt != "10:00" {
		t.Fatalf("expected weeklySendAt to be '10:00', got %q", weeklySendAt)
	}
}
