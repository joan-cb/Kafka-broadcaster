package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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

// Load reads the YAML config file at the given path and returns a validated Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	cfg.applyDefaults()

	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("kafka.brokers must not be empty")
	}
	if c.Kafka.SourceTopic == "" {
		return fmt.Errorf("kafka.source_topic must not be empty")
	}
	if c.Kafka.ConsumerGroup == "" {
		return fmt.Errorf("kafka.consumer_group must not be empty")
	}
	if c.Routing.HeaderKey == "" {
		return fmt.Errorf("routing.header_key must not be empty")
	}
	for i, rule := range c.Transformation.Rules {
		if rule.From == "" || rule.To == "" {
			return fmt.Errorf("transformation.rules[%d]: from and to must not be empty", i)
		}
	}
	for i, enc := range c.Enrichment {
		if enc.Type == "" {
			return fmt.Errorf("enrichment[%d]: type must not be empty", i)
		}
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.Kafka.DLQTopic == "" {
		c.Kafka.DLQTopic = c.Kafka.SourceTopic + ".dlq"
	}
	if c.Metrics.Port == 0 {
		c.Metrics.Port = 9090
	}
	if c.Metrics.Path == "" {
		c.Metrics.Path = "/metrics"
	}
}
