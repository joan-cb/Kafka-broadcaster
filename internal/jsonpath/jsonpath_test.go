package jsonpath_test

import (
	"testing"

	"github.com/globalcommerce/kafka-broadcaster/internal/jsonpath"
	"github.com/stretchr/testify/assert"
)

func TestParsePath_DollarDotStyle(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"user", "name"}, jsonpath.ParsePath("$.user.name"))
	assert.Equal(t, []string{"a"}, jsonpath.ParsePath("$.a"))
}

func TestGet_Nested(t *testing.T) {
	t.Parallel()
	doc := map[string]interface{}{
		"event": map[string]interface{}{"type": "x"},
	}
	v, ok := jsonpath.Get(doc, []string{"event", "type"})
	assert.True(t, ok)
	assert.Equal(t, "x", v)
}
