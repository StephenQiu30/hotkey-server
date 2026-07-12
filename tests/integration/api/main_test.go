package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logging"
	"github.com/StephenQiu30/hotkey-server/internal/platform/security"
	"github.com/StephenQiu30/hotkey-server/tests/testutil"
)

func TestMain(m *testing.M) {
	_ = logging.Init("error", "json", "stdout")
	os.Exit(m.Run())
}

// TestIntegrationSmoke verifies the full wiring: register -> login -> protected endpoint.
func TestIntegrationSmoke(t *testing.T) {
	db := testutil.SetupTestDB(t)

	router := testutil.SetupTestRouter(t, db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Step 1: Verify email and register with the one-time ticket.
	sendBody := `{"email":"smoke@example.com","purpose":"register"}`
	sendResp, err := http.Post(ts.URL+"/api/v1/auth/verifications", "application/json", bytes.NewBufferString(sendBody))
	if err != nil || sendResp.StatusCode != http.StatusOK {
		t.Fatalf("send verification: status=%v err=%v", sendResp.StatusCode, err)
	}
	_ = sendResp.Body.Close()

	confirmBody := `{"email":"smoke@example.com","purpose":"register","code":"` + testutil.TestVerificationCode + `"}`
	confirmResp, err := http.Post(ts.URL+"/api/v1/auth/verifications/confirm", "application/json", bytes.NewBufferString(confirmBody))
	if err != nil {
		t.Fatal(err)
	}
	var ticketData struct {
		Ticket string `json:"ticket"`
	}
	if err := decodeData(confirmResp, &ticketData); err != nil {
		t.Fatal(err)
	}
	_ = confirmResp.Body.Close()

	regBody := fmt.Sprintf(`{"verification_ticket":%q,"password":"Passw0rd!","display_name":"Smoke Test"}`, ticketData.Ticket)
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

	var regResult struct {
		User struct {
			ID          int64  `json:"id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		} `json:"user"`
		Token string `json:"token"`
	}
	if err := decodeData(regResp, &regResult); err != nil {
		t.Fatalf("register decode: %v", err)
	}
	if regResult.User.ID == 0 || regResult.Token == "" {
		t.Fatal("register: expected non-zero user ID")
	}
	if regResult.User.Email != "smoke@example.com" {
		t.Fatalf("register: expected email smoke@example.com, got %s", regResult.User.Email)
	}
	if regResult.User.DisplayName != "Smoke Test" {
		t.Fatalf("register: expected display_name Smoke Test, got %s", regResult.User.DisplayName)
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

	var monitors []dto.Monitor
	if err := decodeData(monResp, &monitors); err != nil {
		t.Fatalf("monitors decode: %v", err)
	}
	// New user has no monitors, so empty list is expected.
	if monitors == nil {
		monitors = []dto.Monitor{} // normalize nil to empty
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

	var monitorsList []dto.Monitor
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

func TestIntegrationRegisterRejectsLegacyPayload(t *testing.T) {
	db := testutil.SetupTestDB(t)

	router := testutil.SetupTestRouter(t, db)
	ts := httptest.NewServer(router)
	defer ts.Close()

	body := `{"email":"legacy@example.com","password":"Passw0rd!","display_name":"Legacy"}`
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// decodeData extracts the data payload from the unified response envelope.
// It now also handles the new envelope with code/message fields.
func decodeData(resp *http.Response, out any) error {
	var envelope struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
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

	// Verify error uses unified envelope
	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusUnauthorized {
		t.Fatalf("expected UNAUTHORIZED code, got %d", body.Code)
	}
}

// TestUnifiedEnvelope verifies all API responses use the unified envelope
// with code, message, data, and request_id fields.
func TestUnifiedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.ErrorHandlerMiddleware())

	r.GET("/api/v1/test/ok", func(c *gin.Context) {
		platformhttp.RespondOK(c, gin.H{"key": "value"})
	})
	r.GET("/api/v1/test/page", func(c *gin.Context) {
		platformhttp.RespondPage(c, []string{"a", "b"}, 1, 10, 2)
	})
	r.GET("/api/v1/test/error", func(c *gin.Context) {
		c.Error(platformhttp.NewAppError(enum.ErrorCodeNotFound, nil))
	})

	ts := httptest.NewServer(r)
	defer ts.Close()

	// Test OK response
	t.Run("success", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/v1/test/ok")
		if err != nil {
			t.Fatalf("ok request failed: %v", err)
		}
		defer resp.Body.Close()

		var body struct {
			Code      int             `json:"code"`
			Data      json.RawMessage `json:"data"`
			RequestID string          `json:"request_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode ok: %v", err)
		}
		if body.Code != http.StatusOK {
			t.Fatalf("expected SUCCESS code, got %d", body.Code)
		}
		if len(body.Data) == 0 {
			t.Fatal("expected non-empty data")
		}
	})

	// Test Page response
	t.Run("page", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/v1/test/page")
		if err != nil {
			t.Fatalf("page request failed: %v", err)
		}
		defer resp.Body.Close()

		var body struct {
			Code      int             `json:"code"`
			Data      json.RawMessage `json:"data"`
			Page      int             `json:"page"`
			PageSize  int             `json:"page_size"`
			Total     int             `json:"total"`
			RequestID string          `json:"request_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		if body.Code != http.StatusOK {
			t.Fatalf("expected SUCCESS code, got %d", body.Code)
		}
		if body.Page != 1 || body.PageSize != 10 || body.Total != 2 {
			t.Fatalf("unexpected pagination: page=%d page_size=%d total=%d", body.Page, body.PageSize, body.Total)
		}
	})

	// Test Error response (via c.Error + ErrorHandlerMiddleware)
	t.Run("error", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/v1/test/error")
		if err != nil {
			t.Fatalf("error request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.StatusCode)
		}

		var body struct {
			Code      int             `json:"code"`
			Data      json.RawMessage `json:"data"`
			RequestID string          `json:"request_id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if body.Code != http.StatusNotFound {
			t.Fatalf("expected NOT_FOUND code, got %d", body.Code)
		}
		if string(body.Data) != "null" {
			t.Fatalf("expected null data on error, got %s", string(body.Data))
		}
	})
}

