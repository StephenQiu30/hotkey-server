package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/platform/crypto"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
	"github.com/gin-gonic/gin"
)

func newRouterWithAuthorization(t *testing.T) *gin.Engine {
	t.Helper()
	repo := serviceauth.NewMemoryRepository()
	authSvc, err := serviceauth.NewService(repo, serviceauth.Config{AccessTokenSecret: "test-az-secret"})
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	enc, err := crypto.NewAESGCMEncryptor(key)
	if err != nil {
		t.Fatal(err)
	}
	azSvc, err := serviceauth.NewAuthorizationService(repo, nil, enc, nil)
	if err != nil {
		t.Fatal(err)
	}
	return transporthttp.NewRouterWithDependencies(transporthttp.Dependencies{
		AuthService:          authSvc,
		AuthorizationService: azSvc,
	})
}

func TestAuthorizationHTTPConnectAndList(t *testing.T) {
	router := newRouterWithAuthorization(t)

	// Register and login
	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "az-flow@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d: %s", register.Code, register.Body.String())
	}

	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "az-flow@example.com",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", login.Code, login.Body.String())
	}
	accessToken := jsonStringAt(t, login.Body.Bytes(), "accessToken")

	// Connect GitHub authorization
	connect := postJSONWithBearer(t, router, "/api/v1/authorizations/connect", accessToken, map[string]string{
		"platform":       "github",
		"platformUserId": "github-user-123",
		"displayName":    "Test GitHub",
		"accessToken":    "ghp_test_token_abc",
	})
	if connect.Code != http.StatusCreated {
		t.Fatalf("expected connect 201, got %d: %s", connect.Code, connect.Body.String())
	}
	assertJSONField(t, connect.Body.Bytes(), "authorization.status", "connected")

	// List authorizations
	list := getWithBearer(router, "/api/v1/authorizations", accessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", list.Code, list.Body.String())
	}

	// Test authorization health
	azID := jsonStringAt(t, connect.Body.Bytes(), "authorization.id")
	testPath := "/api/v1/authorizations/" + azID + "/test"
	test := postJSONWithBearer(t, router, testPath, accessToken, nil)
	if test.Code != http.StatusOK {
		t.Fatalf("expected test 200, got %d: %s", test.Code, test.Body.String())
	}

	// Disconnect authorization
	deletePath := "/api/v1/authorizations/" + azID
	deleteReq := deleteWithBearer(router, deletePath, accessToken)
	if deleteReq.Code != http.StatusNoContent {
		t.Fatalf("expected disconnect 204, got %d: %s", deleteReq.Code, deleteReq.Body.String())
	}

	// Verify revoked in list
	list2 := getWithBearer(router, "/api/v1/authorizations", accessToken)
	if list2.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", list2.Code, list2.Body.String())
	}
	var listResp2 struct {
		Authorizations []map[string]interface{} `json:"authorizations"`
	}
	if err := json.Unmarshal(list2.Body.Bytes(), &listResp2); err != nil {
		t.Fatalf("failed to parse list2 response: %v", err)
	}
	if len(listResp2.Authorizations) != 1 || listResp2.Authorizations[0]["status"] != "revoked" {
		t.Fatalf("expected 1 revoked authorization, got %v", listResp2.Authorizations)
	}
}

func TestAuthorizationHTTPDeleteAccount(t *testing.T) {
	router := newRouterWithAuthorization(t)

	// Register and login
	register := postJSON(t, router, "/api/v1/auth/register", map[string]string{
		"email":    "delete-account@example.com",
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d: %s", register.Code, register.Body.String())
	}

	login := postJSON(t, router, "/api/v1/auth/login", map[string]string{
		"email":    "delete-account@example.com",
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d: %s", login.Code, login.Body.String())
	}
	accessToken := jsonStringAt(t, login.Body.Bytes(), "accessToken")

	// Connect authorization
	connect := postJSONWithBearer(t, router, "/api/v1/authorizations/connect", accessToken, map[string]string{
		"platform":       "github",
		"platformUserId": "gh-1",
		"displayName":    "GitHub",
		"accessToken":    "ghp_token",
	})
	if connect.Code != http.StatusCreated {
		t.Fatalf("expected connect 201, got %d: %s", connect.Code, connect.Body.String())
	}

	// Delete account
	deleteAccount := deleteWithBearer(router, "/api/v1/me", accessToken)
	if deleteAccount.Code != http.StatusNoContent {
		t.Fatalf("expected delete account 204, got %d: %s", deleteAccount.Code, deleteAccount.Body.String())
	}

	// Verify token no longer works
	me := getWithBearer(router, "/api/v1/me", accessToken)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("expected me 401 after delete, got %d: %s", me.Code, me.Body.String())
	}
}

func TestAuthorizationHTTPUnauthorized(t *testing.T) {
	router := newRouterWithAuthorization(t)

	// List without auth should fail
	list := getWithBearer(router, "/api/v1/authorizations", "")
	if list.Code != http.StatusUnauthorized {
		t.Fatalf("expected list 401, got %d: %s", list.Code, list.Body.String())
	}

	// Connect without auth should fail
	connect := postJSONWithBearer(t, router, "/api/v1/authorizations/connect", "", map[string]string{
		"platform": "github",
	})
	if connect.Code != http.StatusUnauthorized {
		t.Fatalf("expected connect 401, got %d: %s", connect.Code, connect.Body.String())
	}
}

// Helper functions are defined in channel_contract_test.go
