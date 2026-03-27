package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/sentinel/internal/api"
	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
	"github.com/JeremiahM37/sentinel/internal/models"
	"github.com/JeremiahM37/sentinel/internal/titleutil"
)

func setupTestServer(t *testing.T) (*httptest.Server, *db.JobDB) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Port:                  9200,
		DBPath:                dbPath,
		LogLevel:              "ERROR",
		VerifyIntervalSeconds: 3600, // don't tick during tests
		VerifyMaxChecks:       30,
		MaxSourcesPerType:     5,
		TitleMatchThreshold:   0.7,
	}

	database, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	g := guardian.New(database, cfg)
	// Don't start guardian loop for tests

	router := api.NewRouter(database, g)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)

	return ts, database
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var health models.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", health.Status)
	}
}

func TestCreateAndGetJob(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Create
	body, _ := json.Marshal(models.JobCreateRequest{
		Title:     "The Matrix",
		MediaType: models.MediaTypeMovie,
	})

	resp, err := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/jobs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created models.JobResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Title != "The Matrix" {
		t.Errorf("expected title 'The Matrix', got '%s'", created.Title)
	}
	if created.Status != models.JobStatusPending {
		t.Errorf("expected status 'pending', got '%s'", created.Status)
	}

	// Get
	resp2, err := http.Get(ts.URL + "/api/jobs/" + created.ID)
	if err != nil {
		t.Fatalf("GET /api/jobs/{id}: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}

	var fetched models.JobResponse
	json.NewDecoder(resp2.Body).Decode(&fetched)
	if fetched.ID != created.ID {
		t.Errorf("ID mismatch: %s vs %s", fetched.ID, created.ID)
	}
}

func TestListJobs(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Create two jobs
	for _, title := range []string{"Movie A", "Movie B"} {
		body, _ := json.Marshal(models.JobCreateRequest{
			Title:     title,
			MediaType: models.MediaTypeMovie,
		})
		resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
		resp.Body.Close()
	}

	resp, err := http.Get(ts.URL + "/api/jobs")
	if err != nil {
		t.Fatalf("GET /api/jobs: %v", err)
	}
	defer resp.Body.Close()

	var list models.JobListResponse
	json.NewDecoder(resp.Body).Decode(&list)
	if list.Total != 2 {
		t.Errorf("expected 2 jobs, got %d", list.Total)
	}
}

func TestCancelJob(t *testing.T) {
	ts, _ := setupTestServer(t)

	body, _ := json.Marshal(models.JobCreateRequest{
		Title:     "Cancel Me",
		MediaType: models.MediaTypeTV,
	})
	resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	var created models.JobResponse
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Cancel
	cancelResp, err := http.Post(ts.URL+"/api/jobs/"+created.ID+"/cancel", "", nil)
	if err != nil {
		t.Fatalf("POST cancel: %v", err)
	}
	defer cancelResp.Body.Close()

	if cancelResp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", cancelResp.StatusCode)
	}

	var cancelled models.JobResponse
	json.NewDecoder(cancelResp.Body).Decode(&cancelled)
	if cancelled.Status != models.JobStatusCancelled {
		t.Errorf("expected 'cancelled', got '%s'", cancelled.Status)
	}
}

func TestDeleteJob(t *testing.T) {
	ts, _ := setupTestServer(t)

	body, _ := json.Marshal(models.JobCreateRequest{
		Title:     "Delete Me",
		MediaType: models.MediaTypeEbook,
	})
	resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	var created models.JobResponse
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/jobs/"+created.ID, nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()

	if delResp.StatusCode != 204 {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}

	// Verify gone
	resp2, _ := http.Get(ts.URL + "/api/jobs/" + created.ID)
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Errorf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

func TestGetStats(t *testing.T) {
	ts, _ := setupTestServer(t)

	body, _ := json.Marshal(models.JobCreateRequest{
		Title:     "Stats Test",
		MediaType: models.MediaTypeMovie,
	})
	resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	statsResp, err := http.Get(ts.URL + "/api/stats")
	if err != nil {
		t.Fatalf("GET /api/stats: %v", err)
	}
	defer statsResp.Body.Close()

	var stats models.StatsResponse
	json.NewDecoder(statsResp.Body).Decode(&stats)
	if stats.TotalJobs != 1 {
		t.Errorf("expected 1 total job, got %d", stats.TotalJobs)
	}
	if stats.Pending != 1 {
		t.Errorf("expected 1 pending, got %d", stats.Pending)
	}
}

func TestNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, _ := http.Get(ts.URL + "/api/jobs/nonexistent-id")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestInvalidCreateRequest(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Missing title
	body, _ := json.Marshal(map[string]string{
		"media_type": "movie",
	})
	resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for missing title, got %d", resp.StatusCode)
	}

	// Invalid media_type
	body2, _ := json.Marshal(map[string]string{
		"title":      "Test",
		"media_type": "invalid",
	})
	resp2, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body2))
	resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Errorf("expected 400 for invalid media_type, got %d", resp2.StatusCode)
	}
}

func TestTitleMatchScore(t *testing.T) {
	tests := []struct {
		a, b     string
		minScore float64
	}{
		{"The Matrix", "Matrix", 0.9},
		{"Spider-Man: No Way Home", "Spider Man No Way Home", 0.9},
		{"Completely Different", "Nothing Similar Here", 0.0},
		{"", "Something", 0.0},
	}

	for _, tc := range tests {
		score := titleutil.TitleMatchScore(tc.a, tc.b)
		if score < tc.minScore {
			t.Errorf("TitleMatchScore(%q, %q) = %.2f, want >= %.2f",
				tc.a, tc.b, score, tc.minScore)
		}
	}
}

func TestDBPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	// Create and insert
	database, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ctx := context.Background()
	job := &models.Job{
		ID:                 "test-persist-123",
		Title:              "Persistent Job",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusPending,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          mustParseTime("2026-01-01T00:00:00Z"),
		UpdatedAt:          mustParseTime("2026-01-01T00:00:00Z"),
	}

	if err := database.CreateJob(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	database.Close()

	// Reopen and fetch
	database2, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	defer database2.Close()

	fetched, err := database2.GetJob(ctx, "test-persist-123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected job, got nil")
	}
	if fetched.Title != "Persistent Job" {
		t.Errorf("expected 'Persistent Job', got '%s'", fetched.Title)
	}
}

func TestRetryJob(t *testing.T) {
	ts, database := setupTestServer(t)
	ctx := context.Background()

	// Create a job and mark it failed directly in DB
	body, _ := json.Marshal(models.JobCreateRequest{
		Title:     "Retry Me",
		MediaType: models.MediaTypeMovie,
	})
	resp, _ := http.Post(ts.URL+"/api/jobs", "application/json", bytes.NewReader(body))
	var created models.JobResponse
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Mark as failed
	job, _ := database.GetJob(ctx, created.ID)
	job.Status = models.JobStatusFailed
	database.UpdateJob(ctx, job)

	// Retry
	retryResp, err := http.Post(ts.URL+"/api/jobs/"+created.ID+"/retry", "", nil)
	if err != nil {
		t.Fatalf("POST retry: %v", err)
	}
	defer retryResp.Body.Close()

	if retryResp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", retryResp.StatusCode)
	}

	var retried models.JobResponse
	json.NewDecoder(retryResp.Body).Decode(&retried)
	if retried.Status != models.JobStatusPending {
		t.Errorf("expected 'pending', got '%s'", retried.Status)
	}
}

// Suppress unused import warnings
var _ = os.TempDir

func mustParseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
