package config


// Config is the root configuration for a kafka-broadcaster deployment.
type Config struct {
	Kafka          KafkaConfig          `yaml:"kafka"`
	Routing        RoutingConfig        `yaml:"routing"`
	Transformation TransformationConfig `yaml:"transformation"`
	Enrichment     []EnrichmentConfig   `yaml:"enrichment"`
	Metrics        MetricsConfig        `yaml:"metrics"`
}

// KafkaConfig holds all Kafka broker and topic settings.
type KafkaConfig struct {
	Brokers       []string `yaml:"brokers"`
	SourceTopic   string   `yaml:"source_topic"`
	ConsumerGroup string   `yaml:"consumer_group"`
	DLQTopic      string   `yaml:"dlq_topic"`
}

// RoutingConfig defines how the target topic is resolved from a Kafka header.
type RoutingConfig struct {
	HeaderKey string `yaml:"header_key"`
}

// TransformationConfig holds the list of JSON path transformation rules.
type TransformationConfig struct {
	Rules []TransformationRule `yaml:"rules"`
}

// TransformationRule maps a JSON field from one path to another.
type TransformationRule struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// EnrichmentConfig defines a single enrichment step and its parameters.
type EnrichmentConfig struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// MetricsConfig holds settings for the Prometheus metrics HTTP server.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}