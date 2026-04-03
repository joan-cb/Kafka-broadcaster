package config

import "fmt"

// ContentValueType is the strict JSON type expected at key_path for content routing.
type ContentValueType string

const (
	ContentValueString ContentValueType = "string"
	ContentValueBool   ContentValueType = "bool"
	ContentValueInt    ContentValueType = "int"
)

// Validate returns an error if t is not a supported content routing value type.
func (t ContentValueType) Validate() error {
	switch t {
	case ContentValueString, ContentValueBool, ContentValueInt:
		return nil
	default:
		return fmt.Errorf("value_type must be string, bool, or int")
	}
}

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

// RoutingConfig defines how the target topic is resolved from headers and,
// optionally, from a field in the transformed JSON payload.
type RoutingConfig struct {
	HeaderKey      string               `yaml:"header_key"`
	ContentRouting ContentRoutingConfig `yaml:"content_routing"`
}

// ContentRoutingConfig maps a single JSON field (after transformation, before
// enrichment) to one or more target topics. When enabled, lookup success
// selects targets and the header is ignored for topic choice; on lookup
// failure the message goes to the DLQ with stage content_routing.
type ContentRoutingConfig struct {
	Enabled   bool                `yaml:"enabled"`
	KeyPath   string              `yaml:"key_path"`
	ValueType ContentValueType    `yaml:"value_type"`
	Routes    map[string][]string `yaml:"routes"`
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
