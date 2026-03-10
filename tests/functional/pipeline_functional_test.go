//go:build functional

package functional_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/consumer"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/pipeline"
	"github.com/globalcommerce/kafka-broadcaster/internal/producer"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"log/slog"
	"os"
)

func startKafka(ctx context.Context, t *testing.T) string {
	t.Helper()
	kc, err := kafka.Run(ctx, "confluentinc/cp-kafka:7.4.0",
		kafka.WithClusterID("test-cluster"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tc.TerminateContainer(kc) })

	broker, err := kc.Brokers(ctx)
	require.NoError(t, err)
	return broker[0]
}

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestFunctional_FullPipeline(t *testing.T) {
	ctx := context.Background()
	broker := startKafka(ctx, t)

	const (
		sourceTopic = "functional.source"
		targetTopic = "functional.target"
		dlqTopic    = "functional.dlq"
		consGroup   = "functional-test-group"
	)

	kafkaCfg := &config.KafkaConfig{
		Brokers:       []string{broker},
		SourceTopic:   sourceTopic,
		ConsumerGroup: consGroup,
		DLQTopic:      dlqTopic,
	}
	l := logger()

	// Seed a message to the source topic before starting the pipeline consumer.
	seedClient, err := kgo.NewClient(kgo.SeedBrokers(broker))
	require.NoError(t, err)
	defer seedClient.Close()

	payload, _ := json.Marshal(map[string]interface{}{"txId": "TX-FUNC-001", "amount": 50})
	seedRecord := &kgo.Record{
		Topic: sourceTopic,
		Value: payload,
		Headers: []kgo.RecordHeader{
			{Key: "target-topic", Value: []byte(targetTopic)},
		},
	}
	results := seedClient.ProduceSync(ctx, seedRecord)
	require.NoError(t, results.FirstErr())

	// Build pipeline components.
	cons, err := consumer.New(kafkaCfg, l)
	require.NoError(t, err)
	defer cons.Close()

	prod, err := producer.New(kafkaCfg, l)
	require.NoError(t, err)
	defer prod.Close()

	enc, err := enricher.NewChain(nil)
	require.NoError(t, err)

	pipelineCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	p := pipeline.New(pipeline.Config{
		Router:      router.New("target-topic"),
		Transformer: transformer.New(nil),
		Enricher:    enc,
		Producer:    prod,
		SourceTopic: sourceTopic,
		Logger:      l,
	})

	records := cons.Run(pipelineCtx)

	// Run pipeline in background; cancel after first message is processed.
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(pipelineCtx, records)
	}()

	// Read from the target topic to verify delivery.
	targetClient, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.ConsumeTopics(targetTopic),
		kgo.ConsumerGroup(fmt.Sprintf("verifier-%d", time.Now().UnixNano())),
	)
	require.NoError(t, err)
	defer targetClient.Close()

	deadline := time.After(20 * time.Second)
	var received []byte
outer:
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for message on target topic")
		default:
			fetches := targetClient.PollFetches(pipelineCtx)
			fetches.EachRecord(func(r *kgo.Record) {
				received = r.Value
				cancel()
			})
			if received != nil {
				break outer
			}
		}
	}

	<-done

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(received, &result))
	assert.Equal(t, "TX-FUNC-001", result["txId"])
	assert.Equal(t, float64(50), result["amount"])
}
