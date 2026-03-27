package libraries

import (
	"bytes"
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

// KavitaChecker verifies comics and ebooks exist in Kavita with definitive
// proof: pages > 0 and the folder path on disk.
type KavitaChecker struct{}

func (c *KavitaChecker) Name() string { return "kavita" }

func (c *KavitaChecker) SupportedTypes() []models.MediaType {
	return []models.MediaType{models.MediaTypeComic, models.MediaTypeEbook}
}

func (c *KavitaChecker) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("kavita") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.KavitaURL+"/api/Server/server-info", nil)
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

func (c *KavitaChecker) authenticate(ctx context.Context, client *http.Client, cfg *config.Config) string {
	loginBody, _ := json.Marshal(map[string]string{
		"username": cfg.KavitaUser,
		"password": cfg.KavitaPass,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.KavitaURL+"/api/Account/login", bytes.NewReader(loginBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("Kavita auth failed", "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}
	if token, ok := data["token"].(string); ok {
		return token
	}
	return ""
}

func (c *KavitaChecker) Verify(ctx context.Context, job *models.Job, cfg *config.Config) models.VerificationProof {
	now := time.Now().UTC()
	client := &http.Client{Timeout: 30 * time.Second}

	token := c.authenticate(ctx, client, cfg)
	if token == "" {
		return errorProof("kavita", "Authentication failed", now)
	}

	// Search series
	searchBody, _ := json.Marshal(map[string]string{
		"queryString": job.Title,
	})
	searchReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.KavitaURL+"/api/Search/search", bytes.NewReader(searchBody))
	if err != nil {
		return errorProof("kavita", err.Error(), now)
	}
	searchReq.Header.Set("Authorization", "Bearer "+token)
	searchReq.Header.Set("Content-Type", "application/json")

	searchResp, err := client.Do(searchReq)
	if err != nil {
		slog.Warn("Kavita verification error", "title", job.Title, "error", err)
		return errorProof("kavita", err.Error(), now)
	}
	defer searchResp.Body.Close()
	searchRespBody, _ := io.ReadAll(searchResp.Body)

	var searchData struct {
		Series []map[string]any `json:"series"`
	}
	if err := json.Unmarshal(searchRespBody, &searchData); err != nil {
		return errorProof("kavita", err.Error(), now)
	}

	for _, series := range searchData.Series {
		seriesName := getStr(series, "name")
		score := titleutil.TitleMatchScore(job.Title, seriesName)

		if score < cfg.TitleMatchThreshold {
			continue
		}

		seriesID := int(getNum(series, "seriesId"))
		if seriesID == 0 {
			continue
		}

		// Get series detail
		detailReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/api/Series/%d", cfg.KavitaURL, seriesID), nil)
		if err != nil {
			continue
		}
		detailReq.Header.Set("Authorization", "Bearer "+token)

		detailResp, err := client.Do(detailReq)
		if err != nil {
			continue
		}
		detailBody, _ := io.ReadAll(detailResp.Body)
		detailResp.Body.Close()

		var detail map[string]any
		if err := json.Unmarshal(detailBody, &detail); err != nil {
			continue
		}

		pages := int(getNum(detail, "pages"))
		folderPath := getStr(detail, "folderPath")

		if pages <= 0 {
			continue
		}

		return models.VerificationProof{
			Library:      "kavita",
			Status:       models.VerificationFound,
			TitleMatched: seriesName,
			FilePath:     folderPath,
			PageCount:    &pages,
			Extra: map[string]any{
				"kavita_series_id": seriesID,
				"match_score":      score,
				"word_count":       getNum(detail, "wordCount"),
			},
			CheckedAt: now,
		}
	}

	return notFoundProof("kavita", now)
}
