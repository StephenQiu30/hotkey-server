package constant

// Default configuration values used when env vars are not set.
const (
	DefaultHTTPAddr             = ":8080"
	DefaultLLMProvider          = "openai"
	DefaultLLMBaseURL           = "https://api.openai.com/v1"
	DefaultLLMModel             = "gpt-4o-mini"
	DefaultLLMMaxTokens         = 4096
	DefaultLLMTemperature       = 0.7
	DefaultDailyDigestTime      = "08:00"
	DefaultDailyDigestTimezone  = "Asia/Shanghai"
	DefaultDailyDigestTarget    = "yesterday"
	DefaultDailyDigestTopN      = 20
	DefaultRedisAddr            = "localhost:6379"
	DefaultKafkaConsumerGroup   = "hotkey-workers"
	DefaultLogLevel             = "info"
	DefaultLogFormat            = "json"
	DefaultLogOutput            = "stdout"
	DefaultXBaseURL             = "https://api.x.com"
)
