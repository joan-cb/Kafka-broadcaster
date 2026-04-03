//go:build stubbed

package stubbed_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/contentrouter"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/pipeline"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"log/slog"
	"os"
)

// in-memory stub implementations

type memProducer struct {
	messages map[string][][]byte
}

func newMemProducer() *memProducer {
	return &memProducer{messages: make(map[string][][]byte)}
}

func (m *memProducer) Produce(_ context.Context, topic string, record *kgo.Record) error {
	m.messages[topic] = append(m.messages[topic], record.Value)
	return nil
}

type memDLQ struct {
	records []dlqEntry
}

type dlqEntry struct {
	stage  string
	record *kgo.Record
}

func (m *memDLQ) Send(_ context.Context, record *kgo.Record, stage string, _ error) {
	m.records = append(m.records, dlqEntry{stage: stage, record: record})
}

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, nil))
}

func TestStubbed_FullPipeline_RouteTransformEnrich(t *testing.T) {
	prod := newMemProducer()
	d := &memDLQ{}

	enricherChain, err := enricher.NewChain([]config.EnrichmentConfig{
		{
			Type: "uuid_from_transaction",
			Config: map[string]interface{}{
				"input_field":  "$.txId",
				"output_field": "$.eventId",
				"algorithm":    "sha256",
			},
		},
	})
	require.NoError(t, err)

	p := pipeline.New(pipeline.Config{
		Router: router.New("target-topic"),
		Transformer: transformer.New([]config.TransformationRule{
			{From: "$.transaction_id", To: "$.txId"},
		}),
		Enricher:    enricherChain,
		Producer:    prod,
		DLQ:         d,
		SourceTopic: "internal.events",
		Logger:      logger(),
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"transaction_id": "TX-12345",
		"amount":         99.99,
	})

	ch := make(chan *kgo.Record, 1)
	ch <- &kgo.Record{
		Topic: "internal.events",
		Value: payload,
		Headers: []kgo.RecordHeader{
			{Key: "target-topic", Value: []byte("orders.processed")},
		},
	}
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.records, "no messages should be DLQ'd")
	require.Len(t, prod.messages["orders.processed"], 1)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(prod.messages["orders.processed"][0], &result))

	assert.Equal(t, "TX-12345", result["txId"], "transformation must rename transaction_id → txId")
	assert.NotEmpty(t, result["eventId"], "enrichment must populate eventId")
	assert.Nil(t, result["transaction_id"], "original key must be removed")
}

func TestStubbed_MissingHeader_RoutesToDLQ(t *testing.T) {
	prod := newMemProducer()
	d := &memDLQ{}

	enc, _ := enricher.NewChain(nil)
	p := pipeline.New(pipeline.Config{
		Router:      router.New("target-topic"),
		Transformer: transformer.New(nil),
		Enricher:    enc,
		Producer:    prod,
		DLQ:         d,
		SourceTopic: "internal.events",
		Logger:      logger(),
	})

	ch := make(chan *kgo.Record, 1)
	ch <- &kgo.Record{
		Topic: "internal.events",
		Value: []byte(`{"id":"1"}`),
		// no headers at all
	}
	close(ch)

	p.Run(context.Background(), ch)

	assert.Len(t, d.records, 1)
	assert.Equal(t, "routing", d.records[0].stage)
	assert.Empty(t, prod.messages)
}

func TestStubbed_MultipleMessages_AllRouted(t *testing.T) {
	prod := newMemProducer()
	d := &memDLQ{}

	enc, _ := enricher.NewChain(nil)
	p := pipeline.New(pipeline.Config{
		Router:      router.New("target-topic"),
		Transformer: transformer.New(nil),
		Enricher:    enc,
		Producer:    prod,
		DLQ:         d,
		SourceTopic: "internal.events",
		Logger:      logger(),
	})

	const msgCount = 10
	ch := make(chan *kgo.Record, msgCount)
	for i := 0; i < msgCount; i++ {
		ch <- &kgo.Record{
			Topic: "internal.events",
			Value: []byte(`{"id":"x"}`),
			Headers: []kgo.RecordHeader{
				{Key: "target-topic", Value: []byte("target.topic")},
			},
		}
	}
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.records)
	assert.Len(t, prod.messages["target.topic"], msgCount)
}

func TestStubbed_ContentRouting_EndToEnd(t *testing.T) {
	prod := newMemProducer()
	d := &memDLQ{}

	cr, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.kind",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"order": {"orders.out", "audit.out"},
		},
	})
	require.NoError(t, err)

	enc, err := enricher.NewChain([]config.EnrichmentConfig{
		{
			Type: "uuid_from_transaction",
			Config: map[string]interface{}{
				"input_field":  "$.txId",
				"output_field": "$.eventId",
				"algorithm":    "sha256",
			},
		},
	})
	require.NoError(t, err)

	p := pipeline.New(pipeline.Config{
		Router:        router.New("target-topic"),
		ContentRouter: cr,
		Transformer: transformer.New([]config.TransformationRule{
			{From: "$.transaction_id", To: "$.txId"},
		}),
		Enricher:    enc,
		Producer:    prod,
		DLQ:         d,
		SourceTopic: "internal.events",
		Logger:      logger(),
	})

	payload, _ := json.Marshal(map[string]interface{}{
		"transaction_id": "TX-CR-1",
		"kind":           "order",
	})

	ch := make(chan *kgo.Record, 1)
	ch <- &kgo.Record{
		Topic:   "internal.events",
		Value:   payload,
		Headers: nil,
	}
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.records)
	require.Len(t, prod.messages["orders.out"], 1)
	require.Len(t, prod.messages["audit.out"], 1)
	assert.Equal(t, prod.messages["orders.out"][0], prod.messages["audit.out"][0])

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(prod.messages["orders.out"][0], &result))
	assert.Equal(t, "TX-CR-1", result["txId"])
	assert.NotEmpty(t, result["eventId"])
}
