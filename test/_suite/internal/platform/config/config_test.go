package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultIsValid(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Default().Validate() error = %v", err)
	}
	if cfg.Role != "all" {
		t.Fatalf("Default().Role = %q, want all", cfg.Role)
	}
}

func TestValidateRejectsInvalidRuntimeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "role", mutate: func(c *Config) { c.Role = "scheduler" }},
		{name: "http address", mutate: func(c *Config) { c.HTTPAddr = "" }},
		{name: "shutdown timeout", mutate: func(c *Config) { c.ShutdownTimeout = 0 }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := Default()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want an error")
			}
		})
	}
}

func TestConfigValidateRejectsUnsupportedEnvironment(t *testing.T) {
	t.Parallel()

	for _, environment := range []string{"development", "testing", "production"} {
		environment := environment
		t.Run(environment, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			cfg.Environment = environment
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() supported environment error = %v", err)
			}
		})
	}

	cfg := Default()
	cfg.Environment = "staging-sensitive-value"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want unsupported environment rejection")
	}
	if strings.Contains(err.Error(), cfg.Environment) {
		t.Fatalf("Validate() leaked environment value: %v", err)
	}
}

func TestConfigValidateRejectsInvalidEnabledSMTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*SMTPConfig)
	}{
		{name: "missing host", mutate: func(c *SMTPConfig) { c.Host = "" }},
		{name: "zero port", mutate: func(c *SMTPConfig) { c.Port = 0 }},
		{name: "port exceeds maximum", mutate: func(c *SMTPConfig) { c.Port = 65536 }},
		{name: "unsupported TLS mode", mutate: func(c *SMTPConfig) { c.TLSMode = "plaintext-sensitive" }},
		{name: "invalid from email", mutate: func(c *SMTPConfig) { c.FromEmail = "not-an-email-sensitive" }},
		{name: "username without password", mutate: func(c *SMTPConfig) { c.Password = "" }},
		{name: "password without username", mutate: func(c *SMTPConfig) { c.Username = "" }},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			cfg.Authentication.SMTP = SMTPConfig{
				Enabled:   true,
				Host:      "smtp.sensitive.example",
				Port:      465,
				TLSMode:   "tls",
				Username:  "smtp-user-sensitive",
				Password:  "smtp-password-sensitive",
				FromEmail: "sender-sensitive@example.test",
			}
			test.mutate(&cfg.Authentication.SMTP)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want invalid enabled SMTP rejection")
			}
			for _, secret := range []string{
				"smtp.sensitive.example",
				"plaintext-sensitive",
				"not-an-email-sensitive",
				"smtp-user-sensitive",
				"smtp-password-sensitive",
				"sender-sensitive@example.test",
			} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("Validate() leaked SMTP value: %v", err)
				}
			}
		})
	}
}

func TestConfigValidateAcceptsCompleteEnabledSMTP(t *testing.T) {
	t.Parallel()

	for _, tlsMode := range []string{"tls", "starttls"} {
		tlsMode := tlsMode
		t.Run(tlsMode, func(t *testing.T) {
			t.Parallel()
			cfg := Default()
			cfg.Authentication.SMTP = SMTPConfig{
				Enabled:   true,
				Host:      "smtp.example.test",
				Port:      465,
				TLSMode:   tlsMode,
				Username:  "sender@example.test",
				Password:  "test-only-password",
				FromEmail: "sender@example.test",
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() complete SMTP error = %v", err)
			}
		})
	}
}

