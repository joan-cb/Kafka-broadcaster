package router_test

import (
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

func headers(pairs ...string) []kgo.RecordHeader {
	h := make([]kgo.RecordHeader, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		h = append(h, kgo.RecordHeader{Key: pairs[i], Value: []byte(pairs[i+1])})
	}
	return h
}

func TestRouter_Resolve_HeaderPresent(t *testing.T) {
	t.Parallel()
	r := router.New("target-topic")
	topic, err := r.Resolve(headers("target-topic", "orders.created"))
	require.NoError(t, err)
	assert.Equal(t, "orders.created", topic)
}

func TestRouter_Resolve_HeaderAbsent(t *testing.T) {
	t.Parallel()
	r := router.New("target-topic")
	_, err := r.Resolve(headers("some-other-header", "value"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target-topic")
}

func TestRouter_Resolve_EmptyHeaders(t *testing.T) {
	t.Parallel()
	r := router.New("target-topic")
	_, err := r.Resolve(nil)
	require.Error(t, err)
}

func TestRouter_Resolve_EmptyHeaderValue(t *testing.T) {
	t.Parallel()
	r := router.New("target-topic")
	_, err := r.Resolve(headers("target-topic", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty value")
}

func TestRouter_Resolve_MultipleHeaders_CorrectKeySelected(t *testing.T) {
	t.Parallel()
	r := router.New("target-topic")
	topic, err := r.Resolve(headers(
		"x-request-id", "abc-123",
		"target-topic", "payments.events",
		"x-source", "checkout",
	))
	require.NoError(t, err)
	assert.Equal(t, "payments.events", topic)
}