// ---------------------------------------------------------------------------
// Security integration tests (no DB required).
// ---------------------------------------------------------------------------

// newSecurityRouter builds a minimal Gin engine with the production middleware
// stack needed to test CORS, JWT auth, 404, 405, and panic recovery.
func newSecurityRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// CORS with an explicit allowlist (no wildcard).
	r.Use(platformhttp.CORSMiddleware([]string{"https://example.com"}, false))

	r.Use(platformhttp.SecurityHeadersMiddleware())
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.ContextMetadataMiddleware("test"))

	// Public routes.
	r.GET("/healthz", func(c *gin.Context) {
		platformhttp.RespondOK(c, gin.H{"status": "ok"})
	})

	// Protected routes.
	protected := r.Group("")
	protected.Use(platformhttp.AuthMiddleware("test-secret", "hotkey-server", "hotkey-web", false))
	protected.GET("/api/v1/monitors", func(c *gin.Context) {
		platformhttp.RespondOK(c, gin.H{"data": "ok"})
	})

	// 404 / 405 handlers.
	r.NoRoute(platformhttp.NoRouteHandler())
	r.NoMethod(platformhttp.NoMethodHandler())

	// Error handler.
	r.Use(platformhttp.ErrorHandlerMiddleware())

	return r
}

// TestCORSMiddlewareAllowedOrigin verifies that a request from an allowed
// origin receives the expected CORS headers.
func TestCORSMiddlewareAllowedOrigin(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin = https://example.com, got %q", origin)
	}
	if creds := resp.Header.Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("expected Access-Control-Allow-Credentials = true, got %q", creds)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Vary: Origin must be set.
	vary := resp.Header.Get("Vary")
	if !strings.Contains(vary, "Origin") {
		t.Errorf("expected Vary to contain Origin, got %q", vary)
	}
}

// TestCORSMiddlewareDeniedOrigin verifies that a request from a disallowed
// origin does NOT receive CORS headers.
func TestCORSMiddlewareDeniedOrigin(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Origin", "https://evil.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Access-Control-Allow-Origin header should not be present for denied origins.
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for denied origin, got %q", origin)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (denied origin still serves the resource), got %d", resp.StatusCode)
	}
}

