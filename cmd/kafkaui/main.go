package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	port := os.Getenv("UI_PORT")
	if port == "" {
		port = "8080"
	}

	brokerList := strings.Split(brokers, ",")

	srv := &server{
		brokers: brokerList,
		logger:  logger,
	}

	static, _ := fs.Sub(staticFiles, "static")

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/api/topics", srv.handleTopics)
	mux.HandleFunc("/api/produce", srv.handleProduce)
	mux.HandleFunc("/api/consume", srv.handleConsume)

	addr := ":" + port
	logger.Info("kafka-broadcaster UI started", "addr", fmt.Sprintf("http://localhost%s", addr), "brokers", brokers)

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE streams need no write timeout
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

type server struct {
	brokers []string
	logger  *slog.Logger
}

// handleTopics returns a list of all topic names on the cluster.
func (s *server) handleTopics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	client, err := kgo.NewClient(kgo.SeedBrokers(s.brokers...))
	if err != nil {
		s.jsonError(w, "failed to connect to Kafka", err, http.StatusInternalServerError)
		return
	}
	defer client.Close()

	adm := kadm.NewClient(client)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	metadata, err := adm.ListTopics(ctx)
	if err != nil {
		s.jsonError(w, "failed to list topics", err, http.StatusInternalServerError)
		return
	}

	topics := make([]string, 0, len(metadata))
	for name := range metadata {
		topics = append(topics, name)
	}

	s.jsonOK(w, map[string]interface{}{"topics": topics})
}

// produceRequest is the JSON body for POST /api/produce.
type produceRequest struct {
	Broker  string            `json:"broker"`
	Topic   string            `json:"topic"`
	Key     string            `json:"key"`
	Payload string            `json:"payload"`
	Headers map[string]string `json:"headers"`
}

// handleProduce produces a single message to the specified topic.
func (s *server) handleProduce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req produceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "invalid request body", err, http.StatusBadRequest)
		return
	}
	if req.Topic == "" {
		s.jsonError(w, "topic is required", nil, http.StatusBadRequest)
		return
	}

	brokers := s.brokers
	if req.Broker != "" {
		brokers = strings.Split(req.Broker, ",")
	}

	client, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		s.jsonError(w, "failed to connect to Kafka", err, http.StatusInternalServerError)
		return
	}
	defer client.Close()

	headers := make([]kgo.RecordHeader, 0, len(req.Headers))
	for k, v := range req.Headers {
		if k != "" {
			headers = append(headers, kgo.RecordHeader{Key: k, Value: []byte(v)})
		}
	}

	record := &kgo.Record{
		Topic:   req.Topic,
		Value:   []byte(req.Payload),
		Headers: headers,
	}
	if req.Key != "" {
		record.Key = []byte(req.Key)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	results := client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		s.jsonError(w, "failed to produce message", err, http.StatusInternalServerError)
		return
	}

	pr := results[0]
	s.jsonOK(w, map[string]interface{}{
		"topic":     pr.Record.Topic,
		"partition": pr.Record.Partition,
		"offset":    pr.Record.Offset,
	})
}

// handleConsume streams messages from a topic as Server-Sent Events.
// Query params: topic (required), from=beginning|latest (default: latest), limit (default: 50).
func (s *server) handleConsume(w http.ResponseWriter, r *http.Request) {
	topic := r.URL.Query().Get("topic")
	if topic == "" {
		http.Error(w, "topic query param is required", http.StatusBadRequest)
		return
	}
	from := r.URL.Query().Get("from")
	broker := r.URL.Query().Get("broker")

	brokers := s.brokers
	if broker != "" {
		brokers = strings.Split(broker, ",")
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumerGroup(fmt.Sprintf("kafkaui-%d", time.Now().UnixNano())),
	}
	if from == "beginning" {
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	} else {
		opts = append(opts, kgo.ConsumeResetOffset(kgo.NewOffset().AtEnd()))
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		http.Error(w, "failed to connect to Kafka: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send a connected event so the UI knows the stream is live.
	fmt.Fprintf(w, "event: connected\ndata: {\"topic\":%q}\n\n", topic)
	flusher.Flush()

	ctx := r.Context()
	for {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return
		}

		fetches.EachError(func(t string, p int32, err error) {
			s.logger.Error("consume error", "topic", t, "partition", p, "error", err)
		})

		fetches.EachRecord(func(rec *kgo.Record) {
			headers := make(map[string]string, len(rec.Headers))
			for _, h := range rec.Headers {
				headers[h.Key] = string(h.Value)
			}

			msg := map[string]interface{}{
				"topic":     rec.Topic,
				"partition": rec.Partition,
				"offset":    rec.Offset,
				"key":       string(rec.Key),
				"value":     string(rec.Value),
				"headers":   headers,
				"timestamp": rec.Timestamp.Format(time.RFC3339),
			}
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		})
	}
}

func (s *server) jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) jsonError(w http.ResponseWriter, msg string, err error, code int) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	s.logger.Error(msg, "error", errStr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "detail": errStr})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
