package libraries

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

func TestSupportsMediaType(t *testing.T) {
	tests := []struct {
		name      string
		checker   LibraryChecker
		mediaType models.MediaType
		supported bool
	}{
		{"jellyfin supports movie", &JellyfinChecker{}, models.MediaTypeMovie, true},
		{"jellyfin supports tv", &JellyfinChecker{}, models.MediaTypeTV, true},
		{"jellyfin rejects audiobook", &JellyfinChecker{}, models.MediaTypeAudiobook, false},
		{"jellyfin rejects ebook", &JellyfinChecker{}, models.MediaTypeEbook, false},
		{"audiobookshelf supports audiobook", &AudiobookshelfChecker{}, models.MediaTypeAudiobook, true},
		{"audiobookshelf rejects movie", &AudiobookshelfChecker{}, models.MediaTypeMovie, false},
		{"kavita supports comic", &KavitaChecker{}, models.MediaTypeComic, true},
		{"kavita supports ebook", &KavitaChecker{}, models.MediaTypeEbook, true},
		{"kavita rejects movie", &KavitaChecker{}, models.MediaTypeMovie, false},
		{"sonarr supports tv", &SonarrChecker{}, models.MediaTypeTV, true},
		{"sonarr rejects movie", &SonarrChecker{}, models.MediaTypeMovie, false},
		{"radarr supports movie", &RadarrChecker{}, models.MediaTypeMovie, true},
		{"radarr rejects tv", &RadarrChecker{}, models.MediaTypeTV, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SupportsMediaType(tc.checker, tc.mediaType)
			if got != tc.supported {
				t.Errorf("SupportsMediaType(%s, %q) = %v, want %v",
					tc.checker.Name(), tc.mediaType, got, tc.supported)
			}
		})
	}
}

func TestCheckerNames(t *testing.T) {
	checkers := []struct {
		checker LibraryChecker
		name    string
	}{
		{&JellyfinChecker{}, "jellyfin"},
		{&AudiobookshelfChecker{}, "audiobookshelf"},
		{&KavitaChecker{}, "kavita"},
		{&SonarrChecker{}, "sonarr"},
		{&RadarrChecker{}, "radarr"},
	}

	for _, tc := range checkers {
		t.Run(tc.name, func(t *testing.T) {
			if tc.checker.Name() != tc.name {
				t.Errorf("Name() = %q, want %q", tc.checker.Name(), tc.name)
			}
		})
	}
}

// --- Jellyfin Checker Tests ---

func TestJellyfinIsAvailable(t *testing.T) {
	t.Run("available", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/System/Info/Public" {
				w.WriteHeader(200)
			}
		}))
		defer ts.Close()

		c := &JellyfinChecker{}
		cfg := &config.Config{JellyfinURL: ts.URL, JellyfinAPIKey: "key"}
		if !c.IsAvailable(context.Background(), cfg) {
			t.Error("expected available")
		}
	})

	t.Run("not configured", func(t *testing.T) {
		c := &JellyfinChecker{}
		cfg := &config.Config{}
		if c.IsAvailable(context.Background(), cfg) {
			t.Error("expected unavailable when not configured")
		}
	})

	t.Run("server down", func(t *testing.T) {
		c := &JellyfinChecker{}
		cfg := &config.Config{JellyfinURL: "http://127.0.0.1:1", JellyfinAPIKey: "key"}
		if c.IsAvailable(context.Background(), cfg) {
			t.Error("expected unavailable when server down")
		}
	})
}

func TestJellyfinVerifyFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"Items": []map[string]any{
				{
					"Id":             "jf-123",
					"Name":           "The Matrix",
					"ProductionYear": 1999.0,
					"Path":           "/media/movies/The Matrix (1999)/The Matrix.mkv",
					"RunTimeTicks":   81360000000.0, // ~2.26 hours in ticks
					"MediaSources": []any{
						map[string]any{
							"Path": "/media/movies/The Matrix (1999)/The Matrix.mkv",
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := &JellyfinChecker{}
	cfg := &config.Config{
		JellyfinURL:         ts.URL,
		JellyfinAPIKey:      "key",
		TitleMatchThreshold: 0.7,
	}

	year := 1999
	job := &models.Job{
		ID:        "test",
		Title:     "The Matrix",
		MediaType: models.MediaTypeMovie,
		Year:      &year,
	}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found", proof.Status)
	}
	if proof.TitleMatched != "The Matrix" {
		t.Errorf("TitleMatched = %q", proof.TitleMatched)
	}
	if proof.FilePath == "" {
		t.Error("FilePath should not be empty")
	}
	if proof.RuntimeSeconds == nil {
		t.Error("RuntimeSeconds should not be nil")
	}
}

func TestJellyfinVerifyNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"Items": []map[string]any{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := &JellyfinChecker{}
	cfg := &config.Config{
		JellyfinURL:         ts.URL,
		JellyfinAPIKey:      "key",
		TitleMatchThreshold: 0.7,
	}

	job := &models.Job{ID: "test", Title: "Nonexistent Movie", MediaType: models.MediaTypeMovie}
	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationNotFound {
		t.Errorf("Status = %q, want not_found", proof.Status)
	}
}

func TestJellyfinVerifyBelowThreshold(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"Items": []map[string]any{
				{"Id": "1", "Name": "Something Completely Different", "Path": "/path"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c := &JellyfinChecker{}
	cfg := &config.Config{
		JellyfinURL:         ts.URL,
		JellyfinAPIKey:      "key",
		TitleMatchThreshold: 0.7,
	}

	job := &models.Job{ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie}
	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationNotFound {
		t.Errorf("Status = %q, want not_found (below threshold)", proof.Status)
	}
}

func TestJellyfinVerifyHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer ts.Close()

	c := &JellyfinChecker{}
	cfg := &config.Config{JellyfinURL: ts.URL, JellyfinAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "Test", MediaType: models.MediaTypeMovie}
	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationError {
		t.Errorf("Status = %q, want error", proof.Status)
	}
}

// --- Audiobookshelf Checker Tests ---

func TestAudiobookshelfIsAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	c := &AudiobookshelfChecker{}
	cfg := &config.Config{AudiobookshelfURL: ts.URL, AudiobookshelfAPIKey: "key"}
	if !c.IsAvailable(context.Background(), cfg) {
		t.Error("expected available")
	}
}

func TestAudiobookshelfVerifyFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/libraries":
			json.NewEncoder(w).Encode(map[string]any{
				"libraries": []map[string]string{{"id": "lib1"}},
			})
		case r.URL.Path == "/api/libraries/lib1/search":
			json.NewEncoder(w).Encode(map[string]any{
				"book": []map[string]any{
					{
						"libraryItem": map[string]any{
							"id":        "abs-123",
							"path":      "/audiobooks/Test Book",
							"isMissing": false,
							"media": map[string]any{
								"numAudioFiles": 15.0,
								"duration":      36000.0,
								"metadata": map[string]any{
									"title":      "Test Book",
									"authorName": "Test Author",
								},
							},
						},
					},
				},
			})
		}
	}))
	defer ts.Close()

	c := &AudiobookshelfChecker{}
	cfg := &config.Config{
		AudiobookshelfURL:    ts.URL,
		AudiobookshelfAPIKey: "key",
		TitleMatchThreshold:  0.7,
	}

	job := &models.Job{
		ID: "test", Title: "Test Book", MediaType: models.MediaTypeAudiobook, Author: "Test Author",
	}
	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found", proof.Status)
	}
	if proof.AudioFileCount == nil || *proof.AudioFileCount != 15 {
		t.Errorf("AudioFileCount = %v", proof.AudioFileCount)
	}
	if proof.RuntimeSeconds == nil || *proof.RuntimeSeconds != 36000.0 {
		t.Errorf("RuntimeSeconds = %v", proof.RuntimeSeconds)
	}
}

func TestAudiobookshelfVerifyMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/libraries":
			json.NewEncoder(w).Encode(map[string]any{
				"libraries": []map[string]string{{"id": "lib1"}},
			})
		case r.URL.Path == "/api/libraries/lib1/search":
			json.NewEncoder(w).Encode(map[string]any{
				"book": []map[string]any{
					{
						"libraryItem": map[string]any{
							"isMissing": true,
							"media": map[string]any{
								"numAudioFiles": 0.0,
								"metadata": map[string]any{
									"title": "Test Book",
								},
							},
						},
					},
				},
			})
		}
	}))
	defer ts.Close()

	c := &AudiobookshelfChecker{}
	cfg := &config.Config{
		AudiobookshelfURL:    ts.URL,
		AudiobookshelfAPIKey: "key",
		TitleMatchThreshold:  0.7,
	}

	job := &models.Job{ID: "test", Title: "Test Book", MediaType: models.MediaTypeAudiobook}
	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationNotFound {
		t.Errorf("Status = %q, want not_found (isMissing)", proof.Status)
	}
}

// --- Sonarr Checker Tests ---

func TestSonarrIsAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/health" && r.Header.Get("X-Api-Key") == "sonarr-key" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(401)
		}
	}))
	defer ts.Close()

	c := &SonarrChecker{}
	cfg := &config.Config{SonarrURL: ts.URL, SonarrAPIKey: "sonarr-key"}
	if !c.IsAvailable(context.Background(), cfg) {
		t.Error("expected available")
	}
}

func TestSonarrVerifyFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":    1.0,
				"title": "Breaking Bad",
				"year":  2008.0,
				"path":  "/tv/Breaking Bad",
				"statistics": map[string]any{
					"episodeFileCount":  62.0,
					"totalEpisodeCount": 62.0,
					"sizeOnDisk":        150000000000.0,
				},
			},
		})
	}))
	defer ts.Close()

	c := &SonarrChecker{}
	cfg := &config.Config{SonarrURL: ts.URL, SonarrAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "Breaking Bad", MediaType: models.MediaTypeTV}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found", proof.Status)
	}
	if proof.TitleMatched != "Breaking Bad" {
		t.Errorf("TitleMatched = %q", proof.TitleMatched)
	}
	if proof.FilePath != "/tv/Breaking Bad" {
		t.Errorf("FilePath = %q", proof.FilePath)
	}
}

func TestSonarrVerifyNoEpisodeFiles(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":    1.0,
				"title": "Breaking Bad",
				"path":  "/tv/Breaking Bad",
				"statistics": map[string]any{
					"episodeFileCount":  0.0,
					"totalEpisodeCount": 62.0,
					"sizeOnDisk":        0.0,
				},
			},
		})
	}))
	defer ts.Close()

	c := &SonarrChecker{}
	cfg := &config.Config{SonarrURL: ts.URL, SonarrAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "Breaking Bad", MediaType: models.MediaTypeTV}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationNotFound {
		t.Errorf("Status = %q, want not_found (no episode files)", proof.Status)
	}
}

func TestSonarrVerifyByTvdbID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":     1.0,
				"title":  "Some Different Title",
				"tvdbId": 81189.0,
				"path":   "/tv/Breaking Bad",
				"statistics": map[string]any{
					"episodeFileCount":  62.0,
					"totalEpisodeCount": 62.0,
					"sizeOnDisk":        150000000000.0,
				},
			},
		})
	}))
	defer ts.Close()

	c := &SonarrChecker{}
	cfg := &config.Config{SonarrURL: ts.URL, SonarrAPIKey: "key", TitleMatchThreshold: 0.7}
	tvdbID := 81189
	job := &models.Job{ID: "test", Title: "Breaking Bad", MediaType: models.MediaTypeTV, TvdbID: &tvdbID}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found (tvdb match)", proof.Status)
	}
}

// --- Radarr Checker Tests ---

func TestRadarrVerifyFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":      1.0,
				"title":   "The Matrix",
				"year":    1999.0,
				"imdbId":  "tt0133093",
				"hasFile": true,
				"path":    "/movies/The Matrix (1999)",
				"runtime": 136.0,
				"movieFile": map[string]any{
					"path": "/movies/The Matrix (1999)/The Matrix.mkv",
					"size": 15000000000.0,
					"quality": map[string]any{
						"quality": map[string]any{
							"name": "Bluray-1080p",
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	c := &RadarrChecker{}
	cfg := &config.Config{RadarrURL: ts.URL, RadarrAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found", proof.Status)
	}
	if proof.TitleMatched != "The Matrix" {
		t.Errorf("TitleMatched = %q", proof.TitleMatched)
	}
	if proof.FilePath == "" {
		t.Error("FilePath should not be empty")
	}
	if proof.RuntimeSeconds == nil {
		t.Error("RuntimeSeconds should not be nil")
	} else if *proof.RuntimeSeconds != 8160.0 { // 136 * 60
		t.Errorf("RuntimeSeconds = %f, want 8160", *proof.RuntimeSeconds)
	}
}

func TestRadarrVerifyNoFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":      1.0,
				"title":   "The Matrix",
				"hasFile": false,
				"path":    "/movies/The Matrix (1999)",
			},
		})
	}))
	defer ts.Close()

	c := &RadarrChecker{}
	cfg := &config.Config{RadarrURL: ts.URL, RadarrAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationNotFound {
		t.Errorf("Status = %q, want not_found (no file)", proof.Status)
	}
}

func TestRadarrVerifyByImdbID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":      1.0,
				"title":   "Different Title Entirely",
				"imdbId":  "tt0133093",
				"hasFile": true,
				"path":    "/movies/The Matrix (1999)",
				"movieFile": map[string]any{
					"path": "/movies/The Matrix (1999)/The Matrix.mkv",
				},
			},
		})
	}))
	defer ts.Close()

	c := &RadarrChecker{}
	cfg := &config.Config{RadarrURL: ts.URL, RadarrAPIKey: "key", TitleMatchThreshold: 0.7}
	job := &models.Job{
		ID: "test", Title: "The Matrix", MediaType: models.MediaTypeMovie, ImdbID: "tt0133093",
	}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found (imdb match)", proof.Status)
	}
}

// --- Kavita Checker Tests ---

func TestKavitaIsAvailable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/Server/server-info" {
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	c := &KavitaChecker{}
	cfg := &config.Config{KavitaURL: ts.URL, KavitaPass: "pass"}
	if !c.IsAvailable(context.Background(), cfg) {
		t.Error("expected available")
	}
}

func TestKavitaVerifyAuthFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/Account/login" {
			w.WriteHeader(401)
		}
	}))
	defer ts.Close()

	c := &KavitaChecker{}
	cfg := &config.Config{KavitaURL: ts.URL, KavitaUser: "admin", KavitaPass: "wrong", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "Test Comic", MediaType: models.MediaTypeComic}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationError {
		t.Errorf("Status = %q, want error (auth failure)", proof.Status)
	}
}

func TestKavitaVerifyFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/Account/login":
			json.NewEncoder(w).Encode(map[string]any{"token": "fake-token"})
		case "/api/Search/search":
			json.NewEncoder(w).Encode(map[string]any{
				"series": []map[string]any{
					{"name": "One Piece", "seriesId": 42.0},
				},
			})
		case "/api/Series/42":
			json.NewEncoder(w).Encode(map[string]any{
				"pages":      1050.0,
				"folderPath": "/comics/One Piece",
				"wordCount":  500000.0,
			})
		}
	}))
	defer ts.Close()

	c := &KavitaChecker{}
	cfg := &config.Config{KavitaURL: ts.URL, KavitaUser: "admin", KavitaPass: "pass", TitleMatchThreshold: 0.7}
	job := &models.Job{ID: "test", Title: "One Piece", MediaType: models.MediaTypeComic}

	proof := c.Verify(context.Background(), job, cfg)
	if proof.Status != models.VerificationFound {
		t.Errorf("Status = %q, want found", proof.Status)
	}
	if proof.PageCount == nil || *proof.PageCount != 1050 {
		t.Errorf("PageCount = %v", proof.PageCount)
	}
	if proof.FilePath != "/comics/One Piece" {
		t.Errorf("FilePath = %q", proof.FilePath)
	}
}

