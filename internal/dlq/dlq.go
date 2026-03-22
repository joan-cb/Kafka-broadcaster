package dlq

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/globalcommerce/kafka-broadcaster/internal/metrics"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Sender routes failed messages to a dedicated Dead Letter Queue topic.
type Sender struct {
	client   *kgo.Client
	dlqTopic string
	metrics  *metrics.Metrics
	logger   *slog.Logger
}

// New creates a DLQ Sender backed by the provided Kafka brokers.
func New(brokers []string, dlqTopic string, m *metrics.Metrics, logger *slog.Logger) (*Sender, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating dlq producer: %w", err)
	}
	return &Sender{
		client:   client,
		dlqTopic: dlqTopic,
		metrics:  m,
		logger:   logger,
	}, nil
}

// Send logs the pipeline failure and produces the original record to the DLQ topic
// with an added "x-error-reason" header containing the error message.
func (s *Sender) Send(ctx context.Context, record *kgo.Record, stage string, reason error) {
	s.logger.Error("pipeline failure — sending to DLQ",
		"stage", stage,
		"topic", record.Topic,
		"partition", record.Partition,
		"offset", record.Offset,
		"error", reason,
	)

	dlqRecord := cloneRecord(record)
	dlqRecord.Topic = s.dlqTopic
	dlqRecord.Headers = append(dlqRecord.Headers,
		kgo.RecordHeader{Key: "x-error-reason", Value: []byte(reason.Error())},
		kgo.RecordHeader{Key: "x-failed-stage", Value: []byte(stage)},
		kgo.RecordHeader{Key: "x-original-topic", Value: []byte(record.Topic)},
	)

	results := s.client.ProduceSync(ctx, dlqRecord)
	if err := results.FirstErr(); err != nil {
		s.logger.Error("failed to produce to DLQ", "dlq_topic", s.dlqTopic, "error", err)
	}

	if s.metrics != nil {
		s.metrics.MessagesDLQ.WithLabelValues(stage).Inc()
	}
}

func cloneRecord(r *kgo.Record) *kgo.Record {
	cp := *r
	return &cp
}

// Close flushes and shuts down the DLQ producer.
func (s *Sender) Close() {
	if err := s.client.Flush(context.Background()); err != nil {
		s.logger.Error("flushing DLQ producer on close", "error", err)
	}
	s.client.Close()
}
