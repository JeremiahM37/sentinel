package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
	"github.com/JeremiahM37/sentinel/internal/models"
)

func testRouter(t *testing.T) (http.Handler, *db.JobDB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		VerifyIntervalSeconds: 3600,
		VerifyMaxChecks:       30,
		MaxSourcesPerType:     5,
		TitleMatchThreshold:   0.7,
	}

	g := guardian.New(database, cfg)
	router := NewRouter(database, g)
	return router, database
}

func TestHealthEndpoint(t *testing.T) {
	router, _ := testRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp models.HealthResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Status != "ok" {
		t.Errorf("status = %q", resp.Status)
	}
	if resp.Version == "" {
		t.Error("version should not be empty")
	}
	if resp.ConfiguredServices == nil {
		t.Error("configured_services should not be nil")
	}
}

func TestCreateJobEndpoint(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		router, _ := testRouter(t)
		body, _ := json.Marshal(models.JobCreateRequest{
			Title:     "Inception",
			MediaType: models.MediaTypeMovie,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 201 {
			t.Errorf("status = %d, want 201", rr.Code)
		}

		var resp models.JobResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Title != "Inception" {
			t.Errorf("title = %q", resp.Title)
		}
		if resp.Status != models.JobStatusPending {
			t.Errorf("status = %q", resp.Status)
		}
		if resp.ID == "" {
			t.Error("ID should not be empty")
		}
	})

	t.Run("with optional fields", func(t *testing.T) {
		router, _ := testRouter(t)
		year := 2010
		body, _ := json.Marshal(models.JobCreateRequest{
			Title:     "Inception",
			MediaType: models.MediaTypeMovie,
			Year:      &year,
			ImdbID:    "tt1375666",
		})

		req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 201 {
			t.Errorf("status = %d", rr.Code)
		}

		var resp models.JobResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Year == nil || *resp.Year != 2010 {
			t.Errorf("Year = %v", resp.Year)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		router, _ := testRouter(t)
		body, _ := json.Marshal(map[string]string{"media_type": "movie"})

		req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("invalid media type", func(t *testing.T) {
		router, _ := testRouter(t)
		body, _ := json.Marshal(map[string]string{"title": "Test", "media_type": "podcast"})

		req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		router, _ := testRouter(t)

		req := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader([]byte("{invalid")))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})
}

func TestGetJobEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &models.Job{
		ID: "get-endpoint-test", Title: "Test Movie",
		MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
		SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
		CreatedAt: now, UpdatedAt: now,
	}
	database.CreateJob(ctx, job)

	t.Run("existing job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs/get-endpoint-test", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("status = %d, want 200", rr.Code)
		}

		var resp models.JobResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.ID != "get-endpoint-test" {
			t.Errorf("ID = %q", resp.ID)
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs/nonexistent", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 404 {
			t.Errorf("status = %d, want 404", rr.Code)
		}
	})
}

func TestListJobsEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		status := models.JobStatusPending
		mt := models.MediaTypeMovie
		if i%2 == 0 {
			mt = models.MediaTypeTV
		}
		if i >= 3 {
			status = models.JobStatusCompleted
		}
		job := &models.Job{
			ID: "list-" + string(rune('a'+i)), Title: "Title " + string(rune('a'+i)),
			MediaType: mt, Status: status,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now.Add(time.Duration(i) * time.Minute), UpdatedAt: now,
		}
		database.CreateJob(ctx, job)
	}

	t.Run("all jobs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("status = %d", rr.Code)
		}
		var resp models.JobListResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Total != 5 {
			t.Errorf("total = %d, want 5", resp.Total)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs?status=pending", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		var resp models.JobListResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Total != 3 {
			t.Errorf("total = %d, want 3", resp.Total)
		}
	})

	t.Run("filter by media type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs?media_type=tv", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		var resp models.JobListResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Total != 3 {
			t.Errorf("total = %d, want 3", resp.Total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=2&offset=0", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		var resp models.JobListResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Total != 5 {
			t.Errorf("total = %d", resp.Total)
		}
		if len(resp.Jobs) != 2 {
			t.Errorf("jobs len = %d, want 2", len(resp.Jobs))
		}
	})

	t.Run("limit bounds", func(t *testing.T) {
		// Limit < 1 should be clamped to 1
		req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=0", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		var resp models.JobListResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if len(resp.Jobs) != 1 {
			t.Errorf("jobs len = %d, want 1 (clamped from 0)", len(resp.Jobs))
		}
	})
}

func TestCancelJobEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &models.Job{
		ID: "cancel-test", Title: "Cancel Me",
		MediaType: models.MediaTypeMovie, Status: models.JobStatusSearching,
		SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
		CreatedAt: now, UpdatedAt: now,
	}
	database.CreateJob(ctx, job)

	t.Run("cancel active job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/cancel-test/cancel", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("status = %d, want 200", rr.Code)
		}

		var resp models.JobResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Status != models.JobStatusCancelled {
			t.Errorf("status = %q, want cancelled", resp.Status)
		}
		if resp.CompletedAt == nil {
			t.Error("CompletedAt should be set")
		}
	})

	t.Run("cancel already terminal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/cancel-test/cancel", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("cancel nonexistent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/nonexistent/cancel", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 404 {
			t.Errorf("status = %d, want 404", rr.Code)
		}
	})
}

func TestRetryJobEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	completedAt := now
	job := &models.Job{
		ID: "retry-test", Title: "Retry Me",
		MediaType: models.MediaTypeMovie, Status: models.JobStatusFailed,
		SourceAttempts: []models.SourceAttempt{
			{SourceName: "prowlarr", Query: "Retry Me", Success: false},
		},
		VerificationChecks: []models.VerificationProof{},
		VerifyCount:        10,
		CompletedAt:        &completedAt,
		CreatedAt:          now, UpdatedAt: now,
	}
	database.CreateJob(ctx, job)

	t.Run("retry failed job", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/jobs/retry-test/retry", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("status = %d, want 200", rr.Code)
		}

		var resp models.JobResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Status != models.JobStatusPending {
			t.Errorf("status = %q, want pending", resp.Status)
		}
		if len(resp.SourceAttempts) != 0 {
			t.Errorf("source_attempts should be empty after retry, got %d", len(resp.SourceAttempts))
		}
		if resp.VerifyCount != 0 {
			t.Errorf("verify_count should be 0 after retry, got %d", resp.VerifyCount)
		}
		if resp.CompletedAt != nil {
			t.Error("completed_at should be nil after retry")
		}
	})

	t.Run("retry active job", func(t *testing.T) {
		// Update to active state
		j, _ := database.GetJob(ctx, "retry-test")
		j.Status = models.JobStatusSearching
		database.UpdateJob(ctx, j)

		req := httptest.NewRequest(http.MethodPost, "/api/jobs/retry-test/retry", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400 (can't retry active job)", rr.Code)
		}
	})
}

func TestDeleteJobEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &models.Job{
		ID: "delete-test", Title: "Delete Me",
		MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
		SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
		CreatedAt: now, UpdatedAt: now,
	}
	database.CreateJob(ctx, job)

	t.Run("delete existing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/jobs/delete-test", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 204 {
			t.Errorf("status = %d, want 204", rr.Code)
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/jobs/nonexistent", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 404 {
			t.Errorf("status = %d, want 404", rr.Code)
		}
	})
}

func TestGetStatsEndpoint(t *testing.T) {
	router, database := testRouter(t)
	ctx := context.Background()
	now := time.Now().UTC()

	statuses := []models.JobStatus{
		models.JobStatusPending, models.JobStatusPending,
		models.JobStatusCompleted, models.JobStatusFailed,
	}
	for i, s := range statuses {
		job := &models.Job{
			ID: "stats-" + string(rune('a'+i)), Title: "S" + string(rune('a'+i)),
			MediaType: models.MediaTypeMovie, Status: s,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now,
		}
		database.CreateJob(ctx, job)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Errorf("status = %d", rr.Code)
	}

	var stats models.StatsResponse
	json.NewDecoder(rr.Body).Decode(&stats)
	if stats.TotalJobs != 4 {
		t.Errorf("TotalJobs = %d, want 4", stats.TotalJobs)
	}
	if stats.Pending != 2 {
		t.Errorf("Pending = %d, want 2", stats.Pending)
	}
}

func TestLegacyStatusEndpoint(t *testing.T) {
	router, _ := testRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Errorf("status = %d, want 200 for legacy /api/status", rr.Code)
	}
}

func TestVerifyEndpoint(t *testing.T) {
	router, _ := testRouter(t)

	t.Run("valid request", func(t *testing.T) {
		body, _ := json.Marshal(models.VerifyRequest{
			Title:     "The Matrix",
			MediaType: models.MediaTypeMovie,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 200 {
			t.Errorf("status = %d, want 200", rr.Code)
		}

		var resp models.VerifyResponse
		json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Title != "The Matrix" {
			t.Errorf("title = %q", resp.Title)
		}
		// No libraries configured, so not found
		if resp.Found {
			t.Error("expected not found with no libraries configured")
		}
	})

	t.Run("missing title", func(t *testing.T) {
		body, _ := json.Marshal(models.VerifyRequest{MediaType: models.MediaTypeMovie})

		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/verify", bytes.NewReader([]byte("{bad")))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != 400 {
			t.Errorf("status = %d, want 400", rr.Code)
		}
	})
}

func TestCORSHeaders(t *testing.T) {
	router, _ := testRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS Allow-Origin header")
	}
}

func TestCORSPreflight(t *testing.T) {
	router, _ := testRouter(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != 204 {
		t.Errorf("OPTIONS status = %d, want 204", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing CORS Allow-Methods header")
	}
}

func TestNewUUID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := newUUID()
		if len(id) != 36 {
			t.Errorf("UUID length = %d, want 36", len(id))
		}
		if seen[id] {
			t.Errorf("duplicate UUID: %s", id)
		}
		seen[id] = true
	}
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		query    string
		key      string
		fallback int
		expected int
	}{
		{"limit=10", "limit", 50, 10},
		{"", "limit", 50, 50},
		{"limit=abc", "limit", 50, 50},
		{"offset=-1", "offset", 0, -1},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, "/test?"+tc.query, nil)
		got := queryInt(req, tc.key, tc.fallback)
		if got != tc.expected {
			t.Errorf("queryInt(%q, %q, %d) = %d, want %d", tc.query, tc.key, tc.fallback, got, tc.expected)
		}
	}
}

func TestShortIDAPI(t *testing.T) {
	if shortID("abcdefghij") != "abcdefgh" {
		t.Error("shortID long")
	}
	if shortID("abc") != "abc" {
		t.Error("shortID short")
	}
}
