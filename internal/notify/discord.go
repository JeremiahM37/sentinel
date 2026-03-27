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

// Discord embed colors by status.
var statusColors = map[models.JobStatus]int{
	models.JobStatusPending:     0x95A5A6,
	models.JobStatusSearching:   0x3498DB,
	models.JobStatusDownloading: 0xF39C12,
	models.JobStatusVerifying:   0x9B59B6,
	models.JobStatusCompleted:   0x2ECC71,
	models.JobStatusFailed:      0xE74C3C,
	models.JobStatusCancelled:   0x7F8C8D,
}

// DiscordNotifier sends rich embed notifications to a Discord webhook.
type DiscordNotifier struct {
	cfg *config.Config
}

// NewDiscordNotifier creates a new DiscordNotifier.
func NewDiscordNotifier(cfg *config.Config) *DiscordNotifier {
	return &DiscordNotifier{cfg: cfg}
}

func (d *DiscordNotifier) IsConfigured() bool {
	return d.cfg.IsConfigured("discord")
}

func (d *DiscordNotifier) Notify(job *models.Job, event string, details map[string]any) error {
	if !d.IsConfigured() {
		return nil
	}

	color, ok := statusColors[job.Status]
	if !ok {
		color = 0x95A5A6
	}

	fields := []map[string]any{
		{"name": "Media Type", "value": string(job.MediaType), "inline": true},
		{"name": "Status", "value": string(job.Status), "inline": true},
		{"name": "Checks", "value": fmt.Sprintf("%d", job.VerifyCount), "inline": true},
	}

	if job.Year != nil {
		fields = append(fields, map[string]any{
			"name": "Year", "value": fmt.Sprintf("%d", *job.Year), "inline": true,
		})
	}

	if details != nil {
		for key, value := range details {
			valStr := fmt.Sprintf("%v", value)
			if len(valStr) > 200 {
				valStr = valStr[:197] + "..."
			}
			fields = append(fields, map[string]any{
				"name": key, "value": valStr, "inline": false,
			})
		}
	}

	// Limit to 25 fields (Discord max)
	if len(fields) > 25 {
		fields = fields[:25]
	}

	jobIDShort := job.ID
	if len(jobIDShort) > 8 {
		jobIDShort = jobIDShort[:8]
	}

	embed := map[string]any{
		"title":       fmt.Sprintf("Sentinel: %s", job.Title),
		"description": fmt.Sprintf("Event: **%s**", event),
		"color":       color,
		"fields":      fields,
		"footer":      map[string]string{"text": fmt.Sprintf("Job %s", jobIDShort)},
	}

	payload, _ := json.Marshal(map[string]any{
		"embeds": []any{embed},
	})

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(d.cfg.DiscordWebhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		slog.Warn("Discord notification failed", "job_id", job.ID, "error", err)
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Warn("Discord webhook returned error", "job_id", job.ID, "status", resp.StatusCode)
		return fmt.Errorf("discord webhook returned %d", resp.StatusCode)
	}
	return nil
}
