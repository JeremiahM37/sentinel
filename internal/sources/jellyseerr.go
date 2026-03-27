package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// JellyseerrSource requests movies and TV shows via Jellyseerr,
// which delegates to Radarr/Sonarr.
type JellyseerrSource struct{}

func (s *JellyseerrSource) Name() string { return "jellyseerr" }

func (s *JellyseerrSource) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeMovie, models.MediaTypeTV}
}

func (s *JellyseerrSource) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("jellyseerr") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.JellyseerrURL+"/api/v1/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Api-Key", cfg.JellyseerrAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (s *JellyseerrSource) SearchAndDownload(ctx context.Context, job *models.Job, cfg *config.Config) models.SourceAttempt {
	now := time.Now().UTC()
	attempt := models.SourceAttempt{
		SourceName: s.Name(),
		Query:      job.Title,
		StartedAt:  now,
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Search TMDB via Jellyseerr
	searchURL := fmt.Sprintf("%s/api/v1/search?query=%s&page=1&language=en",
		cfg.JellyseerrURL, urlEncode(job.Title))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		attempt.ErrorMessage = fmt.Sprintf("request error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}
	req.Header.Set("X-Api-Key", cfg.JellyseerrAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		attempt.ErrorMessage = fmt.Sprintf("HTTP error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		attempt.ErrorMessage = fmt.Sprintf("search returned %d", resp.StatusCode)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	var searchData struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(body, &searchData); err != nil {
		attempt.ErrorMessage = fmt.Sprintf("JSON decode error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	if len(searchData.Results) == 0 {
		attempt.ErrorMessage = "No results found in Jellyseerr search"
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	best := pickBestResult(searchData.Results, job)
	if best == nil {
		attempt.ErrorMessage = "No suitable match found in search results"
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	mediaTypeJS := getString(best, "mediaType", "movie")
	tmdbID := getNumber(best, "id")

	// Request the media
	requestBody, _ := json.Marshal(map[string]any{
		"mediaType": mediaTypeJS,
		"mediaId":   tmdbID,
	})
	reqPost, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.JellyseerrURL+"/api/v1/request", bytes.NewReader(requestBody))
	if err != nil {
		attempt.ErrorMessage = fmt.Sprintf("request error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}
	reqPost.Header.Set("X-Api-Key", cfg.JellyseerrAPIKey)
	reqPost.Header.Set("Content-Type", "application/json")

	resp2, err := client.Do(reqPost)
	if err != nil {
		attempt.ErrorMessage = fmt.Sprintf("HTTP error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode >= 400 {
		attempt.ErrorMessage = fmt.Sprintf("request creation returned %d", resp2.StatusCode)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	var reqData map[string]any
	_ = json.Unmarshal(body2, &reqData)

	attempt.Success = true
	if id, ok := reqData["id"]; ok {
		attempt.DownloadID = fmt.Sprintf("%v", id)
	} else {
		attempt.DownloadID = fmt.Sprintf("%v", tmdbID)
	}
	attempt.FinishedAt = timePtr(time.Now().UTC())

	slog.Info("Jellyseerr request created",
		"title", job.Title, "tmdb_id", tmdbID)

	return attempt
}

func pickBestResult(results []map[string]any, job *models.Job) map[string]any {
	targetType := "movie"
	if job.MediaType == models.MediaTypeTV {
		targetType = "tv"
	}

	var candidates []map[string]any
	for _, r := range results {
		if getString(r, "mediaType", "") == targetType {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		candidates = results
	}

	if job.Year != nil {
		yearStr := fmt.Sprintf("%d", *job.Year)
		for _, c := range candidates {
			release := getString(c, "releaseDate", "")
			if release == "" {
				release = getString(c, "firstAirDate", "")
			}
			if strings.HasPrefix(release, yearStr) {
				return c
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}
	return nil
}

func getString(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func getNumber(m map[string]any, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return 0
}

func urlEncode(s string) string {
	// Simple percent-encoding for query parameter values
	var b strings.Builder
	for _, c := range s {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~':
			b.WriteRune(c)
		case c == ' ':
			b.WriteString("%20")
		default:
			b.WriteString(fmt.Sprintf("%%%02X", c))
		}
	}
	return b.String()
}

func timePtr(t time.Time) *time.Time {
	return &t
}
