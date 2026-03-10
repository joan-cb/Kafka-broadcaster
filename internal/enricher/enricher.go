package enricher

import (
	"crypto/md5"  //nolint:gosec
	"crypto/sha1" //nolint:gosec
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/globalcommerce/kafka-broadcaster/internal/config"
)

// Enricher transforms a JSON payload by adding or modifying fields.
type Enricher interface {
	Enrich(payload []byte) ([]byte, error)
}

// Chain applies multiple Enrichers in sequence.
type Chain []Enricher

// Enrich applies each enricher in the chain, threading the output of each into the next.
func (c Chain) Enrich(payload []byte) ([]byte, error) {
	var err error
	for _, e := range c {
		payload, err = e.Enrich(payload)
		if err != nil {
			return nil, err
		}
	}
	return payload, nil
}

// NewChain builds an Enricher chain from config. Returns an error for unknown types.
func NewChain(cfgs []config.EnrichmentConfig) (Enricher, error) {
	chain := make(Chain, 0, len(cfgs))
	for _, cfg := range cfgs {
		e, err := newFromConfig(cfg)
		if err != nil {
			return nil, err
		}
		chain = append(chain, e)
	}
	return chain, nil
}

func newFromConfig(cfg config.EnrichmentConfig) (Enricher, error) {
	switch cfg.Type {
	case "uuid_from_transaction":
		return newHashEnricher(cfg.Config)
	default:
		return nil, fmt.Errorf("enricher: unknown type %q", cfg.Type)
	}
}

// hashEnricher reads a field, applies a hash algorithm, and writes the result to an output field.
type hashEnricher struct {
	inputField  []string
	outputField []string
	algorithm   string
}

func newHashEnricher(cfg map[string]interface{}) (*hashEnricher, error) {
	input, _ := cfg["input_field"].(string)
	output, _ := cfg["output_field"].(string)
	algorithm, _ := cfg["algorithm"].(string)

	if input == "" {
		return nil, fmt.Errorf("enricher uuid_from_transaction: input_field is required")
	}
	if output == "" {
		return nil, fmt.Errorf("enricher uuid_from_transaction: output_field is required")
	}
	if algorithm == "" {
		algorithm = "sha256"
	}

	return &hashEnricher{
		inputField:  parsePath(input),
		outputField: parsePath(output),
		algorithm:   algorithm,
	}, nil
}

func (h *hashEnricher) Enrich(payload []byte) ([]byte, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("enricher: unmarshal: %w", err)
	}

	value, ok := getPath(doc, h.inputField)
	if !ok {
		return nil, fmt.Errorf("enricher: input field %q not found in payload", strings.Join(h.inputField, "."))
	}

	raw := fmt.Sprintf("%v", value)
	hashed, err := hash(h.algorithm, raw)
	if err != nil {
		return nil, err
	}

	setPath(doc, h.outputField, hashed)

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("enricher: marshal: %w", err)
	}
	return out, nil
}

func hash(algorithm, input string) (string, error) {
	switch strings.ToLower(algorithm) {
	case "sha256":
		h := sha256.Sum256([]byte(input))
		return hex.EncodeToString(h[:]), nil
	case "sha1":
		h := sha1.Sum([]byte(input)) //nolint:gosec
		return hex.EncodeToString(h[:]), nil
	case "md5":
		h := md5.Sum([]byte(input)) //nolint:gosec
		return hex.EncodeToString(h[:]), nil
	default:
		return "", fmt.Errorf("enricher: unknown algorithm %q", algorithm)
	}
}

func parsePath(p string) []string {
	p = strings.TrimPrefix(p, "$.")
	p = strings.TrimPrefix(p, "$")
	return strings.Split(p, ".")
}

func getPath(doc map[string]interface{}, parts []string) (interface{}, bool) {
	var current interface{} = doc
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

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
