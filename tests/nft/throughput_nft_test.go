//go:build nft

// Package nft contains non-functional tests that measure throughput and latency
// of the kafka-broadcaster pipeline. Run with: go test -tags=nft -bench=. ./tests/nft/
package nft_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/pipeline"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/twmb/franz-go/pkg/kgo"
)

// noopProducer discards all messages.
type noopProducer struct{}

func (n *noopProducer) Produce(_ context.Context, _ string, _ *kgo.Record) error { return nil }

// noopDLQ discards all DLQ messages.
type noopDLQ struct{}

func (n *noopDLQ) Send(_ context.Context, _ *kgo.Record, _ string, _ error) {}

func benchmarkPayload() []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"transaction_id": "TX-BENCH-001",
		"user_id":        "U-42",
		"amount":         199.99,
		"currency":       "GBP",
		"metadata": map[string]interface{}{
			"source": "checkout",
			"region": "EMEA",
		},
	})
	return b
}

func newNFTPipeline(t interface {
	Helper()
	Fatal(...interface{})
}) *pipeline.Pipeline {
	t.Helper()
	enc, _ := enricher.NewChain([]config.EnrichmentConfig{
		{
			Type: "uuid_from_transaction",
			Config: map[string]interface{}{
				"input_field":  "$.txId",
				"output_field": "$.event_id",
				"algorithm":    "sha256",
			},
		},
	})
	return pipeline.New(pipeline.Config{
		Router: router.New("target-topic"),
		Transformer: transformer.New([]config.TransformationRule{
			{From: "$.transaction_id", To: "$.txId"},
		}),
		Enricher:    enc,
		Producer:    &noopProducer{},
		DLQ:         &noopDLQ{},
		SourceTopic: "bench.source",
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

func newBenchPipeline(b *testing.B) *pipeline.Pipeline {
	b.Helper()
	// Enricher reads $.txId which is produced by the transformer (transaction_id → txId).
	enc, _ := enricher.NewChain([]config.EnrichmentConfig{
		{
			Type: "uuid_from_transaction",
			Config: map[string]interface{}{
				"input_field":  "$.txId",
				"output_field": "$.event_id",
				"algorithm":    "sha256",
			},
		},
	})
	return pipeline.New(pipeline.Config{
		Router: router.New("target-topic"),
		Transformer: transformer.New([]config.TransformationRule{
			{From: "$.transaction_id", To: "$.txId"},
		}),
		Enricher:    enc,
		Producer:    &noopProducer{},
		DLQ:         &noopDLQ{},
		SourceTopic: "bench.source",
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

func BenchmarkPipeline_SingleMessage(b *testing.B) {
	p := newBenchPipeline(b)
	payload := benchmarkPayload()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch := make(chan *kgo.Record, 1)
		ch <- &kgo.Record{
			Topic: "bench.source",
			Value: payload,
			Headers: []kgo.RecordHeader{
				{Key: "target-topic", Value: []byte("bench.target")},
			},
		}
		close(ch)
		p.Run(context.Background(), ch)
	}
}

func BenchmarkPipeline_100Messages(b *testing.B) {
	p := newBenchPipeline(b)
	payload := benchmarkPayload()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch := make(chan *kgo.Record, 100)
		for j := 0; j < 100; j++ {
			ch <- &kgo.Record{
				Topic: "bench.source",
				Value: payload,
				Headers: []kgo.RecordHeader{
					{Key: "target-topic", Value: []byte("bench.target")},
				},
			}
		}
		close(ch)
		p.Run(context.Background(), ch)
	}
}

func TestNFT_PipelineThroughput_MinThreshold(t *testing.T) {
	p := newNFTPipeline(t)
	payload := benchmarkPayload()

	const msgCount = 10_000
	ch := make(chan *kgo.Record, msgCount)
	for i := 0; i < msgCount; i++ {
		ch <- &kgo.Record{
			Topic: "bench.source",
			Value: payload,
			Headers: []kgo.RecordHeader{
				{Key: "target-topic", Value: []byte("bench.target")},
			},
		}
	}
	close(ch)

	start := timeNow()
	p.Run(context.Background(), ch)
	elapsed := timeSince(start)

	msgsPerSec := float64(msgCount) / elapsed.Seconds()
	t.Logf("throughput: %.0f messages/sec over %d messages", msgsPerSec, msgCount)

	const minThroughput = 5_000
	if msgsPerSec < minThroughput {
		t.Errorf("throughput %.0f msg/sec is below minimum threshold of %d msg/sec", msgsPerSec, minThroughput)
	}
}
