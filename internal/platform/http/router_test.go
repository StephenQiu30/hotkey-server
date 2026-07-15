package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpointsUseResultContract(t *testing.T) {
	t.Parallel()

	router := NewRouter(ReadinessFunc(func(context.Context) error { return nil }))

	for _, path := range []string{"/healthz", "/readyz"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			request := httptest.NewRequest(stdhttp.MethodGet, path, nil)
			router.ServeHTTP(response, request)

			if response.Code != stdhttp.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusOK)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("X-Request-ID header is empty")
			}
			assertResult(t, response, 0, "success", map[string]any{"status": "ok"})
		})
	}
}

func TestReadyDoesNotExposeInternalFailure(t *testing.T) {
	t.Parallel()

	router := NewRouter(ReadinessFunc(func(context.Context) error {
		return errors.New("postgres password=secret is unavailable")
	}))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/readyz", nil)

	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusServiceUnavailable)
	}
	assertResult(t, response, 90001, "service not ready", nil)
}

func assertResult(t *testing.T, response *httptest.ResponseRecorder, code int, message string, data any) {
	t.Helper()

	var got Result[any]
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Code != code {
		t.Errorf("code = %d, want %d", got.Code, code)
	}
	if got.Message != message {
		t.Errorf("message = %q, want %q", got.Message, message)
	}
	if data == nil {
		if got.Data != nil {
			t.Errorf("data = %#v, want nil", got.Data)
		}
		return
	}
	want, _ := json.Marshal(data)
	actual, _ := json.Marshal(got.Data)
	if string(actual) != string(want) {
		t.Errorf("data = %s, want %s", actual, want)
	}
}
