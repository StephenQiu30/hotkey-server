package http

import (
	"context"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	httptransport "github.com/StephenQiu30/hotkey-server/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type pushSubscriptionHTTPServiceStub struct {
	capability application.PushCapabilityDTO
	registered application.RegisterPushSubscriptionCommand
	updated    application.UpdatePushSubscriptionCommand
	disabled   application.DisablePushSubscriptionCommand
	result     application.PushSubscriptionDTO
}

func (stub *pushSubscriptionHTTPServiceStub) Capability() application.PushCapabilityDTO {
	return stub.capability
}
func (stub *pushSubscriptionHTTPServiceStub) Register(_ context.Context, command application.RegisterPushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	stub.registered = command
	return stub.result, nil
}
func (stub *pushSubscriptionHTTPServiceStub) List(_ context.Context, query application.ListPushSubscriptionsQuery) (application.ListPushSubscriptionsResult, error) {
	result := application.ListPushSubscriptionsResult{Items: []application.PushSubscriptionDTO{}}
	if query.UserID == stub.result.UserID {
		result.Items = append(result.Items, stub.result)
	}
	return result, nil
}
func (stub *pushSubscriptionHTTPServiceStub) Update(_ context.Context, command application.UpdatePushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	stub.updated = command
	stub.result.Version = command.ExpectedVersion + 1
	return stub.result, nil
}
func (stub *pushSubscriptionHTTPServiceStub) Disable(_ context.Context, command application.DisablePushSubscriptionCommand) (application.PushSubscriptionDTO, error) {
	stub.disabled = command
	stub.result.Version = command.ExpectedVersion + 1
	stub.result.Status = "disabled"
	return stub.result, nil
}

func TestPushSubscriptionRoutesProjectCapabilityAndSafeDeviceMetadata(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	stub := &pushSubscriptionHTTPServiceStub{
		capability: application.PushCapabilityDTO{Available: true, VAPIDPublicKey: "public-vapid-key"},
		result: application.PushSubscriptionDTO{
			ID: 8, Version: 1, UserID: 1, DeviceLabel: "iPhone", Timezone: "Asia/Shanghai",
			TTLSeconds: 3600, Status: "active", MonitorIDs: []int64{4}, CreatedAt: now, UpdatedAt: now,
		},
	}
	router := pushSubscriptionRouter(t, stub, notificationAuthenticator{role: httptransport.RoleViewer})

	capability := performPushRequest(router, stdhttp.MethodGet, "/api/v1/notifications/push-capability", "", nil)
	if capability.Code != stdhttp.StatusOK || capability.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(capability.Body.String(), `"vapid_public_key":"public-vapid-key"`) {
		t.Fatalf("capability = %d %#v %s", capability.Code, capability.Header(), capability.Body.String())
	}
	list := performPushRequest(router, stdhttp.MethodGet, "/api/v1/notifications/push-subscriptions", "", nil)
	if list.Code != stdhttp.StatusOK || strings.Contains(list.Body.String(), "endpoint") || strings.Contains(list.Body.String(), "p256dh") || strings.Contains(list.Body.String(), "auth") {
		t.Fatalf("unsafe list = %d %s", list.Code, list.Body.String())
	}

	registration := `{"endpoint":"https://push.example/subscription/one","keys":{"p256dh":"key","auth":"auth"},"device_label":"iPhone","timezone":"Asia/Shanghai","ttl_seconds":3600,"monitor_ids":[4]}`
	created := performPushRequest(router, stdhttp.MethodPost, "/api/v1/notifications/push-subscriptions", registration, map[string]string{"Idempotency-Key": "push-http-register-1"})
	if created.Code != stdhttp.StatusCreated || created.Header().Get("ETag") != `"v1"` ||
		stub.registered.UserID != 1 || stub.registered.IdempotencyKey != "push-http-register-1" ||
		strings.Contains(created.Body.String(), "push.example") || strings.Contains(created.Body.String(), `"keys"`) {
		t.Fatalf("registration = %d %#v %s command=%#v", created.Code, created.Header(), created.Body.String(), stub.registered)
	}

	update := `{"device_label":"工作手机","timezone":"UTC","ttl_seconds":7200,"monitor_ids":[4]}`
	updated := performPushRequest(router, stdhttp.MethodPut, "/api/v1/notifications/push-subscriptions/8", update, map[string]string{"If-Match": `"v1"`})
	if updated.Code != stdhttp.StatusOK || updated.Header().Get("ETag") != `"v2"` || stub.updated.ExpectedVersion != 1 {
		t.Fatalf("update = %d %#v %s command=%#v", updated.Code, updated.Header(), updated.Body.String(), stub.updated)
	}
	disabled := performPushRequest(router, stdhttp.MethodDelete, "/api/v1/notifications/push-subscriptions/8", "", map[string]string{"If-Match": `"v2"`})
	if disabled.Code != stdhttp.StatusOK || disabled.Header().Get("ETag") != `"v3"` || stub.disabled.ExpectedVersion != 2 {
		t.Fatalf("disable = %d %#v %s command=%#v", disabled.Code, disabled.Header(), disabled.Body.String(), stub.disabled)
	}
}

func TestPushSubscriptionRoutesRejectAnonymousUnknownFieldsAndWeakVersions(t *testing.T) {
	stub := &pushSubscriptionHTTPServiceStub{result: application.PushSubscriptionDTO{ID: 8, Version: 1, UserID: 1}}
	authenticated := pushSubscriptionRouter(t, stub, notificationAuthenticator{role: httptransport.RoleViewer})
	for _, fixture := range []struct {
		name, method, path, body string
		headers                  map[string]string
	}{
		{name: "unknown field", method: stdhttp.MethodPost, path: "/api/v1/notifications/push-subscriptions", body: `{"endpoint":"https://push.example","keys":{"p256dh":"key","auth":"auth"},"device_label":"phone","timezone":"UTC","ttl_seconds":3600,"monitor_ids":[4],"secret":"leak"}`, headers: map[string]string{"Idempotency-Key": "push-http-reject-1"}},
		{name: "weak version", method: stdhttp.MethodPut, path: "/api/v1/notifications/push-subscriptions/8", body: `{"device_label":"phone","timezone":"UTC","ttl_seconds":3600,"monitor_ids":[4]}`, headers: map[string]string{"If-Match": `W/"v1"`}},
		{name: "missing version", method: stdhttp.MethodDelete, path: "/api/v1/notifications/push-subscriptions/8"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			response := performPushRequest(authenticated, fixture.method, fixture.path, fixture.body, fixture.headers)
			if response.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
	unauthenticated := pushSubscriptionRouter(t, stub, httptransport.NewUnavailableAuthenticator())
	response := performPushRequest(unauthenticated, stdhttp.MethodGet, "/api/v1/notifications/push-capability", "", nil)
	if response.Code != stdhttp.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, body = %s", response.Code, response.Body.String())
	}
}

func pushSubscriptionRouter(t *testing.T, service pushSubscriptionHTTPService, authenticator httptransport.Authenticator) *gin.Engine {
	t.Helper()
	handler, err := newPushSubscriptionHandler(service)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterPushSubscriptionRoutes(router, handler, authenticator)
	return router
}

func performPushRequest(router *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer fixture")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	router.ServeHTTP(response, request)
	return response
}
