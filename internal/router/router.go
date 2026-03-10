package router

import (
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Router resolves a target Kafka topic from a message's headers.
type Router struct {
	headerKey string
}

// New creates a Router that looks for the given headerKey in each message.
func New(headerKey string) *Router {
	return &Router{headerKey: headerKey}
}

// Resolve inspects the provided headers and returns the value of the configured
// header key as the target topic. Returns an error if the key is absent or empty.
func (r *Router) Resolve(headers []kgo.RecordHeader) (string, error) {
	for _, h := range headers {
		if h.Key == r.headerKey {
			value := string(h.Value)
			if value == "" {
				return "", fmt.Errorf("header %q is present but has an empty value", r.headerKey)
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("header %q not found in message", r.headerKey)
}
