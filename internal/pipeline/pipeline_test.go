package pipeline_test

import (
	"context"
	"errors"
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
	return newPipelineWithContent(t, prod, d, enc, rules, nil)
}

func newPipelineWithContent(t *testing.T, prod pipeline.Producer, d pipeline.DLQSender, enc enricher.Enricher, rules []config.TransformationRule, cr *contentrouter.Router) *pipeline.Pipeline {
	t.Helper()
	if enc == nil {
		enc = noopEnricher(t)
	}
	return pipeline.New(pipeline.Config{
		Router:        router.New("target-topic"),
		ContentRouter: cr,
		Transformer:   transformer.New(rules),
		Enricher:      enc,
		Producer:      prod,
		DLQ:           d,
		SourceTopic:   "source.events",
		Logger:        newTestLogger(t),
	})
}

func mustContentRouter(t *testing.T, cfg config.ContentRoutingConfig) *contentrouter.Router {
	t.Helper()
	r, err := contentrouter.New(cfg)
	require.NoError(t, err)
	require.NotNil(t, r)
	return r
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

func TestPipeline_ContentRouting_HappyPath_IgnoresHeader(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	cr := mustContentRouter(t, config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.event.type",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"order.created": {"topic.orders"},
		},
	})
	p := newPipelineWithContent(t, prod, d, nil, nil, cr)

	ch := make(chan *kgo.Record, 1)
	ch <- &kgo.Record{
		Topic:   "source.events",
		Value:   []byte(`{"event":{"type":"order.created"}}`),
		Headers: nil,
	}
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.sends)
	require.Len(t, prod.produced, 1)
	assert.Equal(t, "topic.orders", prod.produced[0].topic)
}

func TestPipeline_ContentRouting_MultiTarget_Deduped(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	cr := mustContentRouter(t, config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"x": {"t1", "t1", "t2"},
		},
	})
	p := newPipelineWithContent(t, prod, d, nil, nil, cr)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "ignored", []byte(`{"k":"x"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.sends)
	require.Len(t, prod.produced, 2)
	assert.Equal(t, "t1", prod.produced[0].topic)
	assert.Equal(t, "t2", prod.produced[1].topic)
	assert.Equal(t, prod.produced[0].value, prod.produced[1].value)
}

func TestPipeline_ContentRouting_Unmapped_SendsDLQ(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	cr := mustContentRouter(t, config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes:    map[string][]string{"known": {"t"}},
	})
	p := newPipelineWithContent(t, prod, d, nil, nil, cr)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "would-be-target", []byte(`{"k":"other"}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, prod.produced)
	require.Len(t, d.sends, 1)
	assert.Equal(t, "content_routing", d.sends[0].stage)
}

func TestPipeline_ContentRouting_AfterTransform_UsesRenamedField(t *testing.T) {
	t.Parallel()
	prod := &stubProducer{}
	d := &stubDLQ{}
	cr := mustContentRouter(t, config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.eventType",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"a": {"out"},
		},
	})
	p := newPipelineWithContent(t, prod, d, nil, []config.TransformationRule{
		{From: "$.event.type", To: "$.eventType"},
	}, cr)

	ch := make(chan *kgo.Record, 1)
	ch <- record(t, "target-topic", "ignored", []byte(`{"event":{"type":"a"}}`))
	close(ch)

	p.Run(context.Background(), ch)

	assert.Empty(t, d.sends)
	require.Len(t, prod.produced, 1)
	assert.Equal(t, "out", prod.produced[0].topic)
}
