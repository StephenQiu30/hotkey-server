package http_test

import (
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/service/monitortopic"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestMonitorTopicCRUDAndStatusHTTPFlow(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	deps := transporthttp.Dependencies{
		MonitorTopicService: svc,
	}
	router := transportRouterWithDependenciesForTest(deps)
	userToken := registerAndLogin(t, router, "topic-user@example.com")

	// List topics (empty)
	list := getWithBearer(router, "/api/v1/me/topics", userToken)
	if list.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}

	// Create topic
	create := postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":                "AI 热点",
		"description":         "监控 AI 领域热点",
		"language":            "zh",
		"platforms":           []string{"weibo", "twitter"},
		"similarityThreshold": 0.85,
		"collectIntervalMin":  15,
		"dailyReportEnabled":  true,
		"obsidianOutputDir":   "/vault/ai-hotspot",
	})
	if create.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", create.Code, create.Body.String())
	}
	topicID := jsonStringAt(t, create.Body.Bytes(), "topic.id")
	if topicID == "" {
		t.Fatalf("expected topic id: %s", create.Body.String())
	}
	assertJSONField(t, create.Body.Bytes(), "topic.name", "AI 热点")
	assertJSONField(t, create.Body.Bytes(), "topic.status", "draft")
	assertJSONField(t, create.Body.Bytes(), "topic.language", "zh")

	// Get topic
	get := getWithBearer(router, "/api/v1/me/topics/"+topicID, userToken)
	if get.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", get.Code, get.Body.String())
	}
	assertJSONField(t, get.Body.Bytes(), "topic.name", "AI 热点")

	// Update topic
	update := patchJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID, userToken, map[string]any{
		"name":     "AI 热点监控",
		"language": "multi",
	})
	if update.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", update.Code, update.Body.String())
	}
	assertJSONField(t, update.Body.Bytes(), "topic.name", "AI 热点监控")
	assertJSONField(t, update.Body.Bytes(), "topic.language", "multi")

	// Activate topic (draft → active)
	activate := postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/status", userToken, map[string]any{
		"status": "active",
	})
	if activate.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", activate.Code, activate.Body.String())
	}
	assertJSONField(t, activate.Body.Bytes(), "topic.status", "active")

	// Pause topic (active → paused)
	pause := postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/status", userToken, map[string]any{
		"status": "paused",
	})
	if pause.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", pause.Code, pause.Body.String())
	}
	assertJSONField(t, pause.Body.Bytes(), "topic.status", "paused")

	// Invalid transition (paused → draft)
	invalid := postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/status", userToken, map[string]any{
		"status": "draft",
	})
	if invalid.Code != 409 {
		t.Fatalf("expected 409 for invalid transition, got %d: %s", invalid.Code, invalid.Body.String())
	}
	assertJSONField(t, invalid.Body.Bytes(), "error.code", "invalid_status_transition")

	// Add keyword
	addKw := postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/keywords", userToken, map[string]any{
		"word": "GPT-5",
		"type": "include",
	})
	if addKw.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", addKw.Code, addKw.Body.String())
	}
	kwID := jsonStringAt(t, addKw.Body.Bytes(), "keyword.id")

	// Add exclusion word
	addEx := postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/keywords", userToken, map[string]any{
		"word": "spam",
		"type": "exclude",
	})
	if addEx.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", addEx.Code, addEx.Body.String())
	}

	// List keywords
	listKw := getWithBearer(router, "/api/v1/me/topics/"+topicID+"/keywords", userToken)
	if listKw.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", listKw.Code, listKw.Body.String())
	}

	// Delete keyword
	delKw := deleteWithBearer(router, "/api/v1/me/topics/"+topicID+"/keywords/"+kwID, userToken)
	if delKw.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", delKw.Code, delKw.Body.String())
	}

	// Create with invalid validation (empty name)
	badCreate := postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"language":  "zh",
		"platforms": []string{"weibo"},
	})
	if badCreate.Code != 400 {
		t.Fatalf("expected 400 for empty name, got %d: %s", badCreate.Code, badCreate.Body.String())
	}

	// Create with invalid language
	badLang := postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":      "Bad",
		"language":  "klingon",
		"platforms": []string{"weibo"},
	})
	if badLang.Code != 400 {
		t.Fatalf("expected 400 for invalid language, got %d: %s", badLang.Code, badLang.Body.String())
	}

	// Delete topic
	del := deleteWithBearer(router, "/api/v1/me/topics/"+topicID, userToken)
	if del.Code != 204 {
		t.Fatalf("expected 204, got %d: %s", del.Code, del.Body.String())
	}

	// Verify deleted
	getDel := getWithBearer(router, "/api/v1/me/topics/"+topicID, userToken)
	if getDel.Code != 404 {
		t.Fatalf("expected 404 after delete, got %d: %s", getDel.Code, getDel.Body.String())
	}
}

