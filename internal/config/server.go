package config

// ServerConfig groups HTTP server related configuration fields.
type ServerConfig struct {
	HTTPAddr       string `mapstructure:"HTTP_ADDR"`
	SwaggerEnabled bool   `mapstructure:"SWAGGER_ENABLED"`
	JWTSecret      string `mapstructure:"JWT_SECRET"`
	XToken         string `mapstructure:"X_BEARER_TOKEN"`
	XBaseURL       string `mapstructure:"X_BASE_URL"`
}
