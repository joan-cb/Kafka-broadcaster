package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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
	if err := c.Routing.ContentRouting.validate(); err != nil {
		return err
	}
	return nil
}

func (c ContentRoutingConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.KeyPath == "" {
		return fmt.Errorf("routing.content_routing.key_path must not be empty when content_routing.enabled is true")
	}
	if err := c.ValueType.Validate(); err != nil {
		return fmt.Errorf("routing.content_routing: %w", err)
	}
	// Reuse same rules as contentrouter.New for routes (keep messages aligned).
	if len(c.Routes) == 0 {
		return fmt.Errorf("routing.content_routing.routes must not be empty when enabled")
	}
	for k, topics := range c.Routes {
		if k == "" {
			return fmt.Errorf("routing.content_routing.routes: keys must not be empty")
		}
		if len(topics) == 0 {
			return fmt.Errorf("routing.content_routing.routes[%q] must list at least one topic", k)
		}
		for i, t := range topics {
			if t == "" {
				return fmt.Errorf("routing.content_routing.routes[%q][%d] must not be empty", k, i)
			}
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
