package config

import (
	"os"
	"testing"
)

func TestLoad_DailyDigestConfigDefaults(t *testing.T) {
	// Ensure required fields are present so Load() succeeds.
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("X_BEARER_TOKEN", "test-token")

	// Clear all digest-related env vars to test defaults.
	for _, k := range []string{
		"OBSIDIAN_VAULT_PATH",
		"DAILY_DIGEST_TIME",
		"DAILY_DIGEST_TIMEZONE",
		"DAILY_DIGEST_TARGET",
		"DAILY_DIGEST_TOP_N",
		"LLM_PROVIDER",
		"LLM_API_KEY",
		"LLM_BASE_URL",
		"LLM_MODEL",
	} {
		os.Unsetenv(k)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// ObsidianVaultPath has no default; should be empty when unset.
	if cfg.ObsidianVaultPath != "" {
		t.Errorf("ObsidianVaultPath = %q, want empty", cfg.ObsidianVaultPath)
	}

	if cfg.DailyDigestTime != "08:00" {
		t.Errorf("DailyDigestTime = %q, want %q", cfg.DailyDigestTime, "08:00")
	}

	if cfg.DailyDigestTimezone != "Asia/Shanghai" {
		t.Errorf("DailyDigestTimezone = %q, want %q", cfg.DailyDigestTimezone, "Asia/Shanghai")
	}

	if cfg.DailyDigestTarget != "yesterday" {
		t.Errorf("DailyDigestTarget = %q, want %q", cfg.DailyDigestTarget, "yesterday")
	}

	if cfg.DailyDigestTopN != 20 {
		t.Errorf("DailyDigestTopN = %d, want %d", cfg.DailyDigestTopN, 20)
	}

	if cfg.LLMProvider != "openai" {
		t.Errorf("LLMProvider = %q, want %q", cfg.LLMProvider, "openai")
	}

	if cfg.LLMAPIKey != "" {
		t.Errorf("LLMAPIKey = %q, want empty", cfg.LLMAPIKey)
	}

	if cfg.LLMBaseURL != "https://api.openai.com/v1" {
		t.Errorf("LLMBaseURL = %q, want %q", cfg.LLMBaseURL, "https://api.openai.com/v1")
	}

	if cfg.LLMModel != "gpt-4o-mini" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "gpt-4o-mini")
	}
}
