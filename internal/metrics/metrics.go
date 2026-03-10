package metrics

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus counters and histograms for kafka-broadcaster.
type Metrics struct {
	MessagesConsumed  *prometheus.CounterVec
	MessagesProduced  *prometheus.CounterVec
	MessagesDLQ       *prometheus.CounterVec
	PipelineDuration  *prometheus.HistogramVec
}

// New registers all metrics with the default Prometheus registry and returns a Metrics instance.
func New(client string) *Metrics {
	m := &Metrics{
		MessagesConsumed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "broadcaster_messages_consumed_total",
			Help:        "Total number of messages consumed from the source topic.",
			ConstLabels: prometheus.Labels{"client": client},
		}, []string{"source_topic"}),

		MessagesProduced: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "broadcaster_messages_produced_total",
			Help:        "Total number of messages produced to target topics.",
			ConstLabels: prometheus.Labels{"client": client},
		}, []string{"target_topic", "status"}),

		MessagesDLQ: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "broadcaster_messages_dlq_total",
			Help:        "Total number of messages sent to the DLQ.",
			ConstLabels: prometheus.Labels{"client": client},
		}, []string{"stage"}),

		PipelineDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:        "broadcaster_pipeline_duration_seconds",
			Help:        "Duration of each pipeline stage in seconds.",
			ConstLabels: prometheus.Labels{"client": client},
			Buckets:     prometheus.DefBuckets,
		}, []string{"stage"}),
	}

	prometheus.MustRegister(
		m.MessagesConsumed,
		m.MessagesProduced,
		m.MessagesDLQ,
		m.PipelineDuration,
	)

	return m
}

// ObserveStageDuration records how long a pipeline stage took.
func (m *Metrics) ObserveStageDuration(stage string, start time.Time) {
	m.PipelineDuration.WithLabelValues(stage).Observe(time.Since(start).Seconds())
}

// StartServer starts the Prometheus HTTP metrics server on the given address and path.
// It blocks until the server fails; errors are logged.
func StartServer(addr, path string, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle(path, promhttp.Handler())
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	logger.Info("starting metrics server", "addr", fmt.Sprintf("http://%s%s", addr, path))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("metrics server error", "error", err)
	}
}
