package config

// ObsidianConfig groups Obsidian daily digest related configuration fields.
type ObsidianConfig struct {
	ObsidianVaultPath   string `mapstructure:"OBSIDIAN_VAULT_PATH"`
	DailyDigestTime     string `mapstructure:"DAILY_DIGEST_TIME"`
	DailyDigestTimezone string `mapstructure:"DAILY_DIGEST_TIMEZONE"`
	DailyDigestTarget   string `mapstructure:"DAILY_DIGEST_TARGET"`
	DailyDigestTopN     int    `mapstructure:"DAILY_DIGEST_TOP_N"`
}
