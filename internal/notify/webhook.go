package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// WebhookNotifier sends JSON payloads to a generic webhook URL on job events.
type WebhookNotifier struct {
	cfg *config.Config
}

// NewWebhookNotifier creates a new WebhookNotifier.
func NewWebhookNotifier(cfg *config.Config) *WebhookNotifier {
	return &WebhookNotifier{cfg: cfg}
}

func (w *WebhookNotifier) IsConfigured() bool {
	return w.cfg.IsConfigured("webhook")
}

func (w *WebhookNotifier) Notify(job *models.Job, event string, details map[string]any) error {
	if !w.IsConfigured() {
		return nil
	}

	payload := map[string]any{
		"event":        event,
		"job_id":       job.ID,
		"title":        job.Title,
		"media_type":   string(job.MediaType),
		"status":       string(job.Status),
		"verify_count": job.VerifyCount,
	}
	if details != nil {
		payload["details"] = details
	}

	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(w.cfg.NotificationWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("Webhook send failed", "job_id", job.ID, "error", err)
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Warn("Webhook returned error", "job_id", job.ID, "event", event, "status", resp.StatusCode)
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
