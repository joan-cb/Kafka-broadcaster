package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Consumer wraps a franz-go Kafka client and exposes consumed records via a channel.
type Consumer struct {
	client *kgo.Client
	logger *slog.Logger
}

// New creates a new Consumer configured from cfg.
func New(cfg *config.KafkaConfig, logger *slog.Logger) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.SourceTopic),
	)
	if err != nil {
		return nil, fmt.Errorf("creating kafka consumer: %w", err)
	}
	return &Consumer{client: client, logger: logger}, nil
}

// Run starts consuming records and sends them to the returned channel.
// The channel is closed when ctx is cancelled or a fatal error occurs.
func (c *Consumer) Run(ctx context.Context) <-chan *kgo.Record {
	out := make(chan *kgo.Record, 256)

	go func() {
		defer close(out)
		for {
			fetches := c.client.PollFetches(ctx)
			if fetches.IsClientClosed() {
				return
			}
			// After ctx is cancelled, PollFetches returns immediately with per-partition
			// errors; exit instead of spinning and logging them thousands of times.
			if ctx.Err() != nil {
				return
			}
			fetches.EachError(func(topic string, partition int32, err error) {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				c.logger.Error("fetch error", "topic", topic, "partition", partition, "error", err)
			})
			fetches.EachRecord(func(record *kgo.Record) {
				select {
				case out <- record:
				case <-ctx.Done():
					return
				}
			})
		}
	}()

	return out
}

// CommitRecords marks the given records as processed on the broker.
func (c *Consumer) CommitRecords(ctx context.Context, records ...*kgo.Record) error {
	if err := c.client.CommitRecords(ctx, records...); err != nil {
		return fmt.Errorf("committing records: %w", err)
	}
	return nil
}

// Close shuts down the underlying Kafka client.
func (c *Consumer) Close() {
	c.client.Close()
}