// TestCORSMiddlewarePreflightAllowed verifies OPTIONS preflight with an allowed
// origin produces the correct status and headers.
func TestCORSMiddlewarePreflightAllowed(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", resp.StatusCode)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "https://example.com" {
		t.Errorf("expected Access-Control-Allow-Origin = https://example.com, got %q", origin)
	}
}

// TestCORSMiddlewarePreflightDenied verifies OPTIONS preflight with a denied
// origin returns 403.
func TestCORSMiddlewarePreflightDenied(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/healthz", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Origin", "https://evil.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for denied preflight, got %d", resp.StatusCode)
	}
}

// TestJWTMissing verifies that a protected endpoint without auth header
// returns 401 with the unified envelope.
func TestJWTMissing(t *testing.T) {
	router := newSecurityRouter()
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

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %d", body.Code)
	}
	if body.Data != nil && string(body.Data) != "null" {
		t.Errorf("expected null data on error, got %s", string(body.Data))
	}
}

// TestJWTInvalid verifies that a protected endpoint with a garbage token
// returns 401 with the unified envelope.
func TestJWTInvalid(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/monitors", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer this.is.garbage")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %d", body.Code)
	}
}

// TestJWTWrongAudience verifies that a token with an incorrect audience is
// rejected with 401.
func TestJWTWrongAudience(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create a token with wrong audience using custom claims.
	claims := security.AccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			Issuer:    "hotkey-server",
			Audience:  jwt.ClaimStrings{"wrong-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/monitors", nil)
	if err != nil {
		t.Fatalf("request creation failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %d", body.Code)
	}
}

// TestNotFoundHandler verifies that hitting an undefined route returns 404
// with the unified envelope.
func TestNotFoundHandler(t *testing.T) {
	router := newSecurityRouter()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/nonexistent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusNotFound {
		t.Errorf("expected NOT_FOUND code, got %d", body.Code)
	}
	if string(body.Data) != "null" {
		t.Errorf("expected null data, got %s", string(body.Data))
	}
}

// TestMethodNotAllowedHandler verifies that a route with the wrong HTTP method
// returns 405 with the unified envelope.
func TestMethodNotAllowedHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.ErrorHandlerMiddleware())
	r.POST("/api/v1/test", func(c *gin.Context) {
		platformhttp.RespondOK(c, gin.H{"ok": true})
	})
	r.NoMethod(platformhttp.NoMethodHandler())

	ts := httptest.NewServer(r)
	defer ts.Close()

	// GET a POST-only endpoint.
	resp, err := http.Get(ts.URL + "/api/v1/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected METHOD_NOT_ALLOWED code, got %d", body.Code)
	}
	if string(body.Data) != "null" {
		t.Errorf("expected null data, got %s", string(body.Data))
	}
}

// TestRecoveryMiddleware verifies that a handler that panics returns 500
// with the unified INTERNAL_ERROR envelope and no internal details leak.
func TestRecoveryMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RecoverMiddleware())
	r.Use(platformhttp.RequestIDMiddleware())
	r.Use(platformhttp.ErrorHandlerMiddleware())

	r.GET("/api/v1/panic", func(c *gin.Context) {
		panic("test panic")
	})

	// Use httptest.NewRecorder directly so panics are caught by
	// RecoverMiddleware before reaching the net/http server goroutine.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil)
	r.ServeHTTP(w, req)

	result := w.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", result.StatusCode)
	}

	bodyBytes := new(bytes.Buffer)
	_, _ = bodyBytes.ReadFrom(result.Body)
	bodyStr := bodyBytes.String()

	var body struct {
		Code      int             `json:"code"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.Unmarshal(bodyBytes.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body: %s)", err, bodyStr)
	}
	if body.Code != http.StatusInternalServerError {
		t.Errorf("expected INTERNAL_ERROR code, got %d", body.Code)
	}
	if string(body.Data) != "null" {
		t.Errorf("expected null data, got %s", string(body.Data))
	}
	// The panic message must never appear in the response body.
	if strings.Contains(bodyStr, "test panic") {
		t.Error("panic message leaked in response body")
	}
}
