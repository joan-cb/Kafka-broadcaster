package transformer_test

import (
	"encoding/json"
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/transformer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonPayload(t *testing.T, v map[string]interface{}) []byte {
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

func TestTransform_KeyRename(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.txId", To: "$.transactionId"},
	})
	in := jsonPayload(t, map[string]interface{}{"txId": "abc123", "amount": 99})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	result := unmarshal(t, out)
	assert.Equal(t, "abc123", result["transactionId"])
	assert.Nil(t, result["txId"])
	assert.Equal(t, float64(99), result["amount"])
}

func TestTransform_NestedPathChange(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.user.name", To: "$.userName"},
	})
	in := jsonPayload(t, map[string]interface{}{
		"user": map[string]interface{}{"name": "Alice"},
	})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	result := unmarshal(t, out)
	assert.Equal(t, "Alice", result["userName"])
	user, _ := result["user"].(map[string]interface{})
	assert.Nil(t, user["name"])
}

func TestTransform_MoveToNestedPath(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.id", To: "$.meta.id"},
	})
	in := jsonPayload(t, map[string]interface{}{"id": "xyz"})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	result := unmarshal(t, out)
	meta, _ := result["meta"].(map[string]interface{})
	assert.Equal(t, "xyz", meta["id"])
	assert.Nil(t, result["id"])
}

func TestTransform_MissingSourceKey_Skipped(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.nonexistent", To: "$.destination"},
	})
	in := jsonPayload(t, map[string]interface{}{"other": "value"})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	result := unmarshal(t, out)
	assert.Equal(t, "value", result["other"])
	assert.Nil(t, result["destination"])
}

func TestTransform_MultipleRules(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.a", To: "$.x"},
		{From: "$.b", To: "$.y"},
	})
	in := jsonPayload(t, map[string]interface{}{"a": 1, "b": 2})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	result := unmarshal(t, out)
	assert.Equal(t, float64(1), result["x"])
	assert.Equal(t, float64(2), result["y"])
	assert.Nil(t, result["a"])
	assert.Nil(t, result["b"])
}

func TestTransform_NoRules_PassThrough(t *testing.T) {
	t.Parallel()
	tr := transformer.New(nil)
	in := jsonPayload(t, map[string]interface{}{"key": "value"})
	out, err := tr.Transform(in)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestTransform_InvalidJSON(t *testing.T) {
	t.Parallel()
	tr := transformer.New([]config.TransformationRule{
		{From: "$.a", To: "$.b"},
	})
	_, err := tr.Transform([]byte(`{not valid json`))
	require.Error(t, err)
}
