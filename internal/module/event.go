package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var EventModule = fx.Module("event",
	fx.Provide(gormimpl.NewEventRepo),
)
