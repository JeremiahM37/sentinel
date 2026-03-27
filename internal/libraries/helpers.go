package libraries

import (
	"time"

	"github.com/JeremiahM37/sentinel/internal/models"
)

// Helper functions shared across library checkers.

func errorProof(library, msg string, at time.Time) models.VerificationProof {
	return models.VerificationProof{
		Library:      library,
		Status:       models.VerificationError,
		ErrorMessage: msg,
		CheckedAt:    at,
	}
}

func notFoundProof(library string, at time.Time) models.VerificationProof {
	return models.VerificationProof{
		Library:   library,
		Status:    models.VerificationNotFound,
		CheckedAt: at,
	}
}

func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getNum(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
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

func getBool(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return nil
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
