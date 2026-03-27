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

// AudiobookshelfChecker verifies audiobooks exist in Audiobookshelf with
// definitive proof: isMissing=false, numAudioFiles > 0, duration in seconds.
type AudiobookshelfChecker struct{}

func (c *AudiobookshelfChecker) Name() string { return "audiobookshelf" }

func (c *AudiobookshelfChecker) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeAudiobook}
}

func (c *AudiobookshelfChecker) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("audiobookshelf") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.AudiobookshelfURL+"/ping", nil)
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

func (c *AudiobookshelfChecker) Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof {
	now := time.Now().UTC()
	client := &http.Client{Timeout: 30 * time.Second}

	// Get libraries
	libReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.AudiobookshelfURL+"/api/libraries", nil)
	if err != nil {
		return errorProof("audiobookshelf", err.Error(), now)
	}
	libReq.Header.Set("Authorization", "Bearer "+cfg.AudiobookshelfAPIKey)

	libResp, err := client.Do(libReq)
	if err != nil {
		slog.Warn("Audiobookshelf verification error", "title", job.Title, "error", err)
		return errorProof("audiobookshelf", err.Error(), now)
	}
	defer libResp.Body.Close()
	libBody, _ := io.ReadAll(libResp.Body)

	var libData struct {
		Libraries []struct {
			ID string `json:"id"`
		} `json:"libraries"`
	}
	if err := json.Unmarshal(libBody, &libData); err != nil {
		return errorProof("audiobookshelf", err.Error(), now)
	}

	for _, lib := range libData.Libraries {
		// Search within library
		searchURL := fmt.Sprintf("%s/api/libraries/%s/search?q=%s&limit=10",
			cfg.AudiobookshelfURL, lib.ID, url.QueryEscape(job.Title))
		searchReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
		if err != nil {
			continue
		}
		searchReq.Header.Set("Authorization", "Bearer "+cfg.AudiobookshelfAPIKey)

		searchResp, err := client.Do(searchReq)
		if err != nil {
			continue
		}
		searchBody, _ := io.ReadAll(searchResp.Body)
		searchResp.Body.Close()

		var searchData struct {
			Book []struct {
				LibraryItem map[string]any `json:"libraryItem"`
			} `json:"book"`
		}
		if err := json.Unmarshal(searchBody, &searchData); err != nil {
			continue
		}

		for _, result := range searchData.Book {
			item := result.LibraryItem
			media := getMap(item, "media")
			metadata := getMap(media, "metadata")

			itemTitle := getStr(metadata, "title")
			score := titleutil.TitleMatchScore(job.Title, itemTitle)

			// Author match bonus
			if job.Author != "" {
				itemAuthor := getStr(metadata, "authorName")
				if itemAuthor != "" && titleutil.TitleMatchScore(job.Author, itemAuthor) > 0.5 {
					score = min64(1.0, score+0.2)
				}
			}

			if score < cfg.TitleMatchThreshold {
				continue
			}

			isMissing := getBool(item, "isMissing")
			numAudioFiles := int(getNum(media, "numAudioFiles"))
			duration := getNum(media, "duration")
			folderPath := getStr(item, "path")

			if isMissing || numAudioFiles == 0 {
				continue
			}

			runtimeSec := duration
			return models.VerificationProof{
				Library:        "audiobookshelf",
				Status:         models.VerificationFound,
				TitleMatched:   itemTitle,
				FilePath:       folderPath,
				RuntimeSeconds: &runtimeSec,
				AudioFileCount: &numAudioFiles,
				Extra: map[string]any{
					"abs_id":      getStr(item, "id"),
					"author":      getStr(metadata, "authorName"),
					"match_score": score,
					"is_missing":  isMissing,
				},
				CheckedAt: now,
			}
		}
	}

	return notFoundProof("audiobookshelf", now)
}
