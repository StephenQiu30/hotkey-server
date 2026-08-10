package http

import (
	"context"
	stdhttp "net/http"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type serverLifecycle struct {
	hooks []fx.Hook
}

func (lifecycle *serverLifecycle) Append(hook fx.Hook) {
	lifecycle.hooks = append(lifecycle.hooks, hook)
}

func TestServerStopCancelsStreamingRequestsBeforeShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	streamStarted := make(chan struct{})
	requestCanceled := make(chan struct{})
	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		c.Status(stdhttp.StatusOK)
		_, _ = c.Writer.Write([]byte("ready\n"))
		c.Writer.Flush()
		close(streamStarted)
		<-c.Request.Context().Done()
		close(requestCanceled)
	})

	server := NewServer(config.Config{HTTPAddr: "127.0.0.1:0"}, router, zap.NewNop())
	lifecycle := &serverLifecycle{}
	RegisterServer(lifecycle, server)
	if len(lifecycle.hooks) != 1 {
		t.Fatalf("lifecycle hooks = %d, want 1", len(lifecycle.hooks))
	}
	if err := lifecycle.hooks[0].OnStart(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := stdhttp.Get("http://" + server.Address() + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	select {
	case <-streamStarted:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}

	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.hooks[0].OnStop(stopContext); err != nil {
		t.Fatalf("stop streaming server: %v", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("stream request context was not canceled")
	}
}
