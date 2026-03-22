package producer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer wraps a franz-go Kafka client for producing records.
type Producer struct {
	client *kgo.Client
	logger *slog.Logger
}

// New creates a new Producer configured from cfg.
func New(cfg *config.KafkaConfig, logger *slog.Logger) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating kafka producer: %w", err)
	}
	return &Producer{client: client, logger: logger}, nil
}

// Produce sends a record to the given topic synchronously.
// The record's topic is overridden by the provided topic argument.
func (p *Producer) Produce(ctx context.Context, topic string, record *kgo.Record) error {
	out := cloneRecord(record)
	out.Topic = topic

	results := p.client.ProduceSync(ctx, out)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("producing to topic %q: %w", topic, err)
	}
	return nil
}

// cloneRecord returns a shallow copy of r with the same headers slice reference.
func cloneRecord(r *kgo.Record) *kgo.Record {
	cp := *r
	return &cp
}

// Close flushes any buffered records and shuts down the underlying Kafka client.
func (p *Producer) Close() {
	if err := p.client.Flush(context.Background()); err != nil {
		p.logger.Error("flushing producer on close", "error", err)
	}
	p.client.Close()
}
