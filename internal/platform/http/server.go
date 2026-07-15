package http

import (
	"context"
	"errors"
	"net"
	stdhttp "net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Server struct {
	server *stdhttp.Server
	logger *zap.Logger
}

func NewServer(cfg config.Config, handler *gin.Engine, logger *zap.Logger) *Server {
	return &Server{
		server: &stdhttp.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger,
	}
}

func RegisterServer(lifecycle fx.Lifecycle, server *Server) {
	var listener net.Listener
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var err error
			listener, err = net.Listen("tcp", server.server.Addr)
			if err != nil {
				return err
			}
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
			return server.server.Shutdown(ctx)
		},
	})
}
