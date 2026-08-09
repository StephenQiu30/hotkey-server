package http

import (
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSAllowsRightsIdempotencyAndConcurrencyHeaders(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(cors([]string{"https://app.example.test"}))
	router.POST("/api/v1/source-endpoints/:id/rights-decision-batches", func(c *gin.Context) { c.Status(stdhttp.StatusNoContent) })
	request := httptest.NewRequest(stdhttp.MethodOptions, "/api/v1/source-endpoints/42/rights-decision-batches", nil)
	request.Header.Set("Origin", "https://app.example.test")
	request.Header.Set("Access-Control-Request-Method", stdhttp.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "content-type,idempotency-key,if-match")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != stdhttp.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", response.Code)
	}
	allowed := strings.ToLower(response.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{"idempotency-key", "if-match"} {
		if !strings.Contains(allowed, header) {
			t.Fatalf("allowed headers = %q, missing %q", allowed, header)
		}
	}
}
