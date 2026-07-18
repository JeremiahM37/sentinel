package jsonmap

import "testing"

func TestStr(t *testing.T) {
	m := map[string]any{"name": "test", "count": 42}
	if Str(m, "name") != "test" {
		t.Error("Str failed for existing key")
	}
	if Str(m, "missing") != "" {
		t.Error("Str should return empty for missing key")
	}
	if Str(m, "count") != "" {
		t.Error("Str should return empty for non-string value")
	}
	if Str(nil, "key") != "" {
		t.Error("Str should return empty for nil map")
	}
}

func TestStrOr(t *testing.T) {
	m := map[string]any{"name": "test", "count": 42}
	if StrOr(m, "name", "fb") != "test" {
		t.Error("StrOr failed for existing key")
	}
	if StrOr(m, "missing", "fb") != "fb" {
		t.Error("StrOr should return fallback for missing key")
	}
	if StrOr(m, "count", "fb") != "fb" {
		t.Error("StrOr should return fallback for non-string value")
	}
	if StrOr(nil, "key", "fb") != "fb" {
		t.Error("StrOr should return fallback for nil map")
	}
}

func TestNum(t *testing.T) {
	m := map[string]any{"f": 3.14, "i": 42, "i64": int64(100), "s": "abc"}
	if Num(m, "f") != 3.14 {
		t.Error("Num failed for float64")
	}
	if Num(m, "i") != 42.0 {
		t.Error("Num failed for int")
	}
	if Num(m, "i64") != 100.0 {
		t.Error("Num failed for int64")
	}
	if Num(m, "s") != 0 {
		t.Error("Num should return 0 for string")
	}
	if Num(m, "missing") != 0 {
		t.Error("Num should return 0 for missing key")
	}
	if Num(nil, "key") != 0 {
		t.Error("Num should return 0 for nil map")
	}
}

func TestBool(t *testing.T) {
	m := map[string]any{"yes": true, "no": false, "str": "true"}
	if !Bool(m, "yes") {
		t.Error("Bool failed for true")
	}
	if Bool(m, "no") {
		t.Error("Bool failed for false")
	}
	if Bool(m, "str") {
		t.Error("Bool should return false for string 'true'")
	}
	if Bool(m, "missing") {
		t.Error("Bool should return false for missing key")
	}
	if Bool(nil, "key") {
		t.Error("Bool should return false for nil map")
	}
}

func TestMap(t *testing.T) {
	inner := map[string]any{"nested": true}
	m := map[string]any{"sub": inner, "str": "abc"}
	got := Map(m, "sub")
	if got == nil || !got["nested"].(bool) {
		t.Error("Map failed for existing nested map")
	}
	if Map(m, "str") != nil {
		t.Error("Map should return nil for non-map value")
	}
	if Map(m, "missing") != nil {
		t.Error("Map should return nil for missing key")
	}
	if Map(nil, "key") != nil {
		t.Error("Map should return nil for nil map")
	}
}
