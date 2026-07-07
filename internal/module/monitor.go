package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/monitor"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var MonitorModule = fx.Module("monitor",
	fx.Provide(gormimpl.NewMonitorRepo),
	fx.Provide(gormimpl.NewPostRepo),
	fx.Provide(gormimpl.NewHitRepo),
	fx.Provide(monitor.NewService),
)
