package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/notify"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var NotifyModule = fx.Module("notify",
	fx.Provide(gormimpl.NewNotifyRepo),
	fx.Provide(notify.NewService),
)
