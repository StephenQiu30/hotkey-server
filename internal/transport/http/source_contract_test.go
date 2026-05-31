package http_test

import (
	"net/http"
	"testing"
)

func TestSourceManagementRequiresAdminAndValidatesCompliance(t *testing.T) {
	router := transportRouterForTest()
	userToken := registerAndLogin(t, router, "source-user@example.com")
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	denied := getWithBearer(router, "/api/v1/admin/sources", userToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected user role to get 403 for source management, got %d with body %s", denied.Code, denied.Body.String())
	}

	missingCompliance := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "Public Page",
		"type":             "public_page",
		"url":              "https://example.com/ai",
		"fetchIntervalMin": 30,
		"rateLimitPerHour": 12,
	})
	if missingCompliance.Code != http.StatusBadRequest {
		t.Fatalf("expected public_page without compliance note to fail 400, got %d with body %s", missingCompliance.Code, missingCompliance.Body.String())
	}
	assertJSONField(t, missingCompliance.Body.Bytes(), "error.code", "compliance_note_required")

	create := postJSONWithBearer(t, router, "/api/v1/admin/sources", adminToken, map[string]any{
		"name":             "OpenAI RSS",
		"type":             "rss",
		"url":              "https://example.com/rss.xml",
		"fetchIntervalMin": 60,
		"channelIDs":       []string{"chn_ai_models"},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected rss source create 201, got %d with body %s", create.Code, create.Body.String())
	}
	sourceID := jsonStringAt(t, create.Body.Bytes(), "source.id")
	if sourceID == "" {
		t.Fatalf("expected source id in response: %s", create.Body.String())
	}

	update := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID, adminToken, map[string]any{
		"name":             "OpenAI Public",
		"type":             "public_page",
		"url":              "https://example.com/news",
		"complianceNote":   "Public pages only with documented rate limits.",
		"fetchIntervalMin": 120,
		"rateLimitPerHour": 4,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("expected source update 200, got %d with body %s", update.Code, update.Body.String())
	}
	assertJSONField(t, update.Body.Bytes(), "source.type", "public_page")

	disable := patchJSONWithBearer(t, router, "/api/v1/admin/sources/"+sourceID+"/status", adminToken, map[string]any{"status": "disabled"})
	if disable.Code != http.StatusOK {
		t.Fatalf("expected source disable 200, got %d with body %s", disable.Code, disable.Body.String())
	}
	assertJSONField(t, disable.Body.Bytes(), "source.status", "disabled")

	runs := getWithBearer(router, "/api/v1/admin/sources/"+sourceID+"/collection-runs", adminToken)
	if runs.Code != http.StatusOK {
		t.Fatalf("expected collection runs list 200, got %d with body %s", runs.Code, runs.Body.String())
	}
}
