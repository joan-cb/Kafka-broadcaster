package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "internal.events"
  consumer_group: "broadcaster-test"
  dlq_topic: "internal.events.dlq"
routing:
  header_key: "target-topic"
transformation:
  rules:
    - from: "$.txId"
      to: "$.transactionId"
enrichment:
  - type: "uuid_from_transaction"
    config:
      input_field: "$.txId"
      output_field: "$.eventId"
      algorithm: "sha256"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"broker:9092"}, cfg.Kafka.Brokers)
	assert.Equal(t, "internal.events", cfg.Kafka.SourceTopic)
	assert.Equal(t, "broadcaster-test", cfg.Kafka.ConsumerGroup)
	assert.Equal(t, "target-topic", cfg.Routing.HeaderKey)
	assert.Len(t, cfg.Transformation.Rules, 1)
	assert.Len(t, cfg.Enrichment, 1)
}

func TestLoad_DefaultDLQTopic(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "x-target"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, "events.dlq", cfg.Kafka.DLQTopic)
}

func TestLoad_DefaultMetrics(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "x-target"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Metrics.Port)
	assert.Equal(t, "/metrics", cfg.Metrics.Path)
}

func TestLoad_MissingBrokers(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "x-target"
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka.brokers")
}

func TestLoad_MissingSourceTopic(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  consumer_group: "grp"
routing:
  header_key: "x-target"
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka.source_topic")
}

func TestLoad_MissingConsumerGroup(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
routing:
  header_key: "x-target"
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka.consumer_group")
}

func TestLoad_MissingHeaderKey(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing: {}
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "routing.header_key")
}

func TestLoad_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `{not: valid: yaml`)
	_, err := config.Load(path)
	require.Error(t, err)
}

func TestLoad_ContentRoutingEnabled_Valid(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "target-topic"
  content_routing:
    enabled: true
    key_path: "$.event.type"
    value_type: string
    routes:
      order.created:
        - "topic.orders"
`)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.True(t, cfg.Routing.ContentRouting.Enabled)
	assert.Equal(t, "$.event.type", cfg.Routing.ContentRouting.KeyPath)
	assert.Equal(t, config.ContentValueString, cfg.Routing.ContentRouting.ValueType)
	assert.Equal(t, []string{"topic.orders"}, cfg.Routing.ContentRouting.Routes["order.created"])
}

func TestLoad_ContentRoutingEnabled_MissingKeyPath(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "target-topic"
  content_routing:
    enabled: true
    value_type: string
    routes:
      a:
        - "t"
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key_path")
}

func TestLoad_ContentRoutingEnabled_EmptyRoutes(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, `
kafka:
  brokers: ["broker:9092"]
  source_topic: "events"
  consumer_group: "grp"
routing:
  header_key: "target-topic"
  content_routing:
    enabled: true
    key_path: "$.k"
    value_type: string
    routes: {}
`)
	_, err := config.Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "routes")
}
