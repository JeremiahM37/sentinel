package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

func TestSupportsMediaType(t *testing.T) {
	tests := []struct {
		name      string
		source    Source
		mediaType models.MediaType
		supported bool
	}{
		{"jellyseerr supports movie", &JellyseerrSource{}, models.MediaTypeMovie, true},
		{"jellyseerr supports tv", &JellyseerrSource{}, models.MediaTypeTV, true},
		{"jellyseerr rejects audiobook", &JellyseerrSource{}, models.MediaTypeAudiobook, false},
		{"prowlarr supports movie", &ProwlarrSource{}, models.MediaTypeMovie, true},
		{"prowlarr supports tv", &ProwlarrSource{}, models.MediaTypeTV, true},
		{"prowlarr supports audiobook", &ProwlarrSource{}, models.MediaTypeAudiobook, true},
		{"prowlarr supports ebook", &ProwlarrSource{}, models.MediaTypeEbook, true},
		{"prowlarr supports comic", &ProwlarrSource{}, models.MediaTypeComic, true},
		{"prowlarr rejects game", &ProwlarrSource{}, models.MediaTypeGame, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SupportsMediaType(tc.source, tc.mediaType)
			if got != tc.supported {
				t.Errorf("SupportsMediaType(%s, %q) = %v, want %v",
					tc.source.Name(), tc.mediaType, got, tc.supported)
			}
		})
	}
}

func TestSourceNames(t *testing.T) {
	if (&JellyseerrSource{}).Name() != "jellyseerr" {
		t.Error("jellyseerr name mismatch")
	}
	if (&ProwlarrSource{}).Name() != "prowlarr" {
		t.Error("prowlarr name mismatch")
	}
}

// --- Jellyseerr Tests ---

func TestJellyseerrIsAvailable(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/status" {
				w.WriteHeader(200)
			}
		}))
		defer ts.Close()

		s := &JellyseerrSource{}
		cfg := &config.Config{JellyseerrURL: ts.URL, JellyseerrAPIKey: "key"}
		if !s.IsAvailable(context.Background(), cfg) {
			t.Error("expected available")
		}
	})

	t.Run("not configured", func(t *testing.T) {
		s := &JellyseerrSource{}
		cfg := &config.Config{}
		if s.IsAvailable(context.Background(), cfg) {
			t.Error("expected unavailable")
		}
	})
}

func TestJellyseerrSearchAndDownloadSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/search"):
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{
						"id":          550.0,
						"mediaType":   "movie",
						"title":       "The Matrix",
						"releaseDate": "1999-03-31",
					},
				},
			})
		case r.URL.Path == "/api/v1/request":
			json.NewEncoder(w).Encode(map[string]any{
				"id": 42.0,
			})
		}
	}))
	defer ts.Close()

	s := &JellyseerrSource{}
	cfg := &config.Config{JellyseerrURL: ts.URL, JellyseerrAPIKey: "key"}
	year := 1999
	job := &models.Job{
		ID:        "test",
		Title:     "The Matrix",
		MediaType: models.MediaTypeMovie,
		Year:      &year,
	}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if !attempt.Success {
		t.Errorf("expected success, error: %s", attempt.ErrorMessage)
	}
	if attempt.DownloadID == "" {
		t.Error("DownloadID should not be empty")
	}
	if attempt.SourceName != "jellyseerr" {
		t.Errorf("SourceName = %q", attempt.SourceName)
	}
	if attempt.FinishedAt == nil {
		t.Error("FinishedAt should not be nil")
	}
}

func TestJellyseerrSearchNoResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{}})
	}))
	defer ts.Close()

	s := &JellyseerrSource{}
	cfg := &config.Config{JellyseerrURL: ts.URL, JellyseerrAPIKey: "key"}
	job := &models.Job{ID: "test", Title: "Nonexistent", MediaType: models.MediaTypeMovie}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if attempt.Success {
		t.Error("expected failure for no results")
	}
	if attempt.ErrorMessage == "" {
		t.Error("expected error message")
	}
}

