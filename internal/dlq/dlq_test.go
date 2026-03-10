package dlq_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/globalcommerce/kafka-broadcaster/internal/dlq"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
)

// TestSend_DoesNotPanic verifies Send handles a non-responsive broker gracefully.
// Full DLQ delivery is covered by functional/e2e tests.
func TestSend_DoesNotPanic(t *testing.T) {
	t.Parallel()
	sender, err := dlq.New([]string{"localhost:19092"}, "test.dlq", nil, newTestLogger(t))
	require.NoError(t, err)

	record := &kgo.Record{
		Topic: "source.topic",
		Value: []byte(`{"id":"1"}`),
	}

	// Use a short-lived context so ProduceSync does not block indefinitely against
	// a broker that is intentionally not running.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NotPanics(t, func() {
		sender.Send(ctx, record, "routing", errors.New("header missing"))
	})
}

func TestNew_ValidArgs_ReturnsNoError(t *testing.T) {
	t.Parallel()
	// franz-go does not validate brokers at construction time.
	sender, err := dlq.New([]string{"localhost:19092"}, "dlq", nil, newTestLogger(t))
	require.NoError(t, err)
	require.NotNil(t, sender)
}
