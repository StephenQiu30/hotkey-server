package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

func TestResultAlwaysHasCodeMessageAndData(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		write  func(*gin.Context)
		status int
		data   any
	}{
		{
			name:   "ok",
			write:  func(c *gin.Context) { OK(c, map[string]string{"id": "monitor-1"}) },
			status: stdhttp.StatusOK,
			data:   map[string]any{"id": "monitor-1"},
		},
		{
			name:   "created",
			write:  func(c *gin.Context) { Created(c, map[string]string{"id": "monitor-1"}) },
			status: stdhttp.StatusCreated,
			data:   map[string]any{"id": "monitor-1"},
		},
		{
			name:   "empty",
			write:  Empty,
			status: stdhttp.StatusOK,
			data:   nil,
		},
		{
			name: "page",
			write: func(c *gin.Context) {
				PageOK(c, Page[string]{Items: []string{"one"}, Total: 1, Page: 1, PageSize: 20})
			},
			status: stdhttp.StatusOK,
			data: map[string]any{
				"items":     []any{"one"},
				"total":     float64(1),
				"page":      float64(1),
				"page_size": float64(20),
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			test.write(context)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			assertJSONResultShape(t, response, 0, "success", test.data)
		})
	}
}

func TestWriteErrorStatusMatrix(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		err    error
		status int
		code   int
	}{
		{"bad request", sharederrors.New(sharederrors.CodeInvalidRequest, stdhttp.StatusBadRequest, "ignored"), stdhttp.StatusBadRequest, sharederrors.CodeInvalidRequest},
		{"unauthenticated", sharederrors.New(sharederrors.CodeUnauthenticated, stdhttp.StatusUnauthorized, "ignored"), stdhttp.StatusUnauthorized, sharederrors.CodeUnauthenticated},
		{"forbidden", sharederrors.New(sharederrors.CodeForbidden, stdhttp.StatusForbidden, "ignored"), stdhttp.StatusForbidden, sharederrors.CodeForbidden},
		{"not found", sharederrors.New(sharederrors.CodeNotFound, stdhttp.StatusNotFound, "ignored"), stdhttp.StatusNotFound, sharederrors.CodeNotFound},
		{"conflict", sharederrors.New(sharederrors.CodeConflict, stdhttp.StatusConflict, "ignored"), stdhttp.StatusConflict, sharederrors.CodeConflict},
		{"rate limited", sharederrors.New(sharederrors.CodeRateLimited, stdhttp.StatusTooManyRequests, "ignored"), stdhttp.StatusTooManyRequests, sharederrors.CodeRateLimited},
		{"internal", errors.New("postgres password=secret"), stdhttp.StatusInternalServerError, sharederrors.CodeInternal},
		{"bad gateway", sharederrors.New(sharederrors.CodeBadGateway, stdhttp.StatusBadGateway, "ignored"), stdhttp.StatusBadGateway, sharederrors.CodeBadGateway},
		{"unavailable", sharederrors.New(sharederrors.CodeUnavailable, stdhttp.StatusServiceUnavailable, "ignored"), stdhttp.StatusServiceUnavailable, sharederrors.CodeUnavailable},
		{"deadline", context.DeadlineExceeded, stdhttp.StatusGatewayTimeout, sharederrors.CodeDeadlineExceeded},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(response)
			WriteError(ginContext, test.err)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			assertJSONResultShape(t, response, test.code, "", nil)
			if string(response.Body.Bytes()) == "" || string(response.Body.Bytes()) == "postgres password=secret" {
				t.Fatal("response leaked internal error")
			}
		})
	}
}

func TestWrapRecoversPanic(t *testing.T) {
	router, telemetry := newRouterForTest(t, ReadinessFunc(func(context.Context) error { return nil }))
	defer func() { _ = telemetry.Shutdown(context.Background()) }()
	router.GET("/panic", Wrap(func(*gin.Context) error {
		panic("postgres password=secret")
	}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "/panic", nil))

	if response.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, stdhttp.StatusInternalServerError)
	}
	assertJSONResultShape(t, response, sharederrors.CodeInternal, "", nil)
	if string(response.Body.Bytes()) == "postgres password=secret" {
		t.Fatal("response leaked panic value")
	}
}

func assertJSONResultShape(t *testing.T, response *httptest.ResponseRecorder, code int, message string, data any) {
	t.Helper()

	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	keys := make([]string, 0, len(body))
	for key := range body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if got, want := keys, []string{"code", "data", "message"}; !equalStrings(got, want) {
		t.Fatalf("response keys = %v, want %v", got, want)
	}

	var got Result[any]
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.Code != code {
		t.Errorf("code = %d, want %d", got.Code, code)
	}
	if message != "" && got.Message != message {
		t.Errorf("message = %q, want %q", got.Message, message)
	}
	if data == nil {
		if string(body["data"]) != "null" {
			t.Errorf("data = %s, want null", body["data"])
		}
		return
	}
	var gotData any
	if err := json.Unmarshal(body["data"], &gotData); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	if !reflect.DeepEqual(gotData, data) {
		t.Errorf("data = %#v, want %#v", gotData, data)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
