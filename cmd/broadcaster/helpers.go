package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/consumer"
	"github.com/globalcommerce/kafka-broadcaster/internal/contentrouter"
	"github.com/globalcommerce/kafka-broadcaster/internal/dlq"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/metrics"
	"github.com/globalcommerce/kafka-broadcaster/internal/pipeline"
	"github.com/globalcommerce/kafka-broadcaster/internal/producer"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
)

func loadConfig() (*config.Config, error) {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "/etc/broadcaster/config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// initMetrics creates the Prometheus metrics registry. Must be called after
// loadConfig so the consumer group is available as the client label.
func initMetrics(cfg *config.Config) *metrics.Metrics {
	clientName := cfg.Kafka.ConsumerGroup
	if clientName == "" {
		clientName = "broadcaster"
	}
	return metrics.New(clientName)
}

func setupSignalContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	_ = stop
	return ctx
}

func startMetricsServerIfEnabled(cfg *config.Config) {
	if !cfg.Metrics.Enabled {
		return
	}
	addr := fmt.Sprintf(":%d", cfg.Metrics.Port)
	go metrics.StartServer(addr, cfg.Metrics.Path, logger)
}

func initConsumer(cfg *config.Config) (*consumer.Consumer, error) {
	cons, err := consumer.New(&cfg.Kafka, logger)
	if err != nil {
		return nil, fmt.Errorf("creating consumer: %w", err)
	}
	return cons, nil
}

func initProducer(cfg *config.Config) (*producer.Producer, error) {
	prod, err := producer.New(&cfg.Kafka, logger)
	if err != nil {
		return nil, fmt.Errorf("creating producer: %w", err)
	}
	return prod, nil
}

func initDLQ(cfg *config.Config, m *metrics.Metrics) (*dlq.Sender, error) {
	dlqSender, err := dlq.New(cfg.Kafka.Brokers, cfg.Kafka.DLQTopic, m, logger)
	if err != nil {
		return nil, fmt.Errorf("creating DLQ sender: %w", err)
	}
	return dlqSender, nil
}

func initEnricherChain(cfg *config.Config) (enricher.Enricher, error) {
	chain, err := enricher.NewChain(cfg.Enrichment)
	if err != nil {
		return nil, fmt.Errorf("building enricher chain: %w", err)
	}
	return chain, nil
}

func initPipeline(cfg *config.Config, prod *producer.Producer, dlqSender *dlq.Sender, enricherChain enricher.Enricher, m *metrics.Metrics) (*pipeline.Pipeline, error) {
	cr, err := contentrouter.New(cfg.Routing.ContentRouting)
	if err != nil {
		return nil, fmt.Errorf("content router: %w", err)
	}
	return pipeline.New(pipeline.Config{
		Router:        router.New(cfg.Routing.HeaderKey),
		ContentRouter: cr,
		Transformer:   transformer.New(cfg.Transformation.Rules),
		Enricher:      enricherChain,
		Producer:      prod,
		DLQ:           dlqSender,
		Metrics:       m,
		SourceTopic:   cfg.Kafka.SourceTopic,
		Logger:        logger,
	}), nil
}