func TestValidateAcceptsWorkerWithoutListeningAddress(t *testing.T) {
	t.Parallel()

	cfg := Config{Environment: "development", Role: "worker", ShutdownTimeout: time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRuntimeAllowsDatabaseOnlyCommandWithoutMinIOConfiguration(t *testing.T) {
	t.Parallel()

	config := Default()
	config.Role = "worker"
	config.DatabaseURL = "postgres://fixture"
	config.MinIO = MinIOConfig{}

	if err := config.ValidateRuntime(); err != nil {
		t.Fatalf("ValidateRuntime() error = %v, want database-only command configuration to pass", err)
	}
}

func TestMinIOConfigValidateRuntimeRequiresCompleteConfigurationWithoutLeakingSecret(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*MinIOConfig)
	}{
		{name: "endpoint", mutate: func(config *MinIOConfig) { config.Endpoint = "" }},
		{name: "bucket", mutate: func(config *MinIOConfig) { config.Bucket = "" }},
		{name: "access key", mutate: func(config *MinIOConfig) { config.AccessKey = "" }},
		{name: "secret key", mutate: func(config *MinIOConfig) { config.SecretKey = "" }},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := MinIOConfig{
				Endpoint:  "127.0.0.1:9000",
				Bucket:    "fixture-bucket",
				AccessKey: "fixture-access",
				SecretKey: "fixture-secret-must-not-appear",
			}
			test.mutate(&config)

			err := config.ValidateRuntime()
			if err == nil {
				t.Fatal("MinIOConfig.ValidateRuntime() error = nil, want incomplete MinIO rejection")
			}
			if strings.Contains(err.Error(), "fixture-secret-must-not-appear") {
				t.Fatalf("MinIOConfig.ValidateRuntime() leaked secret: %v", err)
			}
		})
	}
}

func TestLoadUsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("HOTKEY_ROLE", "worker")
	t.Setenv("HOTKEY_HTTP_ADDR", "")
	t.Setenv("HOTKEY_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("HOTKEY_SOURCE_DOH_URL", "https://cloudflare-dns.com/dns-query")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Role != "worker" {
		t.Errorf("Role = %q, want worker", cfg.Role)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 3s", cfg.ShutdownTimeout)
	}
	if cfg.SourceDNSOverHTTPSURL != "https://cloudflare-dns.com/dns-query" {
		t.Errorf("SourceDNSOverHTTPSURL = %q, want configured DoH endpoint", cfg.SourceDNSOverHTTPSURL)
	}
}

func TestValidateAuthenticationRuntimeRejectsUnsafeConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing JWT secret", mutate: func(c *Config) { c.Authentication.JWTSecret = "" }},
		{name: "short JWT secret", mutate: func(c *Config) { c.Authentication.JWTSecret = "short" }},
		{name: "production insecure cookie", mutate: func(c *Config) {
			c.Environment = "production"
			c.Authentication.RefreshCookieSecure = false
		}},
		{name: "production wildcard CORS", mutate: func(c *Config) {
			c.Environment = "production"
			c.Authentication.RefreshCookieSecure = true
			c.Authentication.AllowedOrigins = []string{"*"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAuthenticationConfig()
			tt.mutate(&cfg)
			if err := cfg.ValidateAuthenticationRuntime(); err == nil {
				t.Fatal("ValidateAuthenticationRuntime() error = nil, want an error")
			}
		})
	}
}

func TestValidateAuthenticationRuntimeRequiresVerificationHMACSecret(t *testing.T) {
	t.Parallel()

	for _, secret := range []string{"", "too-short"} {
		cfg := validAuthenticationConfig()
		cfg.Authentication.VerificationHMACSecret = secret
		if err := cfg.ValidateAuthenticationRuntime(); err == nil {
			t.Fatalf("ValidateAuthenticationRuntime() with HMAC secret %q error = nil, want rejection", secret)
		}
	}
}

func TestValidateAuthenticationRuntimeAcceptsExplicitSafeConfiguration(t *testing.T) {
	t.Parallel()

	cfg := validAuthenticationConfig()
	if err := cfg.ValidateAuthenticationRuntime(); err != nil {
		t.Fatalf("ValidateAuthenticationRuntime() error = %v", err)
	}
}

