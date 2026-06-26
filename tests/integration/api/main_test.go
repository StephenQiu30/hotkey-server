package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

// TestIntegrationSmoke verifies the full wiring: register -> login -> protected endpoint.
func TestIntegrationSmoke(t *testing.T) {
	db := testutil.SetupTestDB(t)

	router := testutil.SetupTestRouter(t, db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Step 1: Register a user.
	regBody := `{"email":"smoke@example.com","password":"Passw0rd!","display_name":"Smoke Test"}`
	regResp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewBufferString(regBody))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	defer regResp.Body.Close()

	if regResp.StatusCode != http.StatusCreated {
		var errBody map[string]string
		json.NewDecoder(regResp.Body).Decode(&errBody)
		t.Fatalf("register: expected 201, got %d: %v", regResp.StatusCode, errBody)
	}

	var regUser struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeData(regResp, &regUser); err != nil {
		t.Fatalf("register decode: %v", err)
	}
	if regUser.ID == 0 {
		t.Fatal("register: expected non-zero user ID")
	}
	if regUser.Email != "smoke@example.com" {
		t.Fatalf("register: expected email smoke@example.com, got %s", regUser.Email)
	}
	if regUser.DisplayName != "Smoke Test" {
		t.Fatalf("register: expected display_name Smoke Test, got %s", regUser.DisplayName)
	}

	// Step 2: Login and get token.
	loginBody := `{"email":"smoke@example.com","password":"Passw0rd!"}`
	loginResp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewBufferString(loginBody))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		var errBody map[string]string
		json.NewDecoder(loginResp.Body).Decode(&errBody)
		t.Fatalf("login: expected 200, got %d: %v", loginResp.StatusCode, errBody)
	}

	var loginResult struct {
		User  struct{ ID int64 } `json:"user"`
		Token string             `json:"token"`
	}
	if err := decodeData(loginResp, &loginResult); err != nil {
		t.Fatalf("login decode: %v", err)
	}
	if loginResult.Token == "" {
		t.Fatal("login: expected non-empty token")
	}

	// Step 3: Access protected endpoint with token.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/monitors", nil)
	req.Header.Set("Authorization", "Bearer "+loginResult.Token)
	monResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("monitors request failed: %v", err)
	}
	defer monResp.Body.Close()

	if monResp.StatusCode != http.StatusOK {
		var errBody map[string]string
		json.NewDecoder(monResp.Body).Decode(&errBody)
		t.Fatalf("monitors: expected 200, got %d: %v", monResp.StatusCode, errBody)
	}

	var monitors []monitor.Monitor
	if err := decodeData(monResp, &monitors); err != nil {
		t.Fatalf("monitors decode: %v", err)
	}
	// New user has no monitors, so empty list is expected.
	if monitors == nil {
		monitors = []monitor.Monitor{} // normalize nil to empty
	}

	// Step 4: Create a monitor, then list.
	createBody := `{"name":"AI News","query_text":"openai agent","poll_interval_minutes":10}`
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/monitors", bytes.NewBufferString(createBody))
	createReq.Header.Set("Authorization", "Bearer "+loginResult.Token)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create monitor request failed: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		var errBody map[string]string
		json.NewDecoder(createResp.Body).Decode(&errBody)
		t.Fatalf("create monitor: expected 201, got %d: %v", createResp.StatusCode, errBody)
	}

	// Verify list now has one monitor.
	listReq, _ := http.NewRequest("GET", ts.URL+"/api/v1/monitors", nil)
	listReq.Header.Set("Authorization", "Bearer "+loginResult.Token)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list monitors request failed: %v", err)
	}
	defer listResp.Body.Close()

	var monitorsList []monitor.Monitor
	if err := decodeData(listResp, &monitorsList); err != nil {
		t.Fatalf("list monitors decode: %v", err)
	}
	if len(monitorsList) != 1 {
		t.Fatalf("list monitors: expected 1, got %d", len(monitorsList))
	}
	if monitorsList[0].Name != "AI News" {
		t.Fatalf("list monitors: expected name AI News, got %s", monitorsList[0].Name)
	}

	// Step 5: Verify notifications endpoint works.
	notifReq, _ := http.NewRequest("GET", ts.URL+"/api/v1/notifications", nil)
	notifReq.Header.Set("Authorization", "Bearer "+loginResult.Token)
	notifResp, err := http.DefaultClient.Do(notifReq)
	if err != nil {
		t.Fatalf("notifications request failed: %v", err)
	}
	defer notifResp.Body.Close()

	if notifResp.StatusCode != http.StatusOK {
		t.Fatalf("notifications: expected 200, got %d", notifResp.StatusCode)
	}
}

// TestIntegrationRegisterReturnsRealFields verifies register returns non-empty user fields.
func TestIntegrationRegisterReturnsRealFields(t *testing.T) {
	db := testutil.SetupTestDB(t)

	router := testutil.SetupTestRouter(t, db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	email := fmt.Sprintf("user-%d@example.com", os.Getpid())
	body := fmt.Sprintf(`{"email":"%s","password":"Passw0rd!","display_name":"Real User"}`, email)
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var result struct {
		ID          int64  `json:"id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if err := decodeData(resp, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if result.ID == 0 {
		t.Error("expected non-zero user ID")
	}
	if result.Email == "" {
		t.Error("expected non-empty email")
	}
	if result.DisplayName == "" {
		t.Error("expected non-empty display_name")
	}
}

func decodeData(resp *http.Response, out any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, out)
}

// TestIntegrationProtectedEndpointRejectsNoToken verifies 401 without auth.
func TestIntegrationProtectedEndpointRejectsNoToken(t *testing.T) {
	db := testutil.SetupTestDB(t)

	router := testutil.SetupTestRouter(t, db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/monitors")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
