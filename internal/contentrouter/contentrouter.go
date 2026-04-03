package contentrouter

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/jsonpath"
)

// Router resolves one or more target Kafka topics from a JSON field in the
// payload (after transformation, before enrichment). When disabled at config
// load time, New returns (nil, nil) and the pipeline uses header routing only.
type Router struct {
	keyPath   string
	valueType config.ContentValueType
	routes    map[string][]string
}

// New builds a Router from config, or returns (nil, nil) when content routing
// is disabled.
func New(c config.ContentRoutingConfig) (*Router, error) {
	if !c.Enabled {
		return nil, nil
	}
	if c.KeyPath == "" {
		return nil, fmt.Errorf("content routing: key_path must not be empty when enabled")
	}
	if err := c.ValueType.Validate(); err != nil {
		return nil, err
	}
	if len(c.Routes) == 0 {
		return nil, fmt.Errorf("content routing: routes must not be empty when enabled")
	}
	for k, topics := range c.Routes {
		if k == "" {
			return nil, fmt.Errorf("content routing: route keys must not be empty")
		}
		if len(topics) == 0 {
			return nil, fmt.Errorf("content routing: routes[%q] must list at least one topic", k)
		}
		for i, t := range topics {
			if t == "" {
				return nil, fmt.Errorf("content routing: routes[%q][%d] must not be empty", k, i)
			}
		}
	}
	return &Router{
		keyPath:   c.KeyPath,
		valueType: c.ValueType,
		routes:    c.Routes,
	}, nil
}

// ResolveTopics reads key_path from JSON payload, coerces the value to the
// configured strict type, looks up topics, and returns a deduplicated list
// (first occurrence order). Missing path, null, wrong JSON type, or unmapped
// values return an error suitable for DLQ stage content_routing.
func (r *Router) ResolveTopics(payload []byte) ([]string, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("contentrouter: unmarshal payload: %w", err)
	}
	parts := jsonpath.ParsePath(r.keyPath)
	raw, ok := jsonpath.Get(doc, parts)
	if !ok {
		return nil, fmt.Errorf("contentrouter: key path not found")
	}
	if raw == nil {
		return nil, fmt.Errorf("contentrouter: key path is null")
	}
	lookupKey, err := r.lookupKey(raw)
	if err != nil {
		return nil, err
	}
	topics, ok := r.routes[lookupKey]
	if !ok || len(topics) == 0 {
		return nil, fmt.Errorf("contentrouter: unmapped routing value")
	}
	return dedupePreserveOrder(topics), nil
}

func (r *Router) lookupKey(raw interface{}) (string, error) {
	switch r.valueType {
	case config.ContentValueString:
		s, ok := raw.(string)
		if !ok {
			return "", fmt.Errorf("contentrouter: expected string JSON value")
		}
		return s, nil
	case config.ContentValueBool:
		b, ok := raw.(bool)
		if !ok {
			return "", fmt.Errorf("contentrouter: expected JSON boolean")
		}
		return strconv.FormatBool(b), nil
	case config.ContentValueInt:
		return integerLookupKey(raw)
	default:
		return "", fmt.Errorf("contentrouter: unsupported value_type")
	}
}

func integerLookupKey(raw interface{}) (string, error) {
	switch v := raw.(type) {
	case float64:
		if math.Trunc(v) != v || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("contentrouter: expected integer JSON number")
		}
		if v > float64(math.MaxInt64) || v < float64(math.MinInt64) {
			return "", fmt.Errorf("contentrouter: integer out of range")
		}
		return strconv.FormatInt(int64(v), 10), nil
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return "", fmt.Errorf("contentrouter: expected integer JSON number")
		}
		return strconv.FormatInt(i, 10), nil
	default:
		return "", fmt.Errorf("contentrouter: expected integer JSON number")
	}
}

func dedupePreserveOrder(topics []string) []string {
	seen := make(map[string]struct{}, len(topics))
	out := make([]string, 0, len(topics))
	for _, t := range topics {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}
