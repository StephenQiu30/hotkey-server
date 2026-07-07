package module

import (
	"github.com/redis/go-redis/v9"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/database"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

var Infra = fx.Module("infra",
	fx.Provide(NewConfig),
	fx.Provide(NewDB),
	fx.Provide(NewRedis),
)

func NewConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func NewDB(cfg *config.Config) (*gorm.DB, error) {
	return database.Open(cfg.DatabaseURL)
}

func NewRedis(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddr,
	})
}
