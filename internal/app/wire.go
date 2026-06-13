//go:build wireinject
// +build wireinject

package app

import (
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/google/wire"
)

// ConfigProviderSet is the Wire provider set for application configuration.
var ConfigProviderSet = wire.NewSet(config.Load)

// InitializeAPI initializes the API application configuration via Wire.
func InitializeAPI() (config.Config, error) {
	wire.Build(ConfigProviderSet)
	return config.Config{}, nil
}

// InitializeWorker initializes the Worker application configuration via Wire.
func InitializeWorker() (config.Config, error) {
	wire.Build(ConfigProviderSet)
	return config.Config{}, nil
}
