package http

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/gin-gonic/gin"
)

type bilibiliWebhookServiceFake struct {
	called int
	openID string
}

func (service *bilibiliWebhookServiceFake) HandleBilibiliDeauthorization(_ context.Context, webhook domain.BilibiliWebhook) (bool, error) {
	service.called++
	service.openID = webhook.OpenID
	return true, nil
}

func TestBilibiliWebhookRouteEchoesOfficialChallengeAndRejectsWrongSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &bilibiliWebhookServiceFake{}
	router := gin.New()
	RegisterBilibiliWebhookRoutes(router, service, "fixture-secret")
	body := `{"event":"verify_webhooks","content":{"data":"challenge"}}`
	response := performBilibiliWebhook(t, router, body, bilibiliSignature(body, "fixture-secret"))
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"data":"challenge"}` || service.called != 0 {
		t.Fatalf("challenge response = %d %s, calls=%d", response.Code, response.Body.String(), service.called)
	}
	response = performBilibiliWebhook(t, router, body, strings.Repeat("0", 40))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong signature status = %d", response.Code)
	}
}

func TestBilibiliWebhookRouteDispatchesDeauthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &bilibiliWebhookServiceFake{}
	router := gin.New()
	RegisterBilibiliWebhookRoutes(router, service, "fixture-secret")
	body := `{"event":"deauthorize","content":{"openid":"creator_open_id"}}`
	response := performBilibiliWebhook(t, router, body, bilibiliSignature(body, "fixture-secret"))
	if response.Code != http.StatusOK || service.called != 1 || service.openID != "creator_open_id" {
		t.Fatalf("deauthorize response = %d %s, service=%#v", response.Code, response.Body.String(), service)
	}
}

func performBilibiliWebhook(t *testing.T, router http.Handler, body, signature string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/source-webhooks/bilibili", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-bilibili-signature", signature)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func bilibiliSignature(body, secret string) string {
	digest := sha1.Sum([]byte(secret + body))
	return hex.EncodeToString(digest[:])
}
