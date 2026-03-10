package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/metrics"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer is the interface for producing a message to a Kafka topic.
type Producer interface {
	Produce(ctx context.Context, topic string, record *kgo.Record) error
}

// DLQSender is the interface for routing a failed message to the DLQ.
type DLQSender interface {
	Send(ctx context.Context, record *kgo.Record, stage string, err error)
}

// Pipeline orchestrates the consume → route → transform → enrich → produce flow.
type Pipeline struct {
	router      *router.Router
	transformer *transformer.Transformer
	enricher    enricher.Enricher
	producer    Producer
	dlq         DLQSender
	metrics     *metrics.Metrics
	sourceTopic string
	logger      *slog.Logger
}

// Config holds all dependencies needed by the pipeline.
type Config struct {
	Router      *router.Router
	Transformer *transformer.Transformer
	Enricher    enricher.Enricher
	Producer    Producer
	DLQ         DLQSender
	Metrics     *metrics.Metrics
	SourceTopic string
	Logger      *slog.Logger
}

// New creates a Pipeline from the provided Config.
func New(cfg Config) *Pipeline {
	return &Pipeline{
		router:      cfg.Router,
		transformer: cfg.Transformer,
		enricher:    cfg.Enricher,
		producer:    cfg.Producer,
		dlq:         cfg.DLQ,
		metrics:     cfg.Metrics,
		sourceTopic: cfg.SourceTopic,
		logger:      cfg.Logger,
	}
}

// Run reads from records and processes each message through the full pipeline.
// It returns when ctx is cancelled or the records channel is closed.
func (p *Pipeline) Run(ctx context.Context, records <-chan *kgo.Record) {
	for {
		select {
		case <-ctx.Done():
			return
		case record, ok := <-records:
			if !ok {
				return
			}
			p.process(ctx, record)
		}
	}
}

func (p *Pipeline) process(ctx context.Context, record *kgo.Record) {
	start := time.Now()

	if p.metrics != nil {
		p.metrics.MessagesConsumed.WithLabelValues(p.sourceTopic).Inc()
	}

	// Stage 1: routing
	stageStart := time.Now()
	targetTopic, err := p.router.Resolve(record.Headers)
	if err != nil {
		p.handleFailure(ctx, record, "routing", err)
		return
	}
	p.observeStage("routing", stageStart)

	// Stage 2: transformation
	stageStart = time.Now()
	payload, err := p.transformer.Transform(record.Value)
	if err != nil {
		p.handleFailure(ctx, record, "transformation", err)
		return
	}
	p.observeStage("transformation", stageStart)

	// Stage 3: enrichment
	stageStart = time.Now()
	payload, err = p.enricher.Enrich(payload)
	if err != nil {
		p.handleFailure(ctx, record, "enrichment", err)
		return
	}
	p.observeStage("enrichment", stageStart)

	// Stage 4: produce
	stageStart = time.Now()
	outRecord := cloneRecord(record)
	outRecord.Value = payload

	if err := p.producer.Produce(ctx, targetTopic, outRecord); err != nil {
		p.handleFailure(ctx, record, "produce", err)
		return
	}
	p.observeStage("produce", stageStart)

	if p.metrics != nil {
		p.metrics.MessagesProduced.WithLabelValues(targetTopic, "success").Inc()
		p.metrics.ObserveStageDuration("total", start)
	}

	p.logger.Debug("message processed",
		"target_topic", targetTopic,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

func (p *Pipeline) handleFailure(ctx context.Context, record *kgo.Record, stage string, err error) {
	p.logger.Error("pipeline stage failed", "stage", stage, "error", err)
	if p.metrics != nil {
		p.metrics.MessagesProduced.WithLabelValues("", "failed").Inc()
	}
	if p.dlq != nil {
		p.dlq.Send(ctx, record, stage, err)
	}
}

func cloneRecord(r *kgo.Record) *kgo.Record {
	cp := *r
	return &cp
}

func (p *Pipeline) observeStage(stage string, start time.Time) {
	if p.metrics != nil {
		p.metrics.ObserveStageDuration(stage, start)
	}
}
