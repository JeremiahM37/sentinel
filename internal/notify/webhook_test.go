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

func TestWebhookNotifierIsConfigured(t *testing.T) {
	t.Run("configured", func(t *testing.T) {
		n := NewWebhookNotifier(&config.Config{NotificationWebhookURL: "http://localhost:9999/hook"})
		if !n.IsConfigured() {
			t.Error("expected configured")
		}
	})

	t.Run("not configured", func(t *testing.T) {
		n := NewWebhookNotifier(&config.Config{})
		if n.IsConfigured() {
			t.Error("expected not configured")
		}
	})
}

func TestWebhookNotifySuccess(t *testing.T) {
	var receivedBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	n := NewWebhookNotifier(&config.Config{NotificationWebhookURL: ts.URL})
	job := &models.Job{
		ID:          "webhook-test",
		Title:       "Test Movie",
		MediaType:   models.MediaTypeMovie,
		Status:      models.JobStatusCompleted,
		VerifyCount: 3,
	}

	err := n.Notify(job, "completed", map[string]any{"library": "jellyfin"})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if receivedBody["event"] != "completed" {
		t.Errorf("event = %v", receivedBody["event"])
	}
	if receivedBody["job_id"] != "webhook-test" {
		t.Errorf("job_id = %v", receivedBody["job_id"])
	}
	if receivedBody["title"] != "Test Movie" {
		t.Errorf("title = %v", receivedBody["title"])
	}
	if receivedBody["media_type"] != "movie" {
		t.Errorf("media_type = %v", receivedBody["media_type"])
	}
	if receivedBody["status"] != "completed" {
		t.Errorf("status = %v", receivedBody["status"])
	}

	details, ok := receivedBody["details"].(map[string]any)
	if !ok {
		t.Fatal("missing details")
	}
	if details["library"] != "jellyfin" {
		t.Errorf("details.library = %v", details["library"])
	}
}

func TestWebhookNotifyNilDetails(t *testing.T) {
	var receivedBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	n := NewWebhookNotifier(&config.Config{NotificationWebhookURL: ts.URL})
	job := &models.Job{
		ID: "nil-details", Title: "Test", MediaType: models.MediaTypeTV, Status: models.JobStatusSearching,
	}

	err := n.Notify(job, "searching", nil)
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	if _, exists := receivedBody["details"]; exists {
		t.Error("details should not be present when nil")
	}
}

func TestWebhookNotifyNotConfigured(t *testing.T) {
	n := NewWebhookNotifier(&config.Config{})
	job := &models.Job{
		ID: "test", Title: "Test", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
	}
	err := n.Notify(job, "test", nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestWebhookNotifyHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	n := NewWebhookNotifier(&config.Config{NotificationWebhookURL: ts.URL})
	job := &models.Job{
		ID: "error-test", Title: "Test", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
	}
	err := n.Notify(job, "test", nil)
	if err == nil {
		t.Error("expected error for 502 response")
	}
}
