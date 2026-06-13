// Package config provides the Fx module for application configuration.
package config

import (
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"go.uber.org/fx"
)

// Module is an Fx module that provides application Config via Viper.
var Module = fx.Module("config", fx.Provide(config.Load))
