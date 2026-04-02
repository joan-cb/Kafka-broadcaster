//go:build e2e

// Package e2e contains end-to-end tests that start a real Kafka broker and the
// kafka-broadcaster binary, asserting message flow over the wire.
package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"gopkg.in/yaml.v3"
)

func startKafka(ctx context.Context, t *testing.T) string {
	t.Helper()
	kc, err := kafka.Run(ctx, "confluentinc/cp-kafka:7.4.0",
		kafka.WithClusterID("e2e-cluster"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tc.TerminateContainer(kc) })
	broker, err := kc.Brokers(ctx)
	require.NoError(t, err)
	return broker[0]
}

func writeTempConfig(t *testing.T, broker, sourceTopic, consumerGroup, dlqTopic string) string {
	t.Helper()
	cfg := map[string]interface{}{
		"kafka": map[string]interface{}{
			"brokers":        []string{broker},
			"source_topic":   sourceTopic,
			"consumer_group": consumerGroup,
			"dlq_topic":      dlqTopic,
		},
		"routing": map[string]interface{}{
			"header_key": "target-topic",
		},
		"transformation": map[string]interface{}{
			"rules": []map[string]interface{}{
				{"from": "$.raw_id", "to": "$.id"},
			},
		},
		"enrichment": []map[string]interface{}{
			{
				"type": "uuid_from_transaction",
				"config": map[string]interface{}{
					"input_field":  "$.id",
					"output_field": "$.event_hash",
					"algorithm":    "sha256",
				},
			},
		},
		"metrics": map[string]interface{}{
			"enabled": false,
		},
	}

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	f, err := os.CreateTemp(t.TempDir(), "e2e-config-*.yaml")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// moduleRoot returns the directory containing go.mod. Tests run with cwd set to
// the package dir (tests/e2e), so relative paths like ./cmd/broadcaster must be
// built from the module root.
func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found from %s", wd)
		dir = parent
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "broadcaster")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/broadcaster")
	cmd.Dir = moduleRoot(t)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to build broadcaster binary")
	return bin
}

func TestE2E_FullMessageLifecycle(t *testing.T) {
	skipE2EIfNoDocker(t)
	tc.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	broker := startKafka(ctx, t)

	const (
		sourceTopic = "e2e.source"
		targetTopic = "e2e.target"
		dlqTopic    = "e2e.dlq"
		consGroup   = "e2e-group"
	)

	cfgPath := writeTempConfig(t, broker, sourceTopic, consGroup, dlqTopic)
	binary := buildBinary(t)

	// Start broadcaster binary.
	broadcasterCtx, cancelBroadcaster := context.WithCancel(ctx)
	defer cancelBroadcaster()

	cmd := exec.CommandContext(broadcasterCtx, binary)
	cmd.Env = append(os.Environ(), "CONFIG_PATH="+cfgPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancelBroadcaster()
		_ = cmd.Wait()
	})

	// Give broadcaster time to connect.
	time.Sleep(3 * time.Second)

	// Produce a test message to the source topic.
	seedClient, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.AllowAutoTopicCreation(),
	)
	require.NoError(t, err)
	defer seedClient.Close()

	payload, _ := json.Marshal(map[string]interface{}{"raw_id": "E2E-001", "value": "test"})
	results := seedClient.ProduceSync(ctx, &kgo.Record{
		Topic: sourceTopic,
		Value: payload,
		Headers: []kgo.RecordHeader{
			{Key: "target-topic", Value: []byte(targetTopic)},
		},
	})
	require.NoError(t, results.FirstErr())

	// Consume from target topic and assert the message was transformed and enriched.
	verifyCtx, cancelVerify := context.WithTimeout(ctx, 30*time.Second)
	defer cancelVerify()

	targetClient, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.ConsumeTopics(targetTopic),
		kgo.ConsumerGroup(fmt.Sprintf("e2e-verifier-%d", time.Now().UnixNano())),
		kgo.AllowAutoTopicCreation(),
	)
	require.NoError(t, err)
	defer targetClient.Close()

	var received []byte
	for received == nil {
		select {
		case <-verifyCtx.Done():
			t.Fatal("timed out waiting for message on target topic")
		default:
		}
		fetches := targetClient.PollFetches(verifyCtx)
		fetches.EachRecord(func(r *kgo.Record) {
			received = r.Value
		})
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(received, &result))

	assert.Equal(t, "E2E-001", result["id"], "transformation: raw_id → id")
	assert.NotEmpty(t, result["event_hash"], "enrichment: event_hash must be populated")
	assert.Nil(t, result["raw_id"], "original raw_id must be removed")
}
