package enricher_test

import (
	"encoding/json"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/enricher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cfg(t string, kv ...string) config.EnrichmentConfig {
	m := map[string]interface{}{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return config.EnrichmentConfig{Type: t, Config: m}
}

func marshal(t *testing.T, v map[string]interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func unmarshal(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func TestEnricher_ValidInput_SHA256(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("uuid_from_transaction", "input_field", "$.txId", "output_field", "$.eventId", "algorithm", "sha256"),
	})
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"txId": "TX-001"})
	out, err := chain.Enrich(payload)
	require.NoError(t, err)

	result := unmarshal(t, out)
	assert.Equal(t, "TX-001", result["txId"])
	assert.NotEmpty(t, result["eventId"])
	assert.Len(t, result["eventId"], 64) // SHA-256 hex is 64 chars
}

func TestEnricher_ValidInput_MD5(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("uuid_from_transaction", "input_field", "$.txId", "output_field", "$.eventId", "algorithm", "md5"),
	})
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"txId": "TX-001"})
	out, err := chain.Enrich(payload)
	require.NoError(t, err)

	result := unmarshal(t, out)
	assert.Len(t, result["eventId"], 32) // MD5 hex is 32 chars
}

func TestEnricher_MissingInputField(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("uuid_from_transaction", "input_field", "$.txId", "output_field", "$.eventId", "algorithm", "sha256"),
	})
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"other": "value"})
	_, err = chain.Enrich(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "txId")
}

func TestEnricher_UnknownAlgorithm(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("uuid_from_transaction", "input_field", "$.txId", "output_field", "$.eventId", "algorithm", "argon2"),
	})
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"txId": "TX-001"})
	_, err = chain.Enrich(payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "argon2")
}

func TestEnricher_UnknownType(t *testing.T) {
	t.Parallel()
	_, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("non_existent_type"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non_existent_type")
}

func TestEnricher_EmptyChain_PassThrough(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain(nil)
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"key": "value"})
	out, err := chain.Enrich(payload)
	require.NoError(t, err)
	assert.Equal(t, payload, out)
}

func TestEnricher_DefaultAlgorithm_SHA256(t *testing.T) {
	t.Parallel()
	chain, err := enricher.NewChain([]config.EnrichmentConfig{
		cfg("uuid_from_transaction", "input_field", "$.txId", "output_field", "$.eventId"),
	})
	require.NoError(t, err)

	payload := marshal(t, map[string]interface{}{"txId": "TX-001"})
	out, err := chain.Enrich(payload)
	require.NoError(t, err)
	result := unmarshal(t, out)
	assert.Len(t, result["eventId"], 64)
}
