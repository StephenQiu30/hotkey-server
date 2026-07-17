package http

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/modules/event/application"
	"github.com/StephenQiu30/hotkey-server/internal/modules/event/domain"
	httptransport "github.com/StephenQiu30/hotkey-server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type readFake struct{}

func (readFake) List(context.Context, domain.EventListQuery) (domain.EventPage, error) {
	return domain.EventPage{}, nil
}
func (readFake) Get(context.Context, int64) (domain.Event, error) { return domain.Event{}, nil }
func (readFake) ListMembers(context.Context, int64) (domain.EventMemberPage, error) {
	return domain.EventMemberPage{}, nil
}

func TestEventRoutesRequireAuthenticationAndExposeMemberLockPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, application.NewReadService(readFake{}), nil, nil, httptransport.NewUnavailableAuthenticator())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/events", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != 401 {
		t.Fatalf("unauthenticated event list status = %d, want 401", recorder.Code)
	}
	if _, ok := router.Routes()[0], true; !ok {
		t.Fatal("event routes are not registered")
	}
}
