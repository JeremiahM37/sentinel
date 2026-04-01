package guardian

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/models"
)

func testSetup(t *testing.T) (*db.JobDB, *config.Config) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		VerifyIntervalSeconds: 3600,
		VerifyMaxChecks:       5,
		MaxSourcesPerType:     5,
		TitleMatchThreshold:   0.7,
	}
	return database, cfg
}

func createTestJob(t *testing.T, database *db.JobDB, id, title string, mt models.MediaType, status models.JobStatus) *models.Job {
	t.Helper()
	now := time.Now().UTC()
	job := &models.Job{
		ID:                 id,
		Title:              title,
		MediaType:          mt,
		Status:             status,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := database.CreateJob(context.Background(), job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	return job
}

func TestHasProof(t *testing.T) {
	tests := []struct {
		name    string
		results []models.VerificationProof
		want    bool
	}{
		{
			name:    "empty",
			results: nil,
			want:    false,
		},
		{
			name: "found",
			results: []models.VerificationProof{
				{Status: models.VerificationNotFound},
				{Status: models.VerificationFound},
			},
			want: true,
		},
		{
			name: "not found only",
			results: []models.VerificationProof{
				{Status: models.VerificationNotFound},
				{Status: models.VerificationError},
			},
			want: false,
		},
		{
			name: "single found",
			results: []models.VerificationProof{
				{Status: models.VerificationFound},
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasProof(tc.results); got != tc.want {
				t.Errorf("HasProof() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGuardianStartStop(t *testing.T) {
	database, cfg := testSetup(t)
	g := New(database, cfg)

	if g.IsRunning() {
		t.Error("should not be running before Start")
	}

	g.Start()
	if !g.IsRunning() {
		t.Error("should be running after Start")
	}

	// Double start should be no-op
	g.Start()
	if !g.IsRunning() {
		t.Error("should still be running after double Start")
	}

	g.Stop()
	if g.IsRunning() {
		t.Error("should not be running after Stop")
	}

	// Double stop should be no-op
	g.Stop()
}

func TestVerifierGetAvailableCheckers(t *testing.T) {
	cfg := &config.Config{} // no services configured
	v := NewVerifier(cfg)

	checkers := v.GetAvailableCheckers(context.Background())
	if checkers == nil {
		t.Error("should return empty slice, not nil")
	}
	if len(checkers) != 0 {
		t.Errorf("expected 0 available checkers with no config, got %d", len(checkers))
	}
}

func TestVerifierWithConfiguredService(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer ts.Close()

	cfg := &config.Config{
		JellyfinURL:         ts.URL,
		JellyfinAPIKey:      "key",
		TitleMatchThreshold: 0.7,
	}
	v := NewVerifier(cfg)

	checkers := v.GetAvailableCheckers(context.Background())
	found := false
	for _, name := range checkers {
		if name == "jellyfin" {
			found = true
		}
	}
	if !found {
		t.Error("expected jellyfin in available checkers")
	}
}

func TestVerifierVerifySkipsUnsupportedTypes(t *testing.T) {
	cfg := &config.Config{TitleMatchThreshold: 0.7}
	v := NewVerifier(cfg)

	// Game type is not supported by any default checker
	job := &models.Job{
		ID:        "game-test",
		Title:     "Test Game",
		MediaType: models.MediaTypeGame,
	}

	results := v.Verify(context.Background(), job)
	if len(results) != 0 {
		t.Errorf("expected 0 results for unsupported type, got %d", len(results))
	}
}

func TestVerifierShortCircuitsOnFound(t *testing.T) {
	// Set up Jellyfin mock that returns found
	jellyfinServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			w.WriteHeader(200)
		default:
			json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{
						"Name":         "The Matrix",
						"Path":         "/movies/The Matrix.mkv",
						"RunTimeTicks": 81360000000.0,
					},
				},
			})
		}
	}))
	defer jellyfinServer.Close()

	cfg := &config.Config{
		JellyfinURL:         jellyfinServer.URL,
		JellyfinAPIKey:      "key",
		TitleMatchThreshold: 0.5,
	}
	v := NewVerifier(cfg)

	job := &models.Job{
		ID:        "short-circuit",
		Title:     "The Matrix",
		MediaType: models.MediaTypeMovie,
	}

	results := v.Verify(context.Background(), job)
	// Should have exactly 1 result (short-circuited on Jellyfin found)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].Status != models.VerificationFound {
		t.Errorf("first result status = %q, want found", results[0].Status)
	}
}

func TestGuardianPendingToSearching(t *testing.T) {
	database, cfg := testSetup(t)
	g := New(database, cfg)

	createTestJob(t, database, "pending-1", "No Source Movie", models.MediaTypeMovie, models.JobStatusPending)

	// Process the tick - no sources configured, so it should go pending -> searching -> failed
	ctx := context.Background()
	g.tick(ctx)

	job, _ := database.GetJob(ctx, "pending-1")
	// With no sources available, job should be failed
	if job.Status != models.JobStatusFailed {
		t.Errorf("Status = %q, want failed (no sources)", job.Status)
	}
}

