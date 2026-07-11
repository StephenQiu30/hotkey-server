package config

// ServerConfig groups HTTP server related configuration fields.
type ServerConfig struct {
	HTTPAddr       string `mapstructure:"HTTP_ADDR"`
	SwaggerEnabled bool   `mapstructure:"SWAGGER_ENABLED"`
	XToken         string `mapstructure:"X_BEARER_TOKEN"`
	XBaseURL       string `mapstructure:"X_BASE_URL"`
}

// AuthConfig groups authentication and security related configuration fields.
type AuthConfig struct {
	JWTSecret          string   `mapstructure:"JWT_SECRET"`
	JWTIssuer          string   `mapstructure:"JWT_ISSUER"`
	JWTAudience        string   `mapstructure:"JWT_AUDIENCE"`
	VerificationPepper string   `mapstructure:"AUTH_VERIFICATION_PEPPER"`
	WebAllowedOrigins  []string `mapstructure:"WEB_ALLOWED_ORIGINS"`
	CookieSecure       bool     `mapstructure:"AUTH_COOKIE_SECURE"`
	CookieDomain       string   `mapstructure:"AUTH_COOKIE_DOMAIN"`
}

// SMTPConfig groups SMTP email delivery related configuration fields.
type SMTPConfig struct {
	SMTPHost      string `mapstructure:"SMTP_HOST"`
	SMTPPort      int    `mapstructure:"SMTP_PORT"`
	SMTPUsername  string `mapstructure:"SMTP_USERNAME"`
	SMTPAuthCode  string `mapstructure:"SMTP_AUTH_CODE"`
	SMTPFromEmail string `mapstructure:"SMTP_FROM_EMAIL"`
	SMTPFromName  string `mapstructure:"SMTP_FROM_NAME"`
}
