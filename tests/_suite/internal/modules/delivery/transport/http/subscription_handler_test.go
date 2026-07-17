package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	deliveryapplication "github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type subscriptionServiceFake struct {
	subscription domain.Subscription
	createCalls  int
	rotateCalls  int
}

func (fake *subscriptionServiceFake) Create(_ context.Context, input deliveryapplication.CreateSubscriptionInput) (deliveryapplication.SubscriptionSecret, error) {
	fake.createCalls++
	fake.subscription.UserID, fake.subscription.ReportType, fake.subscription.Channel, fake.subscription.Timezone, fake.subscription.Schedule, fake.subscription.Enabled = input.Subject.UserID, input.ReportType, input.Channel, input.Timezone, input.Schedule, input.Enabled
	return deliveryapplication.SubscriptionSecret{Subscription: fake.subscription, RSSToken: "one-time-rss-token"}, nil
}
func (fake *subscriptionServiceFake) List(context.Context, identitydomain.Subject) ([]domain.Subscription, error) {
	return []domain.Subscription{fake.subscription}, nil
}
func (fake *subscriptionServiceFake) Get(context.Context, identitydomain.Subject, int64) (domain.Subscription, error) {
	return fake.subscription, nil
}
func (fake *subscriptionServiceFake) Update(_ context.Context, _ deliveryapplication.UpdateSubscriptionInput) (domain.Subscription, error) {
	fake.subscription.Version++
	return fake.subscription, nil
}
func (fake *subscriptionServiceFake) RotateRSSToken(_ context.Context, _ deliveryapplication.RotateRSSTokenInput) (deliveryapplication.SubscriptionSecret, error) {
	fake.rotateCalls++
	fake.subscription.Version++
	return deliveryapplication.SubscriptionSecret{Subscription: fake.subscription, RSSToken: "replacement-rss-token"}, nil
}

type subscriptionAuthenticator struct{}

func (subscriptionAuthenticator) Authenticate(context.Context, string) (httptransport.Subject, error) {
	return httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleViewer}, nil
}

func TestSubscriptionRoutesExposeTokenOnlyAtCreateOrRotation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &subscriptionServiceFake{subscription: domain.Subscription{ID: 7, Version: 1, UserID: 1, ReportType: "daily", Channel: domain.ChannelRSS, TokenHash: domain.TokenHash("private"), Timezone: "UTC", Schedule: "0 8 * * *", Enabled: true}}
	router := gin.New()
	RegisterSubscriptionRoutes(router, service, subscriptionAuthenticator{})

	if response := subscriptionRequest(router, http.MethodGet, "/api/v1/report-subscriptions", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d, want 401", response.Code)
	}
	create := subscriptionRequest(router, http.MethodPost, "/api/v1/report-subscriptions", `{"report_type":"daily","channel":"rss","timezone":"UTC","schedule":"0 8 * * *"}`, "viewer")
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), "one-time-rss-token") || strings.Contains(create.Body.String(), service.subscription.TokenHash) {
		t.Fatalf("create response = %d/%s", create.Code, create.Body.String())
	}
	list := subscriptionRequest(router, http.MethodGet, "/api/v1/report-subscriptions", "", "viewer")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "one-time-rss-token") || strings.Contains(list.Body.String(), service.subscription.TokenHash) {
		t.Fatalf("list response = %d/%s", list.Code, list.Body.String())
	}
	rotate := subscriptionRequest(router, http.MethodPost, "/api/v1/report-subscriptions/7/rss-token/rotate", `{"expected_version":1}`, "viewer")
	if rotate.Code != http.StatusOK || service.rotateCalls != 1 || !strings.Contains(rotate.Body.String(), "replacement-rss-token") || strings.Contains(rotate.Body.String(), service.subscription.TokenHash) {
		t.Fatalf("rotate response = %d/%s", rotate.Code, rotate.Body.String())
	}
}

func subscriptionRequest(router *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