func TestValidateAuthenticationRuntimeRejectsKnownPlaceholderSecrets(t *testing.T) {
	t.Parallel()

	placeholders := []string{
		"change-me-with-at-least-32-characters",
		"replace-with-your-random-secret-at-least-32-bytes",
		"replace-with-another-random-secret-32-bytes",
	}
	for _, placeholder := range placeholders {
		placeholder := placeholder
		for _, field := range []string{"JWT", "HMAC"} {
			field := field
			t.Run(field, func(t *testing.T) {
				t.Parallel()
				cfg := validAuthenticationConfig()
				if field == "JWT" {
					cfg.Authentication.JWTSecret = placeholder
				} else {
					cfg.Authentication.VerificationHMACSecret = placeholder
				}

				err := cfg.ValidateAuthenticationRuntime()
				if err == nil {
					t.Fatal("ValidateAuthenticationRuntime() error = nil, want known placeholder rejection")
				}
				if strings.Contains(err.Error(), placeholder) {
					t.Fatalf("ValidateAuthenticationRuntime() leaked secret: %v", err)
				}
			})
		}
	}

	example, err := os.ReadFile(filepath.Join("..", "..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	content := string(example)
	for _, line := range []string{"HOTKEY_JWT_SECRET=\n", "HOTKEY_VERIFICATION_HMAC_SECRET=\n"} {
		if !strings.Contains(content, line) {
			t.Errorf(".env.example must leave authentication secret blank: %q", strings.TrimSpace(line))
		}
	}
	for _, placeholder := range placeholders {
		if strings.Contains(content, placeholder) {
			t.Errorf(".env.example contains a known authentication placeholder")
		}
	}
}

func TestValidateAuthenticationRuntimeRejectsInvalidCORSOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		origin string
	}{
		{name: "relative URL", origin: "/relative-sensitive"},
		{name: "unsupported scheme", origin: "ftp://cors-sensitive.example"},
		{name: "userinfo", origin: "https://user:pass@cors-sensitive.example"},
		{name: "path", origin: "https://cors-sensitive.example/path"},
		{name: "root path", origin: "https://cors-sensitive.example/"},
		{name: "query", origin: "https://cors-sensitive.example?query=sensitive"},
		{name: "fragment", origin: "https://cors-sensitive.example#sensitive"},
		{name: "empty fragment", origin: "https://cors-sensitive.example#"},
		{name: "missing host", origin: "https:///missing-sensitive"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := validAuthenticationConfig()
			cfg.Authentication.AllowedOrigins = []string{test.origin}
			err := cfg.ValidateAuthenticationRuntime()
			if err == nil {
				t.Fatal("ValidateAuthenticationRuntime() error = nil, want invalid CORS origin rejection")
			}
			if strings.Contains(err.Error(), test.origin) {
				t.Fatalf("ValidateAuthenticationRuntime() leaked CORS origin: %v", err)
			}
		})
	}
}

