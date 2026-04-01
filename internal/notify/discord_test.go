package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

func TestDiscordNotifierIsConfigured(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: "https://discord.com/api/webhooks/123/abc"})
		if !n.IsConfigured() {
			t.Error("expected configured")
		}
	})

	t.Run("not configured", func(t *testing.T) {
		n := NewDiscordNotifier(&config.Config{})
		if n.IsConfigured() {
			t.Error("expected not configured")
		}
	})
}

func TestDiscordNotifySuccess(t *testing.T) {
	var receivedBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: ts.URL})

	year := 2024
	job := &models.Job{
		ID:        "discord-test-id-1234567890",
		Title:     "Test Movie",
		MediaType: models.MediaTypeMovie,
		Status:    models.JobStatusCompleted,
		Year:      &year,
	}

	err := n.Notify(job, "completed", map[string]any{
		"library":   "jellyfin",
		"file_path": "/media/movies/Test.mkv",
	})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Verify payload structure
	embeds, ok := receivedBody["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("missing embeds")
	}

	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatal("invalid embed")
	}

	title, _ := embed["title"].(string)
	if title != "Sentinel: Test Movie" {
		t.Errorf("embed title = %q", title)
	}

	desc, _ := embed["description"].(string)
	if desc != "Event: **completed**" {
		t.Errorf("embed description = %q", desc)
	}

	// Check color is green for completed
	color, _ := embed["color"].(float64)
	if int(color) != 0x2ECC71 {
		t.Errorf("color = %x, want %x", int(color), 0x2ECC71)
	}

	// Footer should have short ID
	footer, _ := embed["footer"].(map[string]any)
	footerText, _ := footer["text"].(string)
	if footerText != "Job discord-" {
		t.Errorf("footer = %q", footerText)
	}
}

func TestDiscordNotifyStatusColors(t *testing.T) {
	tests := []struct {
		status models.JobStatus
		color  int
	}{
		{models.JobStatusPending, 0x95A5A6},
		{models.JobStatusSearching, 0x3498DB},
		{models.JobStatusDownloading, 0xF39C12},
		{models.JobStatusVerifying, 0x9B59B6},
		{models.JobStatusCompleted, 0x2ECC71},
		{models.JobStatusFailed, 0xE74C3C},
		{models.JobStatusCancelled, 0x7F8C8D},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			var receivedBody map[string]any
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				json.Unmarshal(body, &receivedBody)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()

			n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: ts.URL})
			job := &models.Job{
				ID:        "color-test",
				Title:     "Test",
				MediaType: models.MediaTypeMovie,
				Status:    tc.status,
			}

			n.Notify(job, "test", nil)

			embeds, _ := receivedBody["embeds"].([]any)
			embed, _ := embeds[0].(map[string]any)
			color, _ := embed["color"].(float64)
			if int(color) != tc.color {
				t.Errorf("status %q: color = %x, want %x", tc.status, int(color), tc.color)
			}
		})
	}
}

func TestDiscordNotifyNotConfigured(t *testing.T) {
	n := NewDiscordNotifier(&config.Config{})
	job := &models.Job{
		ID: "test", Title: "Test", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
	}

	// Should return nil (no-op) when not configured
	err := n.Notify(job, "test", nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestDiscordNotifyHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: ts.URL})
	job := &models.Job{
		ID: "error-test", Title: "Test", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
	}

	err := n.Notify(job, "test", nil)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestDiscordFieldsLimit(t *testing.T) {
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: ts.URL})

	// Create details with more than 25 fields to test truncation
	details := make(map[string]any)
	for i := 0; i < 30; i++ {
		details[string(rune('a'+i%26))+string(rune('0'+i/26))] = "value"
	}

	job := &models.Job{
		ID:        "field-limit-test",
		Title:     "Many Fields",
		MediaType: models.MediaTypeMovie,
		Status:    models.JobStatusCompleted,
	}

	n.Notify(job, "test", details)

	embeds, _ := receivedBody["embeds"].([]any)
	embed, _ := embeds[0].(map[string]any)
	fields, _ := embed["fields"].([]any)

	if len(fields) > 25 {
		t.Errorf("fields count = %d, max should be 25", len(fields))
	}
}

func TestDiscordLongValueTruncation(t *testing.T) {
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	n := NewDiscordNotifier(&config.Config{DiscordWebhookURL: ts.URL})

	longValue := ""
	for i := 0; i < 250; i++ {
		longValue += "x"
	}

	job := &models.Job{
		ID: "trunc-test", Title: "Test", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
	}

	n.Notify(job, "test", map[string]any{"long_key": longValue})

	embeds, _ := receivedBody["embeds"].([]any)
	embed, _ := embeds[0].(map[string]any)
	fields, _ := embed["fields"].([]any)

	for _, f := range fields {
		field, _ := f.(map[string]any)
		name, _ := field["name"].(string)
		if name == "long_key" {
			val, _ := field["value"].(string)
			if len(val) > 200 {
				t.Errorf("value not truncated, len = %d", len(val))
			}
			return
		}
	}
}
