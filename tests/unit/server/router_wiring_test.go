package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/server"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
	"github.com/StephenQiu30/hotkey-server/tests/testutil/fake/server"
)

// TestRealHandlersAreWired verifies that PostHandler, TopicHandler, and
// TrendHandler are mounted on the router and respond without 404.
// This prevents a repeat of STE-290 where handlers existed in packages
// but were never passed to NewRouter.
func TestRealHandlersAreWired(t *testing.T) {
	passThrough := func(next http.Handler) http.Handler {
		return next
	}

	postHandler := content.NewPostHandler(&fakeserver.PostQueryService{})
	topicHandler := topic.NewTopicHandler(&fakeserver.TopicQueryService{})
	trendHandler := trend.NewTrendHandler(&fakeserver.TrendQueryService{})

	deps := server.Dependencies{
		PostHandler:    postHandler,
		TopicHandler:   topicHandler,
		TrendHandler:   trendHandler,
		AuthMiddleware: passThrough,
	}

	router := server.NewRouter(deps)

	tests := []struct {
		name string
		path string
	}{
		{"posts", "/api/v1/monitors/1/posts"},
		{"topics", "/api/v1/monitors/1/topics"},
		{"monitor trends", "/api/v1/monitors/1/trends"},
		{"topic trends", "/api/v1/topics/1/trends"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("handler not wired for %s: got 404", tt.path)
			}
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s, got %d", tt.path, rr.Code)
			}
		})
	}
}
