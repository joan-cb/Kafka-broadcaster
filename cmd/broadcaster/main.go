package main

import (
	"log/slog"
	"os"
)

var logger *slog.Logger

func init() {
	logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func main() {
	if err := run(); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	m := initMetrics(cfg)

	logger.Info("configuration loaded",
		"source_topic", cfg.Kafka.SourceTopic,
		"consumer_group", cfg.Kafka.ConsumerGroup,
		"dlq_topic", cfg.Kafka.DLQTopic,
	)

	ctx := setupSignalContext()

	startMetricsServerIfEnabled(cfg)

	cons, err := initConsumer(cfg)
	if err != nil {
		return err
	}
	defer cons.Close()

	prod, err := initProducer(cfg)
	if err != nil {
		return err
	}
	defer prod.Close()

	dlqSender, err := initDLQ(cfg, m)
	if err != nil {
		return err
	}
	defer dlqSender.Close()

	enricherChain, err := initEnricherChain(cfg)
	if err != nil {
		return err
	}

	p := initPipeline(cfg, prod, dlqSender, enricherChain, m)

	logger.Info("kafka-broadcaster started")

	records := cons.Run(ctx)
	p.Run(ctx, records)

	logger.Info("kafka-broadcaster shutting down")
	return nil
}
