package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func generateTestHash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func setupTestHandler() *HTTPHandler {
	repo := &fakeRepo{}
	svc := NewService(repo, "test-secret")
	return NewHTTPHandler(svc)
}

func TestRegisterEndpoint(t *testing.T) {
	h := setupTestHandler()
	body := `{"email":"new@example.com","password":"Passw0rd!","displayName":"New User"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
}

func TestLoginEndpoint(t *testing.T) {
	repo := &fakeRepo{
		users: []User{{ID: 1, Email: "user@example.com", Status: "active"}},
	}
	// Generate a valid bcrypt hash for testing
	hash, _ := generateTestHash("Passw0rd!")
	repo.passwordHashes = []string{hash}

	svc := NewService(repo, "test-secret")
	h := NewHTTPHandler(svc)

	body := `{"email":"user@example.com","password":"Passw0rd!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var output LoginOutput
	if err := json.NewDecoder(rr.Body).Decode(&output); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if output.Token == "" {
		t.Fatal("expected non-empty token")
	}
}