func TestMonitorTopicOwnershipIsolation(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	deps := transporthttp.Dependencies{
		MonitorTopicService: svc,
	}
	router := transportRouterWithDependenciesForTest(deps)
	user1 := registerAndLogin(t, router, "owner@example.com")
	user2 := registerAndLogin(t, router, "other@example.com")

	// User 1 creates a topic
	create := postJSONWithBearer(t, router, "/api/v1/me/topics", user1, map[string]any{
		"name":      "Private Topic",
		"language":  "zh",
		"platforms": []string{"weibo"},
	})
	if create.Code != 201 {
		t.Fatalf("expected 201, got %d", create.Code)
	}
	topicID := jsonStringAt(t, create.Body.Bytes(), "topic.id")

	// User 2 cannot see user 1's topic
	get := getWithBearer(router, "/api/v1/me/topics/"+topicID, user2)
	if get.Code != 404 {
		t.Fatalf("expected 404 for other user, got %d", get.Code)
	}

	// User 2 cannot update user 1's topic
	update := patchJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID, user2, map[string]any{
		"name": "Hijacked",
	})
	if update.Code != 404 {
		t.Fatalf("expected 404 for other user update, got %d", update.Code)
	}

	// User 2 cannot delete user 1's topic
	del := deleteWithBearer(router, "/api/v1/me/topics/"+topicID, user2)
	if del.Code != 404 {
		t.Fatalf("expected 404 for other user delete, got %d", del.Code)
	}
}

func TestMonitorTopicValidationErrors(t *testing.T) {
	svc := monitortopic.NewService(monitortopic.NewMemoryRepository())
	deps := transporthttp.Dependencies{
		MonitorTopicService: svc,
	}
	router := transportRouterWithDependenciesForTest(deps)
	userToken := registerAndLogin(t, router, "validation@example.com")

	// Missing language
	resp := postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":      "Test",
		"platforms": []string{"weibo"},
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Missing platforms
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":     "Test",
		"language": "zh",
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Invalid platform
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":      "Test",
		"language":  "zh",
		"platforms": []string{"tiktok"},
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Threshold out of range
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":                "Test",
		"language":            "zh",
		"platforms":           []string{"weibo"},
		"similarityThreshold": 1.5,
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Interval below minimum
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":               "Test",
		"language":           "zh",
		"platforms":          []string{"weibo"},
		"collectIntervalMin": 2,
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}

	// Empty keyword word
	create := postJSONWithBearer(t, router, "/api/v1/me/topics", userToken, map[string]any{
		"name":      "Test",
		"language":  "zh",
		"platforms": []string{"weibo"},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201 for topic creation, got %d: %s", create.Code, create.Body.String())
	}
	topicID := jsonStringAt(t, create.Body.Bytes(), "topic.id")
	if topicID == "" {
		t.Fatal("expected non-empty topic ID from creation response")
	}
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/keywords", userToken, map[string]any{
		"word": "",
		"type": "include",
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400 for empty keyword, got %d: %s", resp.Code, resp.Body.String())
	}

	// Invalid keyword type
	resp = postJSONWithBearer(t, router, "/api/v1/me/topics/"+topicID+"/keywords", userToken, map[string]any{
		"word": "test",
		"type": "invalid",
	})
	if resp.Code != 400 {
		t.Fatalf("expected 400 for invalid keyword type, got %d: %s", resp.Code, resp.Body.String())
	}
}
