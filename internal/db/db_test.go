package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JeremiahM37/sentinel/internal/models"
)

func testDB(t *testing.T) *JobDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestConnect(t *testing.T) {
	t.Run("creates directory and file", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "sub", "dir", "test.db")
		database, err := Connect(dbPath)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		database.Close()
	})

	t.Run("invalid path returns error", func(t *testing.T) {
		_, err := Connect("/dev/null/impossible/test.db")
		if err == nil {
			t.Error("expected error for invalid path")
		}
	})
}

func TestCreateJob(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	job := &models.Job{
		ID:                 "job-001",
		Title:              "The Matrix",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusPending,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := db.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Duplicate should fail
	err := db.CreateJob(ctx, job)
	if err == nil {
		t.Error("expected error on duplicate insert")
	}
}

func TestCreateJobWithOptionalFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	year := 1999
	tvdbID := 12345
	job := &models.Job{
		ID:                 "job-optional",
		Title:              "The Matrix",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusPending,
		Year:               &year,
		Author:             "Wachowskis",
		ImdbID:             "tt0133093",
		TvdbID:             &tvdbID,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := db.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	fetched, err := db.GetJob(ctx, "job-optional")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if fetched.Year == nil || *fetched.Year != 1999 {
		t.Errorf("Year = %v, want 1999", fetched.Year)
	}
	if fetched.Author != "Wachowskis" {
		t.Errorf("Author = %q", fetched.Author)
	}
	if fetched.ImdbID != "tt0133093" {
		t.Errorf("ImdbID = %q", fetched.ImdbID)
	}
	if fetched.TvdbID == nil || *fetched.TvdbID != 12345 {
		t.Errorf("TvdbID = %v", fetched.TvdbID)
	}
}

func TestGetJob(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	t.Run("existing job", func(t *testing.T) {
		now := time.Now().UTC()
		job := &models.Job{
			ID:                 "get-test",
			Title:              "Inception",
			MediaType:          models.MediaTypeMovie,
			Status:             models.JobStatusPending,
			SourceAttempts:     []models.SourceAttempt{},
			VerificationChecks: []models.VerificationProof{},
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		db.CreateJob(ctx, job)

		fetched, err := db.GetJob(ctx, "get-test")
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if fetched == nil {
			t.Fatal("expected job, got nil")
		}
		if fetched.Title != "Inception" {
			t.Errorf("Title = %q", fetched.Title)
		}
	})

	t.Run("nonexistent job returns nil", func(t *testing.T) {
		fetched, err := db.GetJob(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if fetched != nil {
			t.Errorf("expected nil for nonexistent job, got %+v", fetched)
		}
	})
}

func TestUpdateJob(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &models.Job{
		ID:                 "update-test",
		Title:              "Original Title",
		MediaType:          models.MediaTypeMovie,
		Status:             models.JobStatusPending,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	db.CreateJob(ctx, job)

	// Update status
	job.Status = models.JobStatusSearching
	job.SourceAttempts = []models.SourceAttempt{
		{SourceName: "prowlarr", Query: "Original Title", StartedAt: now, Success: false},
	}
	if err := db.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	fetched, _ := db.GetJob(ctx, "update-test")
	if fetched.Status != models.JobStatusSearching {
		t.Errorf("Status = %q, want searching", fetched.Status)
	}
	if len(fetched.SourceAttempts) != 1 {
		t.Errorf("SourceAttempts len = %d, want 1", len(fetched.SourceAttempts))
	}
	if fetched.SourceAttempts[0].SourceName != "prowlarr" {
		t.Errorf("SourceAttempt name = %q", fetched.SourceAttempts[0].SourceName)
	}

	// Update to completed with CompletedAt
	completedAt := time.Now().UTC()
	job.Status = models.JobStatusCompleted
	job.CompletedAt = &completedAt
	job.VerificationChecks = []models.VerificationProof{
		{Library: "jellyfin", Status: models.VerificationFound, CheckedAt: now},
	}
	if err := db.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	fetched2, _ := db.GetJob(ctx, "update-test")
	if fetched2.Status != models.JobStatusCompleted {
		t.Errorf("Status = %q, want completed", fetched2.Status)
	}
	if fetched2.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
	if len(fetched2.VerificationChecks) != 1 {
		t.Errorf("VerificationChecks len = %d", len(fetched2.VerificationChecks))
	}
}

func TestListJobs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create jobs of different types and statuses
	jobs := []models.Job{
		{ID: "list-1", Title: "Movie A", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now},
		{ID: "list-2", Title: "TV Show B", MediaType: models.MediaTypeTV, Status: models.JobStatusSearching,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now},
		{ID: "list-3", Title: "Movie C", MediaType: models.MediaTypeMovie, Status: models.JobStatusCompleted,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now.Add(-1 * time.Hour), UpdatedAt: now},
		{ID: "list-4", Title: "Audiobook D", MediaType: models.MediaTypeAudiobook, Status: models.JobStatusPending,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
	}
	for i := range jobs {
		db.CreateJob(ctx, &jobs[i])
	}

	t.Run("all jobs", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "", "", 50, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(result) != 4 {
			t.Errorf("len = %d, want 4", len(result))
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "pending", "", 50, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		if len(result) != 2 {
			t.Errorf("len = %d, want 2", len(result))
		}
	})

	t.Run("filter by media type", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "", "movie", 50, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 2 {
			t.Errorf("total = %d, want 2", total)
		}
		if len(result) != 2 {
			t.Errorf("len = %d, want 2", len(result))
		}
	})

	t.Run("filter by both", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "pending", "movie", 50, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(result) != 1 {
			t.Errorf("len = %d, want 1", len(result))
		}
	})

	t.Run("limit and offset", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "", "", 2, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 4 {
			t.Errorf("total = %d, want 4", total)
		}
		if len(result) != 2 {
			t.Errorf("len = %d, want 2", len(result))
		}

		result2, _, _ := db.ListJobs(ctx, "", "", 2, 2)
		if len(result2) != 2 {
			t.Errorf("offset page len = %d, want 2", len(result2))
		}
	})

	t.Run("no matches returns empty slice", func(t *testing.T) {
		result, total, err := db.ListJobs(ctx, "failed", "", 50, 0)
		if err != nil {
			t.Fatalf("ListJobs: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if result == nil {
			t.Error("result should be empty slice, not nil")
		}
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})
}

func TestGetActiveJobs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	jobs := []models.Job{
		{ID: "active-1", Title: "A", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "active-2", Title: "B", MediaType: models.MediaTypeMovie, Status: models.JobStatusDownloading,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "active-3", Title: "C", MediaType: models.MediaTypeMovie, Status: models.JobStatusCompleted,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "active-4", Title: "D", MediaType: models.MediaTypeMovie, Status: models.JobStatusFailed,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "active-5", Title: "E", MediaType: models.MediaTypeMovie, Status: models.JobStatusVerifying,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
	}
	for i := range jobs {
		db.CreateJob(ctx, &jobs[i])
	}

	active, err := db.GetActiveJobs(ctx)
	if err != nil {
		t.Fatalf("GetActiveJobs: %v", err)
	}

	// Should return pending, downloading, verifying (3 active statuses present)
	if len(active) != 3 {
		t.Errorf("got %d active jobs, want 3", len(active))
	}

	activeStatuses := make(map[models.JobStatus]bool)
	for _, j := range active {
		activeStatuses[j.Status] = true
	}
	if !activeStatuses[models.JobStatusPending] {
		t.Error("missing pending job")
	}
	if !activeStatuses[models.JobStatusDownloading] {
		t.Error("missing downloading job")
	}
	if !activeStatuses[models.JobStatusVerifying] {
		t.Error("missing verifying job")
	}
}

func TestGetStats(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	jobs := []models.Job{
		{ID: "s1", Title: "A", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "s2", Title: "B", MediaType: models.MediaTypeMovie, Status: models.JobStatusPending,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "s3", Title: "C", MediaType: models.MediaTypeMovie, Status: models.JobStatusCompleted,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
		{ID: "s4", Title: "D", MediaType: models.MediaTypeMovie, Status: models.JobStatusFailed,
			SourceAttempts: []models.SourceAttempt{}, VerificationChecks: []models.VerificationProof{},
			CreatedAt: now, UpdatedAt: now},
	}
	for i := range jobs {
		db.CreateJob(ctx, &jobs[i])
	}

	stats, err := db.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.TotalJobs != 4 {
		t.Errorf("TotalJobs = %d, want 4", stats.TotalJobs)
	}
	if stats.Pending != 2 {
		t.Errorf("Pending = %d, want 2", stats.Pending)
	}
	if stats.Completed != 1 {
		t.Errorf("Completed = %d, want 1", stats.Completed)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if stats.Searching != 0 {
		t.Errorf("Searching = %d, want 0", stats.Searching)
	}
}

func TestDeleteJob(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	job := &models.Job{
		ID: "del-test", Title: "Delete Me", MediaType: models.MediaTypeMovie,
		Status: models.JobStatusPending, SourceAttempts: []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{}, CreatedAt: now, UpdatedAt: now,
	}
	db.CreateJob(ctx, job)

	t.Run("delete existing", func(t *testing.T) {
		deleted, err := db.DeleteJob(ctx, "del-test")
		if err != nil {
			t.Fatalf("DeleteJob: %v", err)
		}
		if !deleted {
			t.Error("expected deleted=true")
		}

		fetched, _ := db.GetJob(ctx, "del-test")
		if fetched != nil {
			t.Error("job should be gone after delete")
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		deleted, err := db.DeleteJob(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("DeleteJob: %v", err)
		}
		if deleted {
			t.Error("expected deleted=false for nonexistent")
		}
	})
}

func TestPersistenceAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()
	now := time.Now().UTC()

	// Write
	database, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	job := &models.Job{
		ID: "persist-1", Title: "Persistent",
		MediaType: models.MediaTypeAudiobook, Status: models.JobStatusDownloading,
		SourceAttempts: []models.SourceAttempt{
			{SourceName: "prowlarr", Query: "Persistent", StartedAt: now, Success: true, DownloadID: "dl-123"},
		},
		VerificationChecks: []models.VerificationProof{},
		CurrentDownloadID:  "dl-123",
		VerifyCount:        5,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	database.CreateJob(ctx, job)
	database.Close()

	// Reopen and verify
	database2, err := Connect(dbPath)
	if err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	defer database2.Close()

	fetched, err := database2.GetJob(ctx, "persist-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected job after reopen")
	}
	if fetched.Title != "Persistent" {
		t.Errorf("Title = %q", fetched.Title)
	}
	if fetched.Status != models.JobStatusDownloading {
		t.Errorf("Status = %q", fetched.Status)
	}
	if fetched.CurrentDownloadID != "dl-123" {
		t.Errorf("CurrentDownloadID = %q", fetched.CurrentDownloadID)
	}
	if fetched.VerifyCount != 5 {
		t.Errorf("VerifyCount = %d", fetched.VerifyCount)
	}
	if len(fetched.SourceAttempts) != 1 {
		t.Fatalf("SourceAttempts len = %d", len(fetched.SourceAttempts))
	}
	if fetched.SourceAttempts[0].DownloadID != "dl-123" {
		t.Errorf("SourceAttempt DownloadID = %q", fetched.SourceAttempts[0].DownloadID)
	}
}

func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		input string
		isNil bool
	}{
		{"", true},
		{"hello", false},
		{" ", false},
	}
	for _, tc := range tests {
		result := nilIfEmpty(tc.input)
		if tc.isNil && result != nil {
			t.Errorf("nilIfEmpty(%q) = %v, want nil", tc.input, result)
		}
		if !tc.isNil && result == nil {
			t.Errorf("nilIfEmpty(%q) = nil, want non-nil", tc.input)
		}
		if !tc.isNil && result != nil && *result != tc.input {
			t.Errorf("nilIfEmpty(%q) = %q", tc.input, *result)
		}
	}
}

func TestEmptyDB(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	t.Run("list empty", func(t *testing.T) {
		jobs, total, err := db.ListJobs(ctx, "", "", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 0 {
			t.Errorf("total = %d", total)
		}
		if jobs == nil {
			t.Error("expected empty slice, not nil")
		}
	})

	t.Run("active empty", func(t *testing.T) {
		jobs, err := db.GetActiveJobs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) != 0 {
			t.Errorf("got %d active jobs on empty db", len(jobs))
		}
	})

	t.Run("stats empty", func(t *testing.T) {
		stats, err := db.GetStats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if stats.TotalJobs != 0 {
			t.Errorf("TotalJobs = %d", stats.TotalJobs)
		}
	})
}
