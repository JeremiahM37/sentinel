package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// Prowlarr category IDs by media type.
var categoryMap = map[models.MediaType][]int{
	models.MediaTypeMovie:     {2000, 2010, 2020, 2030, 2040, 2045, 2050, 2060},
	models.MediaTypeTV:        {5000, 5010, 5020, 5030, 5040, 5045, 5050, 5060},
	models.MediaTypeAudiobook: {3030},
	models.MediaTypeEbook:     {7020},
	models.MediaTypeComic:     {7030},
}

// ProwlarrSource searches indexers via Prowlarr and sends the best result to qBittorrent.
type ProwlarrSource struct{}

func (s *ProwlarrSource) Name() string { return "prowlarr" }

func (s *ProwlarrSource) SupportedTypes() []models.MediaType {
	return []models.MediaType{
		models.MediaTypeMovie, models.MediaTypeTV,
		models.MediaTypeAudiobook, models.MediaTypeEbook, models.MediaTypeComic,
	}
}

func (s *ProwlarrSource) IsAvailable(ctx context.Context, cfg *config.Config) bool {
	if !cfg.IsConfigured("prowlarr") {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.ProwlarrURL+"/api/v1/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Api-Key", cfg.ProwlarrAPIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (s *ProwlarrSource) SearchAndDownload(ctx context.Context, job *models.Job, cfg *config.Config) models.SourceAttempt {
	query := job.Title
	if job.Year != nil {
		query = fmt.Sprintf("%s %d", job.Title, *job.Year)
	}

	now := time.Now().UTC()
	attempt := models.SourceAttempt{
		SourceName: s.Name(),
		Query:      query,
		StartedAt:  now,
	}

	client := &http.Client{Timeout: 60 * time.Second}

	// Build search URL
	params := url.Values{}
	params.Set("query", query)
	params.Set("type", "search")
	if cats, ok := categoryMap[job.MediaType]; ok {
		for _, c := range cats {
			params.Add("categories", fmt.Sprintf("%d", c))
		}
	}

	searchURL := fmt.Sprintf("%s/api/v1/search?%s", cfg.ProwlarrURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		attempt.ErrorMessage = fmt.Sprintf("request error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}
	req.Header.Set("X-Api-Key", cfg.ProwlarrAPIKey)

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

	var results []map[string]any
	if err := json.Unmarshal(body, &results); err != nil {
		attempt.ErrorMessage = fmt.Sprintf("JSON decode error: %v", err)
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	if len(results) == 0 {
		attempt.ErrorMessage = "No results from Prowlarr search"
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	// Sort by seeders descending
	sort.Slice(results, func(i, j int) bool {
		si := getNumber(results[i], "seeders")
		sj := getNumber(results[j], "seeders")
		return si > sj
	})

	best := results[0]
	downloadURL := getString(best, "downloadUrl", "")
	if downloadURL == "" {
		downloadURL = getString(best, "magnetUrl", "")
	}
	if downloadURL == "" {
		attempt.ErrorMessage = "Best result has no download URL"
		attempt.FinishedAt = timePtr(time.Now().UTC())
		return attempt
	}

	// Send to qBittorrent if configured
	if cfg.IsConfigured("qbittorrent") {
		dlID, err := sendToQBittorrent(ctx, cfg, downloadURL, job.Title)
		if err != nil {
			attempt.ErrorMessage = fmt.Sprintf("Failed to add torrent: %v", err)
		} else {
			attempt.Success = true
			attempt.DownloadID = dlID
		}
	} else {
		attempt.Success = true
		attempt.DownloadID = downloadURL
	}

	attempt.FinishedAt = timePtr(time.Now().UTC())
	slog.Info("Prowlarr found release",
		"title", getString(best, "title", "?"),
		"seeders", getNumber(best, "seeders"),
		"job_title", job.Title,
	)

	return attempt
}

func sendToQBittorrent(ctx context.Context, cfg *config.Config, dlURL, title string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Login
	loginBody := strings.NewReader(fmt.Sprintf("username=%s&password=%s",
		url.QueryEscape(cfg.QBittorrentUser), url.QueryEscape(cfg.QBittorrentPass)))
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.QBittorrentURL+"/api/v2/auth/login", loginBody)
	if err != nil {
		return "", err
	}
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	loginResp, err := client.Do(loginReq)
	if err != nil {
		return "", fmt.Errorf("qBit login: %w", err)
	}
	loginRespBody, _ := io.ReadAll(loginResp.Body)
	loginResp.Body.Close()

	if loginResp.StatusCode != 200 || !strings.Contains(string(loginRespBody), "Ok") {
		return "", fmt.Errorf("qBit login failed (status %d)", loginResp.StatusCode)
	}

	// Extract SID cookie
	var sid string
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "SID" {
			sid = cookie.Value
			break
		}
	}

	// Add torrent
	tag := fmt.Sprintf("sentinel:%s", title)
	addBody := strings.NewReader(fmt.Sprintf("urls=%s&tags=%s",
		url.QueryEscape(dlURL), url.QueryEscape(tag)))
	addReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.QBittorrentURL+"/api/v2/torrents/add", addBody)
	if err != nil {
		return "", err
	}
	addReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sid != "" {
		addReq.AddCookie(&http.Cookie{Name: "SID", Value: sid})
	}

	addResp, err := client.Do(addReq)
	if err != nil {
		return "", fmt.Errorf("qBit add: %w", err)
	}
	addRespBody, _ := io.ReadAll(addResp.Body)
	addResp.Body.Close()

	if addResp.StatusCode == 200 && strings.Contains(string(addRespBody), "Ok") {
		return tag, nil
	}
	return "", fmt.Errorf("qBit add failed (status %d): %s", addResp.StatusCode, string(addRespBody))
}
