package config

// KafkaConfig groups Kafka messaging related configuration fields.
type KafkaConfig struct {
	KafkaBrokers       []string `mapstructure:"KAFKA_BROKERS"`
	KafkaConsumerGroup string   `mapstructure:"KAFKA_CONSUMER_GROUP"`
}
