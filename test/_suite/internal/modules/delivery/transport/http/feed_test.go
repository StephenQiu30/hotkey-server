package http

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/delivery/application"
	"github.com/gin-gonic/gin"
)

type feedReader func(context.Context, string) (application.Feed, error)

func (reader feedReader) ReadFeed(ctx context.Context, tokenHash string) (application.Feed, error) {
	return reader(ctx, tokenHash)
}

func TestFeedSupportsETagAndLastModified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	updated := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	reader := feedReader(func(_ context.Context, tokenHash string) (application.Feed, error) {
		if len(tokenHash) != 64 {
			t.Fatalf("token hash length = %d", len(tokenHash))
		}
		return application.Feed{Title: "HotKey", Link: "https://example.test/feed", UpdatedAt: updated}, nil
	})
	router := gin.New()
	RegisterRoutes(router, reader)
	first := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/feeds/secret?format=atom", nil)
	router.ServeHTTP(first, request)
	if first.Code != 200 || first.Header().Get("ETag") == "" || first.Header().Get("Last-Modified") == "" {
		t.Fatalf("first feed response = %d, headers=%v", first.Code, first.Header())
	}
	second := httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/feeds/secret?format=atom", nil)
	request.Header.Set("If-None-Match", first.Header().Get("ETag"))
	router.ServeHTTP(second, request)
	if second.Code != 304 {
		t.Fatalf("conditional feed response = %d, want 304", second.Code)
	}
}
