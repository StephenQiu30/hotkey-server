package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/hotevent"
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var HotEventModule = fx.Module("hot_event",
	fx.Provide(gormimpl.NewHotEventRepo),
	fx.Provide(hotevent.NewService),
	fx.Provide(hotevent.NewQueryService),
)
