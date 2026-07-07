package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var TrendModule = fx.Module("trend",
	fx.Provide(gormimpl.NewTrendRepo),
)