func TestJellyseerrRequestError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/search"):
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 1.0, "mediaType": "movie", "title": "Test"},
				},
			})
		case r.URL.Path == "/api/v1/request":
			w.WriteHeader(409) // conflict (already requested)
		}
	}))
	defer ts.Close()

	s := &JellyseerrSource{}
	cfg := &config.Config{JellyseerrURL: ts.URL, JellyseerrAPIKey: "key"}
	job := &models.Job{ID: "test", Title: "Test", MediaType: models.MediaTypeMovie}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if attempt.Success {
		t.Error("expected failure on 409")
	}
}

// --- Prowlarr Tests ---

func TestProwlarrIsAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" && r.Header.Get("X-Api-Key") == "prowlarr-key" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(401)
		}
	}))
	defer ts.Close()

	s := &ProwlarrSource{}

	t.Run("available", func(t *testing.T) {
		cfg := &config.Config{ProwlarrURL: ts.URL, ProwlarrAPIKey: "prowlarr-key"}
		if !s.IsAvailable(context.Background(), cfg) {
			t.Error("expected available")
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		cfg := &config.Config{ProwlarrURL: ts.URL, ProwlarrAPIKey: "wrong"}
		if s.IsAvailable(context.Background(), cfg) {
			t.Error("expected unavailable with wrong key")
		}
	})
}

func TestProwlarrSearchAndDownloadSuccess(t *testing.T) {
	qbitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/add":
			fmt.Fprint(w, "Ok.")
		}
	}))
	defer qbitServer.Close()

	prowlarrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"title":       "The Matrix 1999 1080p",
				"downloadUrl": "http://example.com/torrent.torrent",
				"seeders":     150.0,
				"size":        15000000000.0,
			},
			{
				"title":    "The Matrix 1999 720p",
				"seeders":  50.0,
				"magnetUrl": "magnet:?xt=urn:btih:abc123",
			},
		})
	}))
	defer prowlarrServer.Close()

	s := &ProwlarrSource{}
	cfg := &config.Config{
		ProwlarrURL:    prowlarrServer.URL,
		ProwlarrAPIKey: "key",
		QBittorrentURL:  qbitServer.URL,
		QBittorrentUser: "admin",
		QBittorrentPass: "pass",
	}

	job := &models.Job{ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie}
	attempt := s.SearchAndDownload(context.Background(), job, cfg)

	if !attempt.Success {
		t.Errorf("expected success, error: %s", attempt.ErrorMessage)
	}
	if attempt.DownloadID == "" {
		t.Error("DownloadID should not be empty")
	}
}

func TestProwlarrSearchNoResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer ts.Close()

	s := &ProwlarrSource{}
	cfg := &config.Config{ProwlarrURL: ts.URL, ProwlarrAPIKey: "key"}
	job := &models.Job{ID: "test", Title: "Nonexistent", MediaType: models.MediaTypeMovie}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if attempt.Success {
		t.Error("expected failure")
	}
	if !strings.Contains(attempt.ErrorMessage, "No results") {
		t.Errorf("unexpected error: %s", attempt.ErrorMessage)
	}
}

func TestProwlarrSearchWithYear(t *testing.T) {
	var receivedQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query().Get("query")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer ts.Close()

	s := &ProwlarrSource{}
	cfg := &config.Config{ProwlarrURL: ts.URL, ProwlarrAPIKey: "key"}
	year := 1999
	job := &models.Job{ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie, Year: &year}

	s.SearchAndDownload(context.Background(), job, cfg)

	if receivedQuery != "The Matrix 1999" {
		t.Errorf("query = %q, want 'The Matrix 1999'", receivedQuery)
	}
}

