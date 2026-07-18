// Package jsonmap provides typed accessors for map[string]any values decoded
// from JSON API responses. All accessors are nil-safe and return the zero
// value (or the given fallback) when the key is missing or the wrong type.
package jsonmap

// Str returns the string at key, or "" if missing or not a string.
func Str(m map[string]any, key string) string {
	return StrOr(m, key, "")
}

// StrOr returns the string at key, or fallback if missing or not a string.
func StrOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

// Num returns the numeric value at key as float64, or 0 if missing or not numeric.
func Num(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}

// Bool returns the bool at key, or false if missing or not a bool.
func Bool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Map returns the nested map at key, or nil if missing or not a map.
func Map(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}
