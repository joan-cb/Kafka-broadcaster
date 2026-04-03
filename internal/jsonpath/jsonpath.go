// Package jsonpath provides minimal JSON object path parsing and lookup,
// matching the "$.a.b" style used by transformation rules.
package jsonpath

import "strings"

// ParsePath strips a leading "$." or "$" prefix and splits on ".".
// "$.user.name" → ["user", "name"].
func ParsePath(p string) []string {
	p = strings.TrimPrefix(p, "$.")
	p = strings.TrimPrefix(p, "$")
	return strings.Split(p, ".")
}

// Get traverses nested map[string]interface{} following parts and returns the
// value and whether the path exists (including null leaf values).
func Get(doc map[string]interface{}, parts []string) (interface{}, bool) {
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