// --- Helper Tests ---

func TestHelperFunctions(t *testing.T) {
	t.Run("getStr", func(t *testing.T) {
		m := map[string]any{"name": "test", "count": 42}
		if getStr(m, "name") != "test" {
			t.Error("getStr failed for existing key")
		}
		if getStr(m, "missing") != "" {
			t.Error("getStr should return empty for missing key")
		}
		if getStr(m, "count") != "" {
			t.Error("getStr should return empty for non-string value")
		}
		if getStr(nil, "key") != "" {
			t.Error("getStr should return empty for nil map")
		}
	})

	t.Run("getNum", func(t *testing.T) {
		m := map[string]any{"f": 3.14, "i": 42, "i64": int64(100), "s": "abc"}
		if getNum(m, "f") != 3.14 {
			t.Error("getNum failed for float64")
		}
		if getNum(m, "i") != 42.0 {
			t.Error("getNum failed for int")
		}
		if getNum(m, "i64") != 100.0 {
			t.Error("getNum failed for int64")
		}
		if getNum(m, "s") != 0 {
			t.Error("getNum should return 0 for string")
		}
		if getNum(m, "missing") != 0 {
			t.Error("getNum should return 0 for missing key")
		}
		if getNum(nil, "key") != 0 {
			t.Error("getNum should return 0 for nil map")
		}
	})

	t.Run("getBool", func(t *testing.T) {
		m := map[string]any{"yes": true, "no": false, "str": "true"}
		if !getBool(m, "yes") {
			t.Error("getBool failed for true")
		}
		if getBool(m, "no") {
			t.Error("getBool failed for false")
		}
		if getBool(m, "str") {
			t.Error("getBool should return false for string 'true'")
		}
		if getBool(m, "missing") {
			t.Error("getBool should return false for missing key")
		}
		if getBool(nil, "key") {
			t.Error("getBool should return false for nil map")
		}
	})

	t.Run("getMap", func(t *testing.T) {
		inner := map[string]any{"nested": true}
		m := map[string]any{"sub": inner, "str": "abc"}
		got := getMap(m, "sub")
		if got == nil || !got["nested"].(bool) {
			t.Error("getMap failed for existing nested map")
		}
		if getMap(m, "str") != nil {
			t.Error("getMap should return nil for non-map value")
		}
		if getMap(m, "missing") != nil {
			t.Error("getMap should return nil for missing key")
		}
		if getMap(nil, "key") != nil {
			t.Error("getMap should return nil for nil map")
		}
	})

	t.Run("min64", func(t *testing.T) {
		if min64(1.0, 2.0) != 1.0 {
			t.Error("min64(1,2) should be 1")
		}
		if min64(3.0, 2.0) != 2.0 {
			t.Error("min64(3,2) should be 2")
		}
		if min64(1.5, 1.5) != 1.5 {
			t.Error("min64(1.5,1.5) should be 1.5")
		}
	})

	t.Run("errorProof", func(t *testing.T) {
		now := time.Now()
		p := errorProof("test-lib", "some error", now)
		if p.Library != "test-lib" {
			t.Errorf("Library = %q", p.Library)
		}
		if p.Status != models.VerificationError {
			t.Errorf("Status = %q", p.Status)
		}
		if p.ErrorMessage != "some error" {
			t.Errorf("ErrorMessage = %q", p.ErrorMessage)
		}
	})

	t.Run("notFoundProof", func(t *testing.T) {
		now := time.Now()
		p := notFoundProof("test-lib", now)
		if p.Library != "test-lib" {
			t.Errorf("Library = %q", p.Library)
		}
		if p.Status != models.VerificationNotFound {
			t.Errorf("Status = %q", p.Status)
		}
	})
}
