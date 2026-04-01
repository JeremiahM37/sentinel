package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMediaTypeNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    MediaType
		expected MediaType
	}{
		{"book normalizes to ebook", "book", MediaTypeEbook},
		{"manga normalizes to comic", "manga", MediaTypeComic},
		{"movie stays movie", MediaTypeMovie, MediaTypeMovie},
		{"tv stays tv", MediaTypeTV, MediaTypeTV},
		{"audiobook stays audiobook", MediaTypeAudiobook, MediaTypeAudiobook},
		{"ebook stays ebook", MediaTypeEbook, MediaTypeEbook},
		{"comic stays comic", MediaTypeComic, MediaTypeComic},
		{"game stays game", MediaTypeGame, MediaTypeGame},
		{"rom stays rom", MediaTypeROM, MediaTypeROM},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeMediaType(tc.input)
			if got != tc.expected {
				t.Errorf("NormalizeMediaType(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestValidMediaTypes(t *testing.T) {
	valid := []MediaType{
		MediaTypeMovie, MediaTypeTV, MediaTypeAudiobook,
		MediaTypeEbook, MediaTypeComic, MediaTypeBook,
		MediaTypeManga, MediaTypeGame, MediaTypeROM,
	}
	for _, mt := range valid {
		if !ValidMediaTypes[mt] {
			t.Errorf("expected %q to be valid", mt)
		}
	}

	invalid := []MediaType{"podcast", "music", "photo", ""}
	for _, mt := range invalid {
		if ValidMediaTypes[mt] {
			t.Errorf("expected %q to be invalid", mt)
		}
	}
}

func TestJobStatusIsTerminal(t *testing.T) {
	tests := []struct {
		status   JobStatus
		terminal bool
	}{
		{JobStatusPending, false},
		{JobStatusSearching, false},
		{JobStatusDownloading, false},
		{JobStatusVerifying, false},
		{JobStatusCompleted, true},
		{JobStatusFailed, true},
		{JobStatusCancelled, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if tc.status.IsTerminal() != tc.terminal {
				t.Errorf("JobStatus(%q).IsTerminal() = %v, want %v",
					tc.status, tc.status.IsTerminal(), tc.terminal)
			}
		})
	}
}

func TestJobStatusIsActive(t *testing.T) {
	tests := []struct {
		status JobStatus
		active bool
	}{
		{JobStatusPending, true},
		{JobStatusSearching, true},
		{JobStatusDownloading, true},
		{JobStatusVerifying, true},
		{JobStatusCompleted, false},
		{JobStatusFailed, false},
		{JobStatusCancelled, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			if tc.status.IsActive() != tc.active {
				t.Errorf("JobStatus(%q).IsActive() = %v, want %v",
					tc.status, tc.status.IsActive(), tc.active)
			}
		})
	}
}

func TestJobCreateRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     JobCreateRequest
		wantErr string
	}{
		{
			name:    "valid movie request",
			req:     JobCreateRequest{Title: "The Matrix", MediaType: MediaTypeMovie},
			wantErr: "",
		},
		{
			name:    "valid tv request with year",
			req:     JobCreateRequest{Title: "Breaking Bad", MediaType: MediaTypeTV, Year: intPtr(2008)},
			wantErr: "",
		},
		{
			name:    "missing title",
			req:     JobCreateRequest{Title: "", MediaType: MediaTypeMovie},
			wantErr: "title is required",
		},
		{
			name:    "invalid media type",
			req:     JobCreateRequest{Title: "Test", MediaType: "podcast"},
			wantErr: "invalid media_type",
		},
		{
			name:    "empty media type",
			req:     JobCreateRequest{Title: "Test", MediaType: ""},
			wantErr: "invalid media_type",
		},
		{
			name:    "book alias valid",
			req:     JobCreateRequest{Title: "Test Book", MediaType: MediaTypeBook},
			wantErr: "",
		},
		{
			name:    "manga alias valid",
			req:     JobCreateRequest{Title: "One Piece", MediaType: MediaTypeManga},
			wantErr: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.req.Validate()
			if got != tc.wantErr {
				t.Errorf("Validate() = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

func TestJobToResponse(t *testing.T) {
	t.Run("nil slices become empty", func(t *testing.T) {
		job := &Job{
			ID:                 "test-id",
			Title:              "Test",
			MediaType:          MediaTypeMovie,
			Status:             JobStatusPending,
			SourceAttempts:     nil,
			VerificationChecks: nil,
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		resp := JobToResponse(job)
		if resp.SourceAttempts == nil {
			t.Error("SourceAttempts should not be nil")
		}
		if len(resp.SourceAttempts) != 0 {
			t.Errorf("expected 0 source attempts, got %d", len(resp.SourceAttempts))
		}
		if resp.VerificationChecks == nil {
			t.Error("VerificationChecks should not be nil")
		}
		if len(resp.VerificationChecks) != 0 {
			t.Errorf("expected 0 verification checks, got %d", len(resp.VerificationChecks))
		}
	})

	t.Run("preserves all fields", func(t *testing.T) {
		year := 1999
		now := time.Now().UTC()
		job := &Job{
			ID:        "job-123",
			Title:     "The Matrix",
			MediaType: MediaTypeMovie,
			Status:    JobStatusCompleted,
			Year:      &year,
			Author:    "Wachowskis",
			SourceAttempts: []SourceAttempt{
				{SourceName: "prowlarr", Query: "The Matrix", Success: true},
			},
			VerificationChecks: []VerificationProof{
				{Library: "jellyfin", Status: VerificationFound},
			},
			VerifyCount: 3,
			CreatedAt:   now,
			UpdatedAt:   now,
			CompletedAt: &now,
		}

		resp := JobToResponse(job)
		if resp.ID != job.ID {
			t.Errorf("ID = %q, want %q", resp.ID, job.ID)
		}
		if resp.Title != job.Title {
			t.Errorf("Title = %q, want %q", resp.Title, job.Title)
		}
		if resp.MediaType != job.MediaType {
			t.Errorf("MediaType = %q, want %q", resp.MediaType, job.MediaType)
		}
		if resp.Status != job.Status {
			t.Errorf("Status = %q, want %q", resp.Status, job.Status)
		}
		if resp.Year == nil || *resp.Year != year {
			t.Errorf("Year = %v, want %d", resp.Year, year)
		}
		if resp.Author != job.Author {
			t.Errorf("Author = %q, want %q", resp.Author, job.Author)
		}
		if resp.VerifyCount != 3 {
			t.Errorf("VerifyCount = %d, want 3", resp.VerifyCount)
		}
		if resp.CompletedAt == nil {
			t.Error("CompletedAt should not be nil")
		}
		if len(resp.SourceAttempts) != 1 {
			t.Errorf("expected 1 source attempt, got %d", len(resp.SourceAttempts))
		}
		if len(resp.VerificationChecks) != 1 {
			t.Errorf("expected 1 verification check, got %d", len(resp.VerificationChecks))
		}
	})
}

func TestMustMarshalJSON(t *testing.T) {
	t.Run("valid input", func(t *testing.T) {
		result := MustMarshalJSON(map[string]string{"key": "value"})
		if result != `{"key":"value"}` {
			t.Errorf("got %q", result)
		}
	})

	t.Run("nil input", func(t *testing.T) {
		result := MustMarshalJSON(nil)
		if result != "null" {
			t.Errorf("got %q, want \"null\"", result)
		}
	})

	t.Run("int", func(t *testing.T) {
		result := MustMarshalJSON(42)
		if result != "42" {
			t.Errorf("got %q, want \"42\"", result)
		}
	})
}

func TestVerificationProofJSON(t *testing.T) {
	rt := 7200.0
	pc := 350
	af := 12
	proof := VerificationProof{
		Library:        "jellyfin",
		Status:         VerificationFound,
		TitleMatched:   "The Matrix",
		FilePath:       "/media/movies/The Matrix (1999)/The Matrix.mkv",
		RuntimeSeconds: &rt,
		PageCount:      &pc,
		AudioFileCount: &af,
		CheckedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	b, err := json.Marshal(proof)
	if err != nil {
		t.Fatal(err)
	}

	var decoded VerificationProof
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Library != "jellyfin" {
		t.Errorf("Library = %q", decoded.Library)
	}
	if decoded.Status != VerificationFound {
		t.Errorf("Status = %q", decoded.Status)
	}
	if decoded.RuntimeSeconds == nil || *decoded.RuntimeSeconds != 7200.0 {
		t.Errorf("RuntimeSeconds = %v", decoded.RuntimeSeconds)
	}
	if decoded.PageCount == nil || *decoded.PageCount != 350 {
		t.Errorf("PageCount = %v", decoded.PageCount)
	}
	if decoded.AudioFileCount == nil || *decoded.AudioFileCount != 12 {
		t.Errorf("AudioFileCount = %v", decoded.AudioFileCount)
	}
}

func TestStatsResponseJSON(t *testing.T) {
	stats := StatsResponse{
		TotalJobs:   100,
		Pending:     5,
		Searching:   3,
		Downloading: 10,
		Verifying:   2,
		Completed:   70,
		Failed:      8,
		Cancelled:   2,
	}

	b, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}

	var decoded StatsResponse
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.TotalJobs != 100 {
		t.Errorf("TotalJobs = %d, want 100", decoded.TotalJobs)
	}
	if decoded.Completed != 70 {
		t.Errorf("Completed = %d, want 70", decoded.Completed)
	}
}

func intPtr(i int) *int {
	return &i
}
