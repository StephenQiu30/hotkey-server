package http_test

import (
	"net/http"
	"testing"
)

func TestXSourceCreateUpdateAndDisableRequiresAdmin(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "x-user@example.com")
	adminToken := registerAdminAndLogin(t, router, "x-admin@example.com")

	// Non-admin cannot create X source.
	denied := postJSONWithBearer(t, router, "/api/v1/admin/sources", userToken, map[string]any{
		"name":             "X AI Feed",
		"type":             "x",
		"url":              "https://api.x.com/2/tweets/search/recent",
		"complianceNote":   "X public API v2.",
		"fetchIntervalMin": 30,
		"rateLimitPerHour": 30,
	})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected user role to get 403 for X source create, got %d with body %s", denied.Code, denied.Body.String())
	}

	// Admin can create X source.
	create := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "X AI Feed",
		"type":             "x",
		"url":              "https://api.x.com/2/tweets/search/recent",
		"complianceNote":   "X public API v2 recent search; OAuth 2.0 PKCE authorized.",
		"fetchIntervalMin": 30,
		"rateLimitPerHour": 30,
		"channelIDs":       []string{"chn_ai_models"},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected X source create 201, got %d with body %s", create.Code, create.Body.String())
	}
	sourceID := jsonStringAt(t, create.Body.Bytes(), "source.id")
	if sourceID == "" {
		t.Fatalf("expected source id in response: %s", create.Body.String())
	}
	assertJSONField(t, create.Body.Bytes(), "source.type", "x")

	// Admin can update X source.
	update := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID, adminToken, map[string]any{
		"name":             "X AI Feed Updated",
		"type":             "x",
		"url":              "https://api.x.com/2/tweets/search/recent",
		"complianceNote":   "X public API v2 recent search; OAuth 2.0 PKCE authorized.",
		"fetchIntervalMin": 60,
		"rateLimitPerHour": 15,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("expected X source update 200, got %d with body %s", update.Code, update.Body.String())
	}

	// Admin can disable X source.
	disable := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID+"/status", adminToken, map[string]any{"status": "disabled"})
	if disable.Code != http.StatusOK {
		t.Fatalf("expected X source disable 200, got %d with body %s", disable.Code, disable.Body.String())
	}
	assertJSONField(t, disable.Body.Bytes(), "source.status", "disabled")
}

func TestXSourceRequiresComplianceNote(t *testing.T) {
	router := transportRouterForTest()
	adminToken := registerAdminAndLogin(t, router, "x-compliance-admin@example.com")

	missingCompliance := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "X No Compliance",
		"type":             "x",
		"url":              "https://api.x.com/2/tweets/search/recent",
		"fetchIntervalMin": 30,
		"rateLimitPerHour": 30,
	})
	if missingCompliance.Code != http.StatusBadRequest {
		t.Fatalf("expected X source without compliance note to fail 400, got %d with body %s", missingCompliance.Code, missingCompliance.Body.String())
	}
	assertJSONField(t, missingCompliance.Body.Bytes(), "error.code", "compliance_note_required")
}

func TestXAuthEndpointsRequireAdmin(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "x-auth-user@example.com")
	adminToken := registerAdminAndLogin(t, router, "x-auth-admin@example.com")

	// Non-admin cannot access X auth endpoints.
	deniedAuth := getWithBearer(router, "/api/v1/admin/x/auth", userToken)
	if deniedAuth.Code != http.StatusForbidden {
		t.Fatalf("expected user role to get 403 for X auth, got %d", deniedAuth.Code)
	}

	deniedRevoke := postJSONWithBearer(t, router, "/api/v1/admin/x/auth/revoke", userToken, map[string]any{"sourceId": "src_x_1"})
	if deniedRevoke.Code != http.StatusForbidden {
		t.Fatalf("expected user role to get 403 for X revoke, got %d", deniedRevoke.Code)
	}

	// Admin can access X auth URL endpoint.
	authURL := getWithBearer(router, "/api/v1/admin/x/auth", adminToken)
	if authURL.Code != http.StatusOK {
		t.Fatalf("expected admin X auth URL 200, got %d with body %s", authURL.Code, authURL.Body.String())
	}
	if jsonStringAt(t, authURL.Body.Bytes(), "authURL") == "" {
		t.Fatalf("expected authURL in response: %s", authURL.Body.String())
	}
}

func TestXAuthStatusEndpointReturnsNotAuthorized(t *testing.T) {
	router := transportRouterForTest()
	adminToken := registerAdminAndLogin(t, router, "x-status-admin@example.com")

	// Before any authorization, status should indicate not authorized.
	status := getWithBearer(router, "/api/v1/admin/x/auth/status?sourceId=src_x_nonexistent", adminToken)
	if status.Code != http.StatusOK {
		t.Fatalf("expected X auth status 200, got %d with body %s", status.Code, status.Body.String())
	}
	assertJSONField(t, status.Body.Bytes(), "authorized", "false")
}
