package config

// DatabaseConfig groups database and cache related configuration fields.
type DatabaseConfig struct {
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisAddr   string `mapstructure:"REDIS_ADDR"`
}
