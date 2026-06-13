package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestHandler() *Handler {
	repo := &fakeRepo{}
	svc := NewService(repo)
	return NewHandler(svc, "test-secret")
}

func TestRegisterHTTPSuccess(t *testing.T) {
	h := setupTestHandler()
	body := `{"email":"test@example.com","password":"Passw0rd!","display_name":"Test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var resp userResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %s", resp.Email)
	}
}

func TestRegisterHTTPDuplicateEmail(t *testing.T) {
	h := setupTestHandler()
	body := `{"email":"dup@example.com","password":"Passw0rd!","display_name":"Dup"}`
	// Register first time
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	// Register again with same email
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

func TestLoginHTTPSuccess(t *testing.T) {
	h := setupTestHandler()
	// Register first
	regBody := `{"email":"login@example.com","password":"Passw0rd!","display_name":"Login"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	// Login
	loginBody := `{"email":"login@example.com","password":"Passw0rd!"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestLoginHTTPWrongPassword(t *testing.T) {
	h := setupTestHandler()
	// Register first
	regBody := `{"email":"wrong@example.com","password":"Passw0rd!","display_name":"Wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(regBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	// Login with wrong password
	loginBody := `{"email":"wrong@example.com","password":"WrongPass!"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRegisterHTTPEndToEnd(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	handler := NewHandler(svc, "test-secret")

	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer ts.Close()

	// Register via HTTP
	body := `{"email":"e2e@example.com","password":"Passw0rd!","display_name":"E2E"}`
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var userResp userResponse
	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if userResp.Email != "e2e@example.com" {
		t.Fatalf("expected email e2e@example.com, got %s", userResp.Email)
	}

	// Login via HTTP
	loginBody := `{"email":"e2e@example.com","password":"Passw0rd!"}`
	loginResp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(loginBody))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", loginResp.StatusCode)
	}
}