func TestLoadReadsNamedAuthenticationSettings(t *testing.T) {
	t.Setenv("HOTKEY_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("HOTKEY_JWT_ISSUER", "hotkey-test")
	t.Setenv("HOTKEY_JWT_AUDIENCE", "hotkey-web")
	t.Setenv("HOTKEY_VERIFICATION_HMAC_SECRET", "verification-hmac-secret-for-tests-32-bytes")
	t.Setenv("HOTKEY_REDIS_URL", "redis://127.0.0.1:6379/15")
	t.Setenv("HOTKEY_SMTP_HOST", "smtp.163.com")
	t.Setenv("HOTKEY_SMTP_PORT", "465")
	t.Setenv("HOTKEY_SMTP_TLS_MODE", "tls")
	t.Setenv("HOTKEY_SMTP_USERNAME", "sender@163.com")
	t.Setenv("HOTKEY_SMTP_PASSWORD", "app-password")
	t.Setenv("HOTKEY_SMTP_FROM_EMAIL", "sender@163.com")
	t.Setenv("HOTKEY_SMTP_FROM_NAME", "HotKey")
	t.Setenv("HOTKEY_CORS_ALLOWED_ORIGINS", "https://app.example.test,https://admin.example.test")
	t.Setenv("HOTKEY_REFRESH_COOKIE_SECURE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Authentication.JWTIssuer != "hotkey-test" || cfg.Authentication.VerificationHMACSecret != "verification-hmac-secret-for-tests-32-bytes" || cfg.Authentication.SMTP.Host != "smtp.163.com" {
		t.Errorf("Load() authentication = %#v, want named values", cfg.Authentication)
	}
	if got, want := strings.Join(cfg.Authentication.AllowedOrigins, ","), "https://app.example.test,https://admin.example.test"; got != want {
		t.Errorf("AllowedOrigins = %q, want %q", got, want)
	}
}

func TestAIConfigUsesOnlyExplicitProviderAndArtifactKeys(t *testing.T) {
	t.Setenv("HOTKEY_OPENAI_API_KEY", "test-openai-key")
	t.Setenv("HOTKEY_DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("HOTKEY_OLLAMA_ENABLED", "true")
	t.Setenv("HOTKEY_OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	t.Setenv("HOTKEY_ONNX_RUNTIME_LIBRARY", "/fixtures/libonnxruntime.dylib")
	t.Setenv("HOTKEY_ONNX_MODEL_PATH", "/fixtures/model.onnx")
	t.Setenv("HOTKEY_ONNX_TOKENIZER_PATH", "/fixtures/tokenizer.json")
	t.Setenv("HOTKEY_ONNX_MANIFEST_PATH", "/fixtures/manifest.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.AI.OpenAIAPIKey, "test-openai-key"; got != want {
		t.Errorf("AI.OpenAIAPIKey = %q, want %q", got, want)
	}
	if cfg.AI.DeepSeekAPIKey != "test-deepseek-key" || !cfg.AI.OllamaEnabled || cfg.AI.OllamaBaseURL != "http://127.0.0.1:11434" {
		t.Errorf("Load() PLAN-018 AI config = %#v", cfg.AI)
	}
	if got, want := cfg.AI.ONNXManifestPath, "/fixtures/manifest.json"; got != want {
		t.Errorf("AI.ONNXManifestPath = %q, want %q", got, want)
	}

	keys := strings.Join(configKeys(), ",")
	for _, key := range []string{
		"openai_api_key",
		"deepseek_api_key",
		"ollama_enabled",
		"ollama_base_url",
		"onnx_runtime_library",
		"onnx_model_path",
		"onnx_tokenizer_path",
		"onnx_manifest_path",
	} {
		if !strings.Contains(keys, key) {
			t.Errorf("configKeys() does not bind %q", key)
		}
	}
	for _, key := range []string{"llm_api_key", "llm_base_url", "llm_model"} {
		if strings.Contains(keys, key) {
			t.Errorf("configKeys() must not bind legacy generic %q", key)
		}
	}

	example, err := os.ReadFile(filepath.Join("..", "..", "..", ".env.example"))
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	for _, key := range []string{
		"HOTKEY_OPENAI_API_KEY=",
		"HOTKEY_DEEPSEEK_API_KEY=",
		"HOTKEY_OLLAMA_ENABLED=false",
		"HOTKEY_OLLAMA_BASE_URL=http://127.0.0.1:11434",
		"HOTKEY_ONNX_RUNTIME_LIBRARY=",
		"HOTKEY_ONNX_MODEL_PATH=",
		"HOTKEY_ONNX_TOKENIZER_PATH=",
		"HOTKEY_ONNX_MANIFEST_PATH=",
	} {
		if !strings.Contains(string(example), key) {
			t.Errorf(".env.example does not document %q", key)
		}
	}
	for _, key := range []string{"HOTKEY_LLM_API_KEY=", "HOTKEY_LLM_BASE_URL=", "HOTKEY_LLM_MODEL="} {
		if strings.Contains(string(example), key) {
			t.Errorf(".env.example retains forbidden generic %q", key)
		}
	}
}

func TestLoadUsesDefaultEnvironmentFileAndProcessOverrides(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(strings.Join([]string{
		"HOTKEY_ROLE=worker",
		"HOTKEY_JWT_SECRET=default-development-secret-with-more-than-32-bytes",
		"HOTKEY_OPENAI_API_KEY=default-openai-key",
		"HOTKEY_ONNX_RUNTIME_LIBRARY=/default/libonnxruntime.dylib",
		"HOTKEY_ONNX_MODEL_PATH=/default/model.onnx",
		"HOTKEY_ONNX_TOKENIZER_PATH=/default/tokenizer.json",
		"HOTKEY_ONNX_MANIFEST_PATH=/default/manifest.json",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(directory)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() default environment: %v", err)
	}
	if cfg.Role != "worker" || cfg.Authentication.JWTSecret != "default-development-secret-with-more-than-32-bytes" ||
		cfg.AI.OpenAIAPIKey != "default-openai-key" || cfg.AI.ONNXManifestPath != "/default/manifest.json" {
		t.Fatalf("Load() default environment = %#v", cfg)
	}

	t.Setenv("HOTKEY_ROLE", "api")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() process override: %v", err)
	}
	if cfg.Role != "api" {
		t.Fatalf("Load() process role = %q, want api", cfg.Role)
	}
}

func TestLoadUsesProductionEnvironmentFile(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(strings.Join([]string{
		"HOTKEY_ENV=production",
		"HOTKEY_ROLE=worker",
		"HOTKEY_JWT_SECRET=default-development-secret-with-more-than-32-bytes",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".env.prod"), []byte(strings.Join([]string{
		"HOTKEY_JWT_SECRET=production-secret-with-more-than-32-bytes",
		"HOTKEY_VERIFICATION_HMAC_SECRET=production-hmac-secret-with-more-than-32-bytes",
		"HOTKEY_CORS_ALLOWED_ORIGINS=https://app.example.test",
		"HOTKEY_REFRESH_COOKIE_SECURE=true",
		"HOTKEY_OPENAI_API_KEY=production-openai-key",
		"HOTKEY_ONNX_RUNTIME_LIBRARY=/production/libonnxruntime.dylib",
		"HOTKEY_ONNX_MODEL_PATH=/production/model.onnx",
		"HOTKEY_ONNX_TOKENIZER_PATH=/production/tokenizer.json",
		"HOTKEY_ONNX_MANIFEST_PATH=/production/manifest.json",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(directory)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() production environment: %v", err)
	}
	if cfg.Environment != "production" || cfg.Authentication.JWTSecret != "production-secret-with-more-than-32-bytes" ||
		!cfg.Authentication.RefreshCookieSecure || cfg.AI.OpenAIAPIKey != "production-openai-key" ||
		cfg.AI.ONNXRuntimeLibrary != "/production/libonnxruntime.dylib" || cfg.AI.ONNXModelPath != "/production/model.onnx" ||
		cfg.AI.ONNXTokenizerPath != "/production/tokenizer.json" || cfg.AI.ONNXManifestPath != "/production/manifest.json" {
		t.Fatalf("Load() production environment = %#v", cfg)
	}
}

func validAuthenticationConfig() Config {
	cfg := Default()
	cfg.Authentication = AuthenticationConfig{
		JWTSecret:              "0123456789abcdef0123456789abcdef",
		JWTIssuer:              "hotkey",
		JWTAudience:            "hotkey-web",
		VerificationHMACSecret: "verification-hmac-secret-for-tests-32-bytes",
		AllowedOrigins:         []string{"http://localhost:3000"},
		RefreshCookieSecure:    false,
	}
	return cfg
}
