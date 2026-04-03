package contentrouter_test

import (
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/contentrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Disabled_ReturnsNil(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{Enabled: false})
	require.NoError(t, err)
	assert.Nil(t, r)
}

func TestNew_Enabled_InvalidValueType(t *testing.T) {
	t.Parallel()
	_, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: "float",
		Routes:    map[string][]string{"x": {"t"}},
	})
	require.Error(t, err)
}

func TestResolveTopics_StringNestedPath(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.event.type",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"order.created": {"topic.orders", "topic.audit"},
		},
	})
	require.NoError(t, err)

	topics, err := r.ResolveTopics([]byte(`{"event":{"type":"order.created"}}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"topic.orders", "topic.audit"}, topics)
}

func TestResolveTopics_DedupesTopics(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes: map[string][]string{
			"a": {"t1", "t1", "t2", "t1"},
		},
	})
	require.NoError(t, err)

	topics, err := r.ResolveTopics([]byte(`{"k":"a"}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"t1", "t2"}, topics)
}

func TestResolveTopics_BoolStrict(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.flag",
		ValueType: config.ContentValueBool,
		Routes: map[string][]string{
			"true":  {"on.topic"},
			"false": {"off.topic"},
		},
	})
	require.NoError(t, err)

	topics, err := r.ResolveTopics([]byte(`{"flag":true}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"on.topic"}, topics)

	_, err = r.ResolveTopics([]byte(`{"flag":"false"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boolean")
}

func TestResolveTopics_IntIntegerJSONOnly(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.n",
		ValueType: config.ContentValueInt,
		Routes: map[string][]string{
			"42": {"t.int"},
		},
	})
	require.NoError(t, err)

	topics, err := r.ResolveTopics([]byte(`{"n":42}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"t.int"}, topics)

	_, err = r.ResolveTopics([]byte(`{"n":3.14}`))
	require.Error(t, err)
}

func TestResolveTopics_MissingPath(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.missing",
		ValueType: config.ContentValueString,
		Routes:    map[string][]string{"x": {"t"}},
	})
	require.NoError(t, err)

	_, err = r.ResolveTopics([]byte(`{"other":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveTopics_NullLeaf(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes:    map[string][]string{"x": {"t"}},
	})
	require.NoError(t, err)

	_, err = r.ResolveTopics([]byte(`{"k":null}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null")
}

func TestResolveTopics_UnmappedValue(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes:    map[string][]string{"known": {"t"}},
	})
	require.NoError(t, err)

	_, err = r.ResolveTopics([]byte(`{"k":"unknown"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmapped")
}

func TestResolveTopics_StringType_RejectsNumber(t *testing.T) {
	t.Parallel()
	r, err := contentrouter.New(config.ContentRoutingConfig{
		Enabled:   true,
		KeyPath:   "$.k",
		ValueType: config.ContentValueString,
		Routes:    map[string][]string{"1": {"t"}},
	})
	require.NoError(t, err)

	_, err = r.ResolveTopics([]byte(`{"k":1}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "string")
}
