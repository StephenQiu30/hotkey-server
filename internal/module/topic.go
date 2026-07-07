package module

import (
	"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
	"go.uber.org/fx"
)

var TopicModule = fx.Module("topic",
	fx.Provide(gormimpl.NewTopicRepo),
)
