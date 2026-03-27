package libraries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
	"github.com/JeremiahM37/sentinel/internal/titleutil"
)

// JellyfinChecker verifies movies and TV shows exist in Jellyfin with
// definitive proof: file path on disk, runtime in seconds, and media sources.
type JellyfinChecker struct{}

func (c *JellyfinChecker) Name() string { return "jellyfin" }

func (c *JellyfinChecker) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeMovie, models.MediaTypeTV}
}

func (c *JellyfinChecker) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("jellyfin") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.JellyfinURL+"/System/Info/Public", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *JellyfinChecker) Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof {
	now := time.Now().UTC()

	includeTypes := "Movie"
	if job.MediaType == models.MediaTypeTV {
		includeTypes = "Series"
	}

	params := url.Values{}
	params.Set("searchTerm", job.Title)
	params.Set("IncludeItemTypes", includeTypes)
	params.Set("Recursive", "true")
	params.Set("Fields", "Path,MediaSources,Overview")
	params.Set("Limit", "10")

	reqURL := fmt.Sprintf("%s/Items?%s", cfg.JellyfinURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return errorProof("jellyfin", err.Error(), now)
	}
	req.Header.Set("X-Emby-Token", cfg.JellyfinAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("Jellyfin verification error", "title", job.Title, "error", err)
		return errorProof("jellyfin", err.Error(), now)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return errorProof("jellyfin", fmt.Sprintf("status %d", resp.StatusCode), now)
	}

	var data struct {
		Items []map[string]any `json:"Items"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return errorProof("jellyfin", err.Error(), now)
	}

	if len(data.Items) == 0 {
		return notFoundProof("jellyfin", now)
	}

	threshold := cfg.TitleMatchThreshold
	var bestItem map[string]any
	var bestScore float64

	for _, item := range data.Items {
		itemTitle := getStr(item, "Name")
		score := titleutil.TitleMatchScore(job.Title, itemTitle)

		if job.Year != nil {
			if prodYear, ok := item["ProductionYear"].(float64); ok && int(prodYear) == *job.Year {
				score = min64(1.0, score+0.2)
			}
		}

		if score > bestScore {
			bestScore = score
			bestItem = item
		}
	}

	if bestItem == nil || bestScore < threshold {
		return models.VerificationProof{
			Library:   "jellyfin",
			Status:    models.VerificationNotFound,
			CheckedAt: now,
			Extra: map[string]any{
				"best_score": bestScore,
				"threshold":  threshold,
			},
		}
	}

	filePath := getStr(bestItem, "Path")
	runtimeTicks := getNum(bestItem, "RunTimeTicks")
	var runtimeSeconds *float64
	if runtimeTicks > 0 {
		rs := runtimeTicks / 10_000_000
		runtimeSeconds = &rs
	}

	var sourcePaths []string
	if ms, ok := bestItem["MediaSources"].([]any); ok {
		for _, s := range ms {
			if sm, ok := s.(map[string]any); ok {
				if p := getStr(sm, "Path"); p != "" {
					sourcePaths = append(sourcePaths, p)
				}
			}
		}
	}

	if filePath == "" && len(sourcePaths) > 0 {
		filePath = sourcePaths[0]
	}

	return models.VerificationProof{
		Library:        "jellyfin",
		Status:         models.VerificationFound,
		TitleMatched:   getStr(bestItem, "Name"),
		FilePath:       filePath,
		RuntimeSeconds: runtimeSeconds,
		Extra: map[string]any{
			"jellyfin_id":        getStr(bestItem, "Id"),
			"year":               bestItem["ProductionYear"],
			"match_score":        bestScore,
			"media_source_count": len(sourcePaths),
			"source_paths":       sourcePaths,
		},
		CheckedAt: now,
	}
}
