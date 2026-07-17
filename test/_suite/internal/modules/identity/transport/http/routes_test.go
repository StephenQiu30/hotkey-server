package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

func TestRegisteredIdentityRouteGroupsUsePlatformGuards(t *testing.T) {
	t.Parallel()

	router := newIdentityRouter(successfulService(), httptransport.Subject{UserID: 1, SessionID: 2, Role: httptransport.RoleViewer}, false)
	for _, route := range []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodGet, path: "/api/v1/auth/me", want: http.StatusUnauthorized},
		{method: http.MethodPost, path: "/api/v1/auth/password", want: http.StatusUnauthorized},
		{method: http.MethodGet, path: "/api/v1/users", want: http.StatusUnauthorized},
		{method: http.MethodPatch, path: "/api/v1/users/3", want: http.StatusUnauthorized},
		{method: http.MethodDelete, path: "/api/v1/users/3", want: http.StatusUnauthorized},
		{method: http.MethodPost, path: "/api/v1/users/3/restore", want: http.StatusUnauthorized},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != route.want {
			t.Errorf("%s %s status = %d, want %d", route.method, route.path, response.Code, route.want)
		}
	}
}