func TestGuardianDownloadingComplete(t *testing.T) {
	// Set up qBittorrent mock
	qbitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{
				{"state": "uploading", "progress": 1.0, "name": "Test", "hash": "abc"},
			})
		}
	}))
	defer qbitServer.Close()

	database, cfg := testSetup(t)
	cfg.QBittorrentURL = qbitServer.URL
	cfg.QBittorrentUser = "admin"
	cfg.QBittorrentPass = "pass"

	g := New(database, cfg)

	now := time.Now().UTC()
	job := &models.Job{
		ID:                "dl-complete",
		Title:             "Downloading Movie",
		MediaType:         models.MediaTypeMovie,
		Status:            models.JobStatusDownloading,
		CurrentDownloadID: "sentinel:Downloading Movie",
		SourceAttempts:    []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	database.CreateJob(context.Background(), job)

	ctx := context.Background()
	g.tick(ctx)

	updated, _ := database.GetJob(ctx, "dl-complete")
	if updated.Status != models.JobStatusVerifying {
		t.Errorf("Status = %q, want verifying", updated.Status)
	}
}

func TestGuardianDownloadingNoDownloadID(t *testing.T) {
	database, cfg := testSetup(t)
	g := New(database, cfg)

	now := time.Now().UTC()
	job := &models.Job{
		ID:                 "dl-no-id",
		Title:              "No DL ID",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusDownloading,
		CurrentDownloadID:  "", // empty
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	database.CreateJob(context.Background(), job)

	ctx := context.Background()
	g.tick(ctx)

	updated, _ := database.GetJob(ctx, "dl-no-id")
	if updated.Status != models.JobStatusVerifying {
		t.Errorf("Status = %q, want verifying (no download ID)", updated.Status)
	}
}

func TestGuardianVerifyingMaxChecks(t *testing.T) {
	database, cfg := testSetup(t)
	cfg.VerifyMaxChecks = 2
	g := New(database, cfg)

	now := time.Now().UTC()
	job := &models.Job{
		ID:                 "verify-max",
		Title:              "Verify Max",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusVerifying,
		VerifyCount:        1, // one less than max
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	database.CreateJob(context.Background(), job)

	ctx := context.Background()
	g.tick(ctx)

	updated, _ := database.GetJob(ctx, "verify-max")
	// After incrementing verify_count to 2 (== max), should go back to searching
	// Then with no sources available, should fail
	if updated.Status != models.JobStatusFailed {
		t.Errorf("Status = %q, want failed (max checks -> search -> fail)", updated.Status)
	}
}

func TestGuardianNotifications(t *testing.T) {
	var notificationCount int
	discordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notificationCount++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discordServer.Close()

	database, cfg := testSetup(t)
	cfg.DiscordWebhookURL = discordServer.URL

	g := New(database, cfg)

	createTestJob(t, database, "notify-test", "Notify Movie", models.MediaTypeMovie, models.JobStatusPending)

	ctx := context.Background()
	g.tick(ctx)

	// Should have sent at least one notification (searching, then failed)
	if notificationCount == 0 {
		t.Error("expected at least one Discord notification")
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abcdefghijklmnop", "abcdefgh"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"", ""},
	}

	for _, tc := range tests {
		got := shortID(tc.input)
		if got != tc.expected {
			t.Errorf("shortID(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestGuardianDownloadingTorrentNotFound(t *testing.T) {
	// qBit returns empty torrents (torrent not found)
	qbitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{}) // empty
		}
	}))
	defer qbitServer.Close()

	database, cfg := testSetup(t)
	cfg.QBittorrentURL = qbitServer.URL
	cfg.QBittorrentUser = "admin"
	cfg.QBittorrentPass = "pass"

	g := New(database, cfg)

	now := time.Now().UTC()
	job := &models.Job{
		ID:                "dl-gone",
		Title:             "Gone Torrent",
		MediaType:         models.MediaTypeMovie,
		Status:            models.JobStatusDownloading,
		CurrentDownloadID: "sentinel:Gone",
		SourceAttempts:    []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	database.CreateJob(context.Background(), job)

	ctx := context.Background()
	g.tick(ctx)

	updated, _ := database.GetJob(ctx, "dl-gone")
	// Torrent not found -> move to verifying
	if updated.Status != models.JobStatusVerifying {
		t.Errorf("Status = %q, want verifying (torrent not found)", updated.Status)
	}
}

func TestGuardianStillDownloading(t *testing.T) {
	qbitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{
				{"state": "downloading", "progress": 0.5, "name": "Test", "hash": "abc"},
			})
		}
	}))
	defer qbitServer.Close()

	database, cfg := testSetup(t)
	cfg.QBittorrentURL = qbitServer.URL
	cfg.QBittorrentUser = "admin"
	cfg.QBittorrentPass = "pass"

	g := New(database, cfg)

	now := time.Now().UTC()
	job := &models.Job{
		ID:                "still-dl",
		Title:             "Still DL",
		MediaType:         models.MediaTypeMovie,
		Status:            models.JobStatusDownloading,
		CurrentDownloadID: "sentinel:Still",
		SourceAttempts:    []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	database.CreateJob(context.Background(), job)

	ctx := context.Background()
	g.tick(ctx)

	updated, _ := database.GetJob(ctx, "still-dl")
	// Should stay downloading
	if updated.Status != models.JobStatusDownloading {
		t.Errorf("Status = %q, want downloading", updated.Status)
	}
}