func TestProwlarrNoDownloadURL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"title": "Test", "seeders": 100.0}, // no downloadUrl or magnetUrl
		})
	}))
	defer ts.Close()

	s := &ProwlarrSource{}
	cfg := &config.Config{ProwlarrURL: ts.URL, ProwlarrAPIKey: "key"}
	job := &models.Job{ID: "test", Title: "Test", MediaType: models.MediaTypeMovie}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if attempt.Success {
		t.Error("expected failure when no download URL")
	}
	if !strings.Contains(attempt.ErrorMessage, "no download URL") {
		t.Errorf("unexpected error: %s", attempt.ErrorMessage)
	}
}

func TestProwlarrWithoutQBittorrent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"title":       "Test Result",
				"downloadUrl": "http://example.com/torrent.torrent",
				"seeders":     50.0,
			},
		})
	}))
	defer ts.Close()

	s := &ProwlarrSource{}
	cfg := &config.Config{
		ProwlarrURL:    ts.URL,
		ProwlarrAPIKey: "key",
		// No qbittorrent configured
	}
	job := &models.Job{ID: "test", Title: "Test", MediaType: models.MediaTypeMovie}

	attempt := s.SearchAndDownload(context.Background(), job, cfg)
	if !attempt.Success {
		t.Errorf("expected success (without qbit), error: %s", attempt.ErrorMessage)
	}
	// DownloadID should be the URL itself
	if attempt.DownloadID != "http://example.com/torrent.torrent" {
		t.Errorf("DownloadID = %q, expected URL", attempt.DownloadID)
	}
}

// --- qBittorrent Monitor Tests ---

func TestQBittorrentMonitorIsAvailable(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		m := NewQBittorrentMonitor(&config.Config{})
		if m.IsAvailable(context.Background()) {
			t.Error("expected unavailable when not configured")
		}
	})

	t.Run("auth success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v2/auth/login" {
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
				fmt.Fprint(w, "Ok.")
			}
		}))
		defer ts.Close()

		m := NewQBittorrentMonitor(&config.Config{
			QBittorrentURL: ts.URL, QBittorrentUser: "admin", QBittorrentPass: "pass",
		})
		if !m.IsAvailable(context.Background()) {
			t.Error("expected available")
		}
	})

	t.Run("auth failure", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Fails.")
		}))
		defer ts.Close()

		m := NewQBittorrentMonitor(&config.Config{
			QBittorrentURL: ts.URL, QBittorrentUser: "admin", QBittorrentPass: "wrong",
		})
		if m.IsAvailable(context.Background()) {
			t.Error("expected unavailable on auth failure")
		}
	})
}

func TestQBittorrentIsDownloadComplete(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		progress float64
		wantDone *bool
	}{
		{"uploading", "uploading", 1.0, boolPtr(true)},
		{"stalledUP", "stalledUP", 1.0, boolPtr(true)},
		{"pausedUP", "pausedUP", 1.0, boolPtr(true)},
		{"stoppedUP", "stoppedUP", 1.0, boolPtr(true)},
		{"queuedUP", "queuedUP", 1.0, boolPtr(true)},
		{"still downloading", "downloading", 0.5, boolPtr(false)},
		{"stalledDL", "stalledDL", 0.3, boolPtr(false)},
		{"progress 100%", "downloading", 1.0, boolPtr(true)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
					fmt.Fprint(w, "Ok.")
				case "/api/v2/torrents/info":
					json.NewEncoder(w).Encode([]map[string]any{
						{"state": tc.state, "progress": tc.progress, "name": "Test", "hash": "abc"},
					})
				}
			}))
			defer ts.Close()

			m := NewQBittorrentMonitor(&config.Config{
				QBittorrentURL: ts.URL, QBittorrentUser: "admin", QBittorrentPass: "pass",
			})

			result := m.IsDownloadComplete(context.Background(), "sentinel:Test")
			if tc.wantDone == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", *result)
				}
			} else {
				if result == nil {
					t.Error("expected non-nil result")
				} else if *result != *tc.wantDone {
					t.Errorf("got %v, want %v", *result, *tc.wantDone)
				}
			}
		})
	}
}

func TestQBittorrentIsDownloadCompleteNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{}) // empty
		}
	}))
	defer ts.Close()

	m := NewQBittorrentMonitor(&config.Config{
		QBittorrentURL: ts.URL, QBittorrentUser: "admin", QBittorrentPass: "pass",
	})

	result := m.IsDownloadComplete(context.Background(), "sentinel:Missing")
	if result != nil {
		t.Errorf("expected nil for not found, got %v", *result)
	}
}

func TestGetTorrentStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			json.NewEncoder(w).Encode([]map[string]any{
				{
					"state":      "downloading",
					"progress":   0.75,
					"name":       "Test Torrent",
					"size":       1000000000.0,
					"downloaded": 750000000.0,
					"eta":        300.0,
					"num_seeds":  10.0,
					"hash":       "abc123def",
				},
			})
		}
	}))
	defer ts.Close()

	m := NewQBittorrentMonitor(&config.Config{
		QBittorrentURL: ts.URL, QBittorrentUser: "admin", QBittorrentPass: "pass",
	})

	status := m.GetTorrentStatus(context.Background(), "sentinel:Test")
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.State != "downloading" {
		t.Errorf("State = %q", status.State)
	}
	if status.Progress != 0.75 {
		t.Errorf("Progress = %f", status.Progress)
	}
	if status.Name != "Test Torrent" {
		t.Errorf("Name = %q", status.Name)
	}
	if status.Seeds != 10 {
		t.Errorf("Seeds = %d", status.Seeds)
	}
}

// --- Helper Tests ---

func TestUrlEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "hello%20world"},
		{"test-123_foo.bar~baz", "test-123_foo.bar~baz"},
		{"a+b", "a%2Bb"},
	}

	for _, tc := range tests {
		got := urlEncode(tc.input)
		if got != tc.expected {
			t.Errorf("urlEncode(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestPickBestResult(t *testing.T) {
	t.Run("filters by media type", func(t *testing.T) {
		results := []map[string]any{
			{"id": 1.0, "mediaType": "tv", "title": "Bad Match"},
			{"id": 2.0, "mediaType": "movie", "title": "Good Match"},
		}
		job := &models.Job{MediaType: models.MediaTypeMovie}
		best := pickBestResult(results, job)
		if best == nil {
			t.Fatal("expected result")
		}
		if best["id"].(float64) != 2.0 {
			t.Errorf("picked wrong result: %v", best["id"])
		}
	})

	t.Run("year match", func(t *testing.T) {
		results := []map[string]any{
			{"id": 1.0, "mediaType": "movie", "title": "Test", "releaseDate": "2020-01-01"},
			{"id": 2.0, "mediaType": "movie", "title": "Test", "releaseDate": "1999-03-31"},
		}
		year := 1999
		job := &models.Job{MediaType: models.MediaTypeMovie, Year: &year}
		best := pickBestResult(results, job)
		if best["id"].(float64) != 2.0 {
			t.Errorf("expected year-matching result, got id=%v", best["id"])
		}
	})

	t.Run("empty results", func(t *testing.T) {
		job := &models.Job{MediaType: models.MediaTypeMovie}
		best := pickBestResult([]map[string]any{}, job)
		if best != nil {
			t.Error("expected nil for empty results")
		}
	})

	t.Run("fallback to all when no type match", func(t *testing.T) {
		results := []map[string]any{
			{"id": 1.0, "mediaType": "person", "title": "Only Result"},
		}
		job := &models.Job{MediaType: models.MediaTypeMovie}
		best := pickBestResult(results, job)
		if best == nil {
			t.Error("expected fallback to first result")
		}
	})
}

func TestCompletionStates(t *testing.T) {
	expected := []string{"uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "checkingUP", "forcedUP"}
	for _, s := range expected {
		if !completionStates[s] {
			t.Errorf("state %q should be a completion state", s)
		}
	}

	notComplete := []string{"downloading", "stalledDL", "pausedDL", "queuedDL", "error", "missingFiles"}
	for _, s := range notComplete {
		if completionStates[s] {
			t.Errorf("state %q should NOT be a completion state", s)
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}
