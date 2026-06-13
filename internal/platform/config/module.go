package config

import (
	"go.uber.org/fx"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

// Module provides configuration to the Fx container.
var Module = fx.Module("config",
	fx.Provide(config.Load),
)
