package config

// LLMConfig groups LLM provider and embedding related configuration fields.
type LLMConfig struct {
	LLMProvider    string  `mapstructure:"LLM_PROVIDER"`
	LLMAPIKey      string  `mapstructure:"LLM_API_KEY"`
	LLMBaseURL     string  `mapstructure:"LLM_BASE_URL"`
	LLMModel       string  `mapstructure:"LLM_MODEL"`
	LLMMaxTokens   int     `mapstructure:"LLM_MAX_TOKENS"`
	LLMTemperature float64 `mapstructure:"LLM_TEMPERATURE"`

	EmbeddingModelPath string `mapstructure:"EMBEDDING_MODEL_PATH"`
}
