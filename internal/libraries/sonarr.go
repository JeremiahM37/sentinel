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

// SonarrChecker verifies TV shows in Sonarr's library.
// Proof includes: file path, episode file count, and monitored status.
type SonarrChecker struct{}

func (c *SonarrChecker) Name() string { return "sonarr" }

func (c *SonarrChecker) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeTV}
}

func (c *SonarrChecker) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("sonarr") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.SonarrURL+"/api/v3/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Api-Key", cfg.SonarrAPIKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *SonarrChecker) Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof {
	now := time.Now().UTC()
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.SonarrURL+"/api/v3/series", nil)
	if err != nil {
		return errorProof("sonarr", err.Error(), now)
	}
	req.Header.Set("X-Api-Key", cfg.SonarrAPIKey)

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("Sonarr verification error", "title", job.Title, "error", err)
		return errorProof("sonarr", err.Error(), now)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var allSeries []map[string]any
	if err := json.Unmarshal(body, &allSeries); err != nil {
		return errorProof("sonarr", err.Error(), now)
	}

	var bestMatch map[string]any
	var bestScore float64

	for _, series := range allSeries {
		seriesTitle := getStr(series, "title")
		score := titleutil.TitleMatchScore(job.Title, seriesTitle)

		if job.TvdbID != nil {
			if tvdbID, ok := series["tvdbId"].(float64); ok && int(tvdbID) == *job.TvdbID {
				score = 1.0
			}
		}
		if job.Year != nil {
			if year, ok := series["year"].(float64); ok && int(year) == *job.Year {
				score = min64(1.0, score+0.15)
			}
		}

		if score > bestScore {
			bestScore = score
			bestMatch = series
		}
	}

	if bestMatch == nil || bestScore < cfg.TitleMatchThreshold {
		return notFoundProof("sonarr", now)
	}

	statistics := getMap(bestMatch, "statistics")
	episodeFileCount := int(getNum(statistics, "episodeFileCount"))
	totalEpisodes := int(getNum(statistics, "totalEpisodeCount"))
	sizeOnDisk := int64(getNum(statistics, "sizeOnDisk"))
	path := getStr(bestMatch, "path")

	if episodeFileCount == 0 {
		return models.VerificationProof{
			Library:      "sonarr",
			Status:       models.VerificationNotFound,
			TitleMatched: getStr(bestMatch, "title"),
			Extra: map[string]any{
				"sonarr_id":   getNum(bestMatch, "id"),
				"reason":      "Series exists but has no episode files",
				"match_score": bestScore,
			},
			CheckedAt: now,
		}
	}

	return models.VerificationProof{
		Library:      "sonarr",
		Status:       models.VerificationFound,
		TitleMatched: getStr(bestMatch, "title"),
		FilePath:     path,
		Extra: map[string]any{
			"sonarr_id":          fmt.Sprintf("%.0f", getNum(bestMatch, "id")),
			"episode_file_count": episodeFileCount,
			"total_episodes":     totalEpisodes,
			"size_on_disk":       sizeOnDisk,
			"match_score":        bestScore,
		},
		CheckedAt: now,
	}
}
