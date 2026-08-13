package http

import (
	"context"
	"errors"
	"net"
	stdhttp "net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Server struct {
	server         *stdhttp.Server
	logger         *zap.Logger
	listener       net.Listener
	cancelRequests context.CancelFunc
}

func (s *Server) Address() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func NewServer(cfg config.Config, handler *gin.Engine, logger *zap.Logger) *Server {
	requestContext, cancelRequests := context.WithCancel(context.Background())
	return &Server{
		server: &stdhttp.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
			BaseContext: func(net.Listener) context.Context {
				return requestContext
			},
		},
		logger:         logger,
		cancelRequests: cancelRequests,
	}
}

func RegisterServer(lifecycle fx.Lifecycle, server *Server) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var err error
			listener, err := net.Listen("tcp", server.server.Addr)
			if err != nil {
				return err
			}
			server.listener = listener
			server.logger.Info("HTTP server started", zap.String("address", listener.Addr().String()))
			go func() {
				if err := server.server.Serve(listener); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
					server.logger.Error("HTTP server stopped unexpectedly", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			server.logger.Info("HTTP server stopping")
			// Long-lived streams do not become idle just because Shutdown stops
			// accepting new connections. Cancel the shared request base context
			// first so long-running handlers can finish inside the
			// graceful-shutdown deadline.
			if server.cancelRequests != nil {
				server.cancelRequests()
			}
			shutdownErr := server.server.Shutdown(ctx)
			if server.listener != nil {
				closeErr := server.listener.Close()
				if closeErr != nil && !errors.Is(closeErr, net.ErrClosed) && shutdownErr == nil {
					shutdownErr = closeErr
				}
				server.listener = nil
			}
			return shutdownErr
		},
	})
}
