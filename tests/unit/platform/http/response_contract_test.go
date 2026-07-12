package platformhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
	platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

func assertExactEnvelope(t *testing.T, rr *httptest.ResponseRecorder, status int, errorCode enum.ErrorCode, data string) {
	t.Helper()
	if rr.Code != status {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 3 {
		t.Fatalf("unexpected envelope keys: %v", body)
	}
	if string(body["code"]) != strconv.Itoa(status) {
		t.Fatalf("code=%s", body["code"])
	}
	wantErrorCode, _ := json.Marshal(errorCode)
	if string(body["error_code"]) != string(wantErrorCode) {
		t.Fatalf("error_code=%s", body["error_code"])
	}
	if string(body["data"]) != data {
		t.Fatalf("data=%s", body["data"])
	}
	if _, exists := body["message"]; exists {
		t.Fatal("message must be absent")
	}
	if _, exists := body["request_id"]; exists {
		t.Fatal("request_id must be absent")
	}
}

func TestUnifiedResponseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformhttp.RequestIDMiddleware())
	r.GET("/ok", func(c *gin.Context) { platformhttp.RespondOK(c, gin.H{"ok": true}) })
	r.POST("/created", func(c *gin.Context) { platformhttp.RespondCreated(c, gin.H{"id": 1}) })
	r.GET("/unauthorized", func(c *gin.Context) {
		platformhttp.RespondError(c, enum.ErrorCodeInvalidCredentials, "must not be exposed")
	})
	r.GET("/internal", func(c *gin.Context) { platformhttp.RespondInternalError(c) })

	tests := []struct {
		method    string
		path      string
		status    int
		errorCode enum.ErrorCode
		data      string
	}{
		{http.MethodGet, "/ok", http.StatusOK, enum.ErrorCodeSuccess, `{"ok":true}`},
		{http.MethodPost, "/created", http.StatusCreated, enum.ErrorCodeSuccess, `{"id":1}`},
		{http.MethodGet, "/unauthorized", http.StatusUnauthorized, enum.ErrorCodeInvalidCredentials, `null`},
		{http.MethodGet, "/internal", http.StatusInternalServerError, enum.ErrorCodeInternal, `null`},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("X-Request-Id", "req-contract")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			assertExactEnvelope(t, rr, tt.status, tt.errorCode, tt.data)
			if rr.Header().Get("X-Request-Id") != "req-contract" {
				t.Fatalf("response header request id=%q", rr.Header().Get("X-Request-Id"))
			}
		})
	}
}

func TestUnifiedPageResponseContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/page", func(c *gin.Context) { platformhttp.RespondPage(c, []int{1}, 2, 10, 11) })
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/page", nil))

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"code", "error_code", "data", "page", "page_size", "total"} {
		if _, exists := body[key]; !exists {
			t.Fatalf("missing %s: %s", key, rr.Body.String())
		}
	}
	if len(body) != 6 {
		t.Fatalf("unexpected page envelope keys: %v", body)
	}
}
