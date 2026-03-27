package libraries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
	"github.com/JeremiahM37/sentinel/internal/titleutil"
)

// RadarrChecker verifies movies in Radarr's library.
// Proof includes: file path, size on disk, and quality profile.
type RadarrChecker struct{}

func (c *RadarrChecker) Name() string { return "radarr" }

func (c *RadarrChecker) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeMovie}
}

func (c *RadarrChecker) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("radarr") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.RadarrURL+"/api/v3/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Api-Key", cfg.RadarrAPIKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *RadarrChecker) Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof {
	now := time.Now().UTC()
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.RadarrURL+"/api/v3/movie", nil)
	if err != nil {
		return errorProof("radarr", err.Error(), now)
	}
	req.Header.Set("X-Api-Key", cfg.RadarrAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("Radarr verification error", "title", job.Title, "error", err)
		return errorProof("radarr", err.Error(), now)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var allMovies []map[string]any
	if err := json.Unmarshal(body, &allMovies); err != nil {
		return errorProof("radarr", err.Error(), now)
	}

	var bestMatch map[string]any
	var bestScore float64

	for _, movie := range allMovies {
		movieTitle := getStr(movie, "title")
		score := titleutil.TitleMatchScore(job.Title, movieTitle)

		if job.ImdbID != "" {
			if imdbID := getStr(movie, "imdbId"); imdbID == job.ImdbID {
				score = 1.0
			}
		}
		if job.Year != nil {
			if year, ok := movie["year"].(float64); ok && int(year) == *job.Year {
				score = min64(1.0, score+0.15)
			}
		}

		if score > bestScore {
			bestScore = score
			bestMatch = movie
		}
	}

	if bestMatch == nil || bestScore < cfg.TitleMatchThreshold {
		return notFoundProof("radarr", now)
	}

	hasFile := getBool(bestMatch, "hasFile")
	movieFile := getMap(bestMatch, "movieFile")
	filePath := getStr(movieFile, "path")
	size := int64(getNum(movieFile, "size"))
	runtime := getNum(bestMatch, "runtime")

	if !hasFile {
		return models.VerificationProof{
			Library:      "radarr",
			Status:       models.VerificationNotFound,
			TitleMatched: getStr(bestMatch, "title"),
			Extra: map[string]any{
				"radarr_id":   getNum(bestMatch, "id"),
				"reason":      "Movie exists in Radarr but has no file on disk",
				"match_score": bestScore,
			},
			CheckedAt: now,
		}
	}

	var runtimeSeconds *float64
	if runtime > 0 {
		rs := runtime * 60
		runtimeSeconds = &rs
	}

	if filePath == "" {
		filePath = getStr(bestMatch, "path")
	}

	// Extract quality name
	qualityName := "unknown"
	if q := getMap(movieFile, "quality"); q != nil {
		if qi := getMap(q, "quality"); qi != nil {
			if name := getStr(qi, "name"); name != "" {
				qualityName = name
			}
		}
	}

	return models.VerificationProof{
		Library:        "radarr",
		Status:         models.VerificationFound,
		TitleMatched:   getStr(bestMatch, "title"),
		FilePath:       filePath,
		RuntimeSeconds: runtimeSeconds,
		Extra: map[string]any{
			"radarr_id":    fmt.Sprintf("%.0f", getNum(bestMatch, "id")),
			"imdb_id":      getStr(bestMatch, "imdbId"),
			"size_on_disk": size,
			"quality":      qualityName,
			"match_score":  bestScore,
		},
		CheckedAt: now,
	}
}
