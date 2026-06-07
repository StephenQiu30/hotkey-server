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

func TestEmailPreferencesPartialUpdatePreservesWeeklyEnabled(t *testing.T) {
	router := transporthttp.NewRouter()

	// Register and login
	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "email-partial@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d", register.Code)
	}
	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "email-partial@example.com",
		"password": "correct horse battery staple",
	})
	token := jsonStringAt(t, login.Body.Bytes(), "accessToken")

	// First, enable weekly emails
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

	// Now update only dailySendAt (no weeklyEnabled in body)
	partialBody, _ := json.Marshal(map[string]any{
		"dailySendAt": "07:00",
	})
	partialReq := httptest.NewRequest("PUT", "/api/v1/me/email", bytes.NewReader(partialBody))
	partialReq.Header.Set("Content-Type", "application/json")
	partialReq.Header.Set("Authorization", "Bearer "+token)
	partialW := httptest.NewRecorder()
	router.ServeHTTP(partialW, partialReq)
	if partialW.Code != http.StatusOK {
		t.Fatalf("expected PUT /me/email 200, got %d: %s", partialW.Code, partialW.Body.String())
	}

	// GET /me/email — weeklyEnabled should still be true
	getReq := getWithBearer(router, "/api/v1/me/email", token)
	if getReq.Code != http.StatusOK {
		t.Fatalf("expected GET /me/email 200, got %d", getReq.Code)
	}

	var prefs map[string]any
	if err := json.Unmarshal(getReq.Body.Bytes(), &prefs); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	weeklyEnabled, ok := prefs["weeklyEnabled"].(bool)
	if !ok {
		t.Fatalf("expected weeklyEnabled to be a boolean, got %T", prefs["weeklyEnabled"])
	}
	if !weeklyEnabled {
		t.Fatal("expected weeklyEnabled to remain true after partial update (only dailySendAt changed)")
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
