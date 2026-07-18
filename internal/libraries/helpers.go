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

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
