package transformer

import (
	"encoding/json"
	"fmt"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
	"github.com/globalcommerce/kafka-broadcaster/internal/jsonpath"
)

// Transformer applies a set of JSON field move/rename rules to a payload.
type Transformer struct {
	rules []config.TransformationRule
}

// New creates a Transformer from the provided rules.
func New(rules []config.TransformationRule) *Transformer {
	return &Transformer{rules: rules}
}

// Transform applies all configured rules to the JSON payload and returns the
// modified payload. Rules are applied in order; if a source path is absent the
// rule is skipped rather than returning an error.
func (t *Transformer) Transform(payload []byte) ([]byte, error) {
	if len(t.rules) == 0 {
		return payload, nil
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("transformer: unmarshal payload: %w", err)
	}

	for _, rule := range t.rules {
		fromParts := jsonpath.ParsePath(rule.From)
		toParts := jsonpath.ParsePath(rule.To)

		value, ok := jsonpath.Get(doc, fromParts)
		if !ok {
			continue
		}
		deletePath(doc, fromParts)
		setPath(doc, toParts, value)
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("transformer: marshal payload: %w", err)
	}
	return out, nil
}

// deletePath removes the leaf key described by parts from doc.
func deletePath(doc map[string]interface{}, parts []string) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		delete(doc, parts[0])
		return
	}
	child, ok := doc[parts[0]]
	if !ok {
		return
	}
	if m, ok := child.(map[string]interface{}); ok {
		deletePath(m, parts[1:])
	}
}

// setPath creates intermediate maps as needed and sets value at the leaf described by parts.
func setPath(doc map[string]interface{}, parts []string, value interface{}) {
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		doc[parts[0]] = value
		return
	}
	child, ok := doc[parts[0]]
	if !ok {
		child = map[string]interface{}{}
		doc[parts[0]] = child
	}
	m, ok := child.(map[string]interface{})
	if !ok {
		m = map[string]interface{}{}
		doc[parts[0]] = m
	}
	setPath(m, parts[1:], value)
}
