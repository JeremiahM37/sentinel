// Package libraries defines the LibraryChecker interface and provides
// implementations for Jellyfin, Audiobookshelf, Kavita, Sonarr, and Radarr.
package libraries

import (
	"context"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// LibraryChecker is the interface for library verification plugins.
// Each checker must return a VerificationProof with concrete evidence
// (file paths, durations, page counts) -- not fuzzy title matching alone.
type LibraryChecker interface {
	// Name returns a unique identifier for this library checker.
	Name() string

	// SupportedTypes returns the media types this checker can verify.
	SupportedTypes() []models.MediaType

	// IsAvailable returns true if this library is configured and reachable.
	IsAvailable(ctx context.Context, cfg *config.Config) bool

	// Verify checks whether the job's content exists in the library.
	Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof
}

// SupportsMediaType checks if a checker supports a given media type.
func SupportsMediaType(c LibraryChecker, mt models.MediaType) bool {
	for _, t := range c.SupportedTypes() {
		if t == mt {
			return true
		}
	}
	return false
}
