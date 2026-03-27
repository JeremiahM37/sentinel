// Package notify provides notification interfaces and implementations for
// Discord webhooks and generic webhooks.
package notify

import (
	"github.com/JeremiahM37/sentinel/internal/models"
)

// Notifier is the interface for all notification channels.
type Notifier interface {
	// IsConfigured returns true if this notifier has the required config.
	IsConfigured() bool

	// Notify sends a notification for a job event.
	Notify(job *models.Job, event string, details map[string]any) error
}
