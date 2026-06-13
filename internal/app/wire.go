//go:build wireinject
// +build wireinject

package app

import (
	"net/http"

	"github.com/google/wire"

	"github.com/StephenQiu30/hotkey-server/internal/auth"
	"github.com/StephenQiu30/hotkey-server/internal/content"
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/server"
	"github.com/StephenQiu30/hotkey-server/internal/topic"
	"github.com/StephenQiu30/hotkey-server/internal/trend"
)

// APIProviderSet provides all dependencies for the API server.
var APIProviderSet = wire.NewSet(
	provideDB,
	provideAuthRepo,
	provideMonitorRepo,
	provideNotifyRepo,
	provideAuthMiddleware,
	providePostQueryService,
	provideTopicQueryService,
	provideTrendQueryService,
	auth.NewService,
	auth.NewHandler,
	monitor.NewService,
	monitor.NewHandler,
	notify.NewService,
	notify.NewHandler,
	content.NewPostHandler,
	topic.NewTopicHandler,
	trend.NewTrendHandler,
	server.NewRouter,
)
