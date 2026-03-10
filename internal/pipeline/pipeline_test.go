package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/globalcommerce/kafka-broadcaster/internal/pipeline"
	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

// stubProducer records all produced messages in memory.
type stubProducer struct {
	produced []*producedMsg
	err      error
}

type producedMsg struct {
	topic string
	value []byte
}

func (s *stubProducer) Produce(_ context.Context, topic string, record *kgo.Record) error {
	if s.err != nil {
		return s.err
	}
	s.produced = append(s.produced, &producedMsg{topic: topic, value: record.Value})
	return nil
}

// stubDLQ records all DLQ sends.
type stubDLQ struct {
	sends []dlqSend
}

type dlqSend struct {
	stage string
	err   error
}

func (s *stubDLQ) Send(_ context.Context, _ *kgo.Record, stage string, err error) {
	s.sends = append(s.sends, dlqSend{stage: stage, err: err})
}

// errorEnricher always returns an error.
type errorEnricher struct{}

func (e *errorEnricher) Enrich(_ []byte) ([]byte, error) {
	return nil, errors.New("enrichment error")
}

func record(t *testing.T, headerKey, headerVal string, value []byte) *kgo.Record {
	t.Helper()
	return &kgo.Record{
		Topic: "source.events",
		Value: value,
		Headers: []kgo.RecordHeader{
			{Key: headerKey, Value: []byte(headerVal)},
		},
	}
}

func noopEnricher(t *testing.T) enricher.Enricher {
	t.Helper()
	e, err := enricher.NewChain(nil)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func newPipeline(t *testing.T, prod pipeline.Producer, d pipeline.DLQSender, enc enricher.Enricher, rules []config.TransformationRule) *pipeline.Pipeline {
	t.Helper()
	if enc == nil {
		enc = noopEnricher(t)
	}
	return pipeline.New(pipeline.Config{
		Router:      router.New("target-topic"),
		Transformer: transformer.New(rules),
		Enricher:    enc,
		Producer:    prod,
		DLQ:         d,
		SourceTopic: "source.events",
		Logger:      newTestLogger(t),
	})
}

func TestPipeline_HappyPath(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, nil)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "orders.v1", []byte(`{"id":"1"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Len(t, prod.produced, 1)
	assert.Equal(t, "orders.v1", prod.produced[0].topic)
	assert.Empty(t, d.sends)
}

func TestPipeline_RoutingFailure_SendsToDLQ(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, nil)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "wrong-header", "orders.v1", []byte(`{"id":"1"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, prod.produced)
	assert.Len(t, d.sends, 1)
	assert.Equal(t, "routing", d.sends[0].stage)
}

func TestPipeline_TransformationFailure_SendsToDLQ(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, []config.TransformationRule{
		{From: "$.a", To: "$.b"},
	})

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "orders.v1", []byte(`not-valid-json`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, prod.produced)
	assert.Len(t, d.sends, 1)
	assert.Equal(t, "transformation", d.sends[0].stage)
}

func TestPipeline_EnrichmentFailure_SendsToDLQ(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, &errorEnricher{}, nil)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "orders.v1", []byte(`{"id":"1"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, prod.produced)
	assert.Len(t, d.sends, 1)
	assert.Equal(t, "enrichment", d.sends[0].stage)
}

func TestPipeline_ProduceFailure_SendsToDLQ(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{err: errors.New("broker unavailable")}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, nil)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "orders.v1", []byte(`{"id":"1"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Len(t, d.sends, 1)
	assert.Equal(t, "produce", d.sends[0].stage)
}

func TestPipeline_ContextCancelled_Exits(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *kgo.Record)

	done := make(chan struct{})
	go func() {
		p.Run(ctx, ch)
		close(done)
	}()

	cancel()
	<-done
}

func TestPipeline_Run_ClosedChannel_Exits(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	p := newPipeline(t, prod, d, nil, nil)

	ch := make(chan *kgo.Record)
	close(ch)

	done := make(chan struct{})
	go func() {
		p.Run(context.Background(), ch)
		close(done)
	}()

	<-done
}
