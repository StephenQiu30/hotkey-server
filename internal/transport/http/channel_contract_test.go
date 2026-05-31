package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	serviceauth "github.com/StephenQiu30/hotkey-server/internal/service/auth"
	servicechannel "github.com/StephenQiu30/hotkey-server/internal/service/channel"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
	"golang.org/x/crypto/bcrypt"
)

func TestChannelSubscriptionKeywordAndPreferenceHTTPFlow(t *testing.T) {
	router := transportRouterForTest()

	unauthorized := getJSON(router, "/api/v1/me/channels")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated subscriptions to return 401, got %d with body %s", unauthorized.Code, unauthorized.Body.String())
	}

	userToken := registerAndLogin(t, router, "channels-user@example.com")
	adminToken := registerAdminAndLogin(t, router, "channels-admin@example.com")

	channels := getWithBearer(router, "/api/v1/channels", userToken)
	if channels.Code != http.StatusOK {
		t.Fatalf("expected channels 200, got %d with body %s", channels.Code, channels.Body.String())
	}
	channelID := jsonStringAt(t, channels.Body.Bytes(), "channels.0.id")
	if channelID == "" {
		t.Fatalf("expected seeded channel id in response: %s", channels.Body.String())
	}

	subscribe := postJSONWithBearer(t, router, "/api/v1/me/channels/"+channelID, userToken, nil)
	if subscribe.Code != http.StatusCreated {
		t.Fatalf("expected subscribe 201, got %d with body %s", subscribe.Code, subscribe.Body.String())
	}
	assertJSONField(t, subscribe.Body.Bytes(), "subscription.channel.id", channelID)

	subscriptions := getWithBearer(router, "/api/v1/me/channels", userToken)
	if subscriptions.Code != http.StatusOK {
		t.Fatalf("expected subscriptions 200, got %d with body %s", subscriptions.Code, subscriptions.Body.String())
	}
	assertJSONField(t, subscriptions.Body.Bytes(), "subscriptions.0.channel.id", channelID)

	keyword := postJSONWithBearer(t, router, "/api/v1/me/keywords", userToken, map[string]any{"keyword": "OpenAI Agents"})
	if keyword.Code != http.StatusCreated {
		t.Fatalf("expected keyword create 201, got %d with body %s", keyword.Code, keyword.Body.String())
	}
	keywordID := jsonStringAt(t, keyword.Body.Bytes(), "keyword.id")
	if keywordID == "" {
		t.Fatalf("expected keyword id: %s", keyword.Body.String())
	}

	updateKeyword := patchJSONWithBearer(t, router, "/api/v1/me/keywords/"+keywordID, userToken, map[string]any{
		"keyword": "Claude Code",
		"enabled": false,
	})
	if updateKeyword.Code != http.StatusOK {
		t.Fatalf("expected keyword update 200, got %d with body %s", updateKeyword.Code, updateKeyword.Body.String())
	}
	assertJSONField(t, updateKeyword.Body.Bytes(), "keyword.keyword", "Claude Code")

	preference := putJSONWithBearer(t, router, "/api/v1/me/preferences/daily-send-at", userToken, map[string]any{"dailySendAt": "07:45"})
	if preference.Code != http.StatusOK {
		t.Fatalf("expected preference update 200, got %d with body %s", preference.Code, preference.Body.String())
	}
	assertJSONField(t, preference.Body.Bytes(), "dailySendAt", "07:45")

	disable := patchJSONWithBearer(t, router, "/api/v1/admin/channels/"+channelID, adminToken, map[string]any{"status": "disabled"})
	if disable.Code != http.StatusOK {
		t.Fatalf("expected admin disable 200, got %d with body %s", disable.Code, disable.Body.String())
	}

	blocked := postJSONWithBearer(t, router, "/api/v1/me/channels/"+channelID, userToken, nil)
	if blocked.Code != http.StatusConflict {
		t.Fatalf("expected disabled channel subscribe 409, got %d with body %s", blocked.Code, blocked.Body.String())
	}
	assertJSONField(t, blocked.Body.Bytes(), "error.code", "channel_disabled")

	unsubscribe := deleteWithBearer(router, "/api/v1/me/channels/"+channelID, userToken)
	if unsubscribe.Code != http.StatusNoContent {
		t.Fatalf("expected unsubscribe 204, got %d with body %s", unsubscribe.Code, unsubscribe.Body.String())
	}

	createChannel := postJSONWithBearer(t, router, "/api/v1/admin/channels", adminToken, map[string]any{
		"name":        "AI Agents",
		"slug":        "ai-agents",
		"description": "agent tooling",
	})
	if createChannel.Code != http.StatusCreated {
		t.Fatalf("expected admin channel create 201, got %d with body %s", createChannel.Code, createChannel.Body.String())
	}
	customChannelID := jsonStringAt(t, createChannel.Body.Bytes(), "channel.id")
	deleteChannel := deleteWithBearer(router, "/api/v1/admin/channels/"+customChannelID, adminToken)
	if deleteChannel.Code != http.StatusNoContent {
		t.Fatalf("expected admin channel delete 204, got %d with body %s", deleteChannel.Code, deleteChannel.Body.String())
	}

	duplicateChannel := postJSONWithBearer(t, router, "/api/v1/admin/channels", adminToken, map[string]any{
		"name": "Duplicate",
		"slug": "ai-products",
	})
	if duplicateChannel.Code != http.StatusConflict {
		t.Fatalf("expected duplicate admin channel create 409, got %d with body %s", duplicateChannel.Code, duplicateChannel.Body.String())
	}
	assertJSONField(t, duplicateChannel.Body.Bytes(), "error.code", "channel_slug_already_exists")
}

func transportRouterForTest() http.Handler {
	authRepo := serviceauth.NewMemoryRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	_, _ = authRepo.CreateUser(context.Background(), user.User{
		ID:           "usr_admin",
		Email:        "channels-admin@example.com",
		PasswordHash: string(hash),
		Role:         user.RoleAdmin,
		Status:       user.StatusActive,
		Timezone:     "Asia/Shanghai",
		DailySendAt:  "08:30",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	authService, err := serviceauth.NewService(authRepo, serviceauth.Config{AccessTokenSecret: "test-router-secret"})
	if err != nil {
		panic(err)
	}
	channelService := servicechannel.NewService(servicechannel.NewMemoryRepository())
	return transporthttp.NewRouterWithServices(authService, channelService)
}

func registerAndLogin(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	register := postJSON(t, handler, "/api/v1/auth/register", map[string]string{
		"email":    email,
		"password": "correct horse battery staple",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d with body %s", register.Code, register.Body.String())
	}
	return loginToken(t, handler, email)
}

func registerAdminAndLogin(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	return loginToken(t, handler, email)
}

func loginToken(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	login := postJSON(t, handler, "/api/v1/auth/login", map[string]string{
		"email":    email,
		"password": "correct horse battery staple",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d with body %s", login.Code, login.Body.String())
	}
	return jsonStringAt(t, login.Body.Bytes(), "accessToken")
}

func getJSON(handler http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func postJSONWithBearer(t *testing.T, handler http.Handler, path string, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return jsonWithBearer(t, handler, http.MethodPost, path, token, body)
}

func patchJSONWithBearer(t *testing.T, handler http.Handler, path string, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return jsonWithBearer(t, handler, http.MethodPatch, path, token, body)
}

func putJSONWithBearer(t *testing.T, handler http.Handler, path string, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return jsonWithBearer(t, handler, http.MethodPut, path, token, body)
}

func deleteWithBearer(handler http.Handler, path string, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func jsonWithBearer(t *testing.T, handler http.Handler, method string, path string, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
