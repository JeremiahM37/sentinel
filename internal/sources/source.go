// Package sources defines the interface for download source plugins and
// provides the Jellyseerr, Prowlarr, and qBittorrent implementations.
package sources

import (
	"context"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// Source is the interface all download source plugins must implement.
type Source interface {
	// Name returns a unique identifier for this source.
	Name() string

	// SupportedTypes returns the media types this source can handle.
	SupportedTypes() []models.MediaType

	// IsAvailable checks if the source is configured and reachable.
	IsAvailable(ctx context.Context, cfg *config.Config) bool

	// SearchAndDownload attempts to find and start a download for the given job.
	// Returns a SourceAttempt recording what happened.
	SearchAndDownload(ctx context.Context, job *models.Job, cfg *config.Config) models.SourceAttempt
}

// SupportsMediaType checks if a source supports a given media type.
func SupportsMediaType(s Source, mt models.MediaType) bool {
	for _, t := range s.SupportedTypes() {
		if t == mt {
			return true
		}
	}
	return false
}
