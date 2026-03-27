package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
)

// completionStates are qBittorrent states that indicate a download is done.
var completionStates = map[string]bool{
	"uploading":  true,
	"stalledUP":  true,
	"pausedUP":   true,
	"stoppedUP":  true,
	"queuedUP":   true,
	"checkingUP": true,
	"forcedUP":   true,
}

// QBittorrentMonitor tracks download progress in qBittorrent.
// It is not a Source (doesn't initiate downloads) but is used by the guardian
// to check whether a download has completed.
type QBittorrentMonitor struct {
	cfg    *config.Config
	cookie string
}

// NewQBittorrentMonitor creates a new monitor instance.
func NewQBittorrentMonitor(cfg *config.Config) *QBittorrentMonitor {
	return &QBittorrentMonitor{cfg: cfg}
}

func (m *QBittorrentMonitor) ensureAuth(ctx context.Context, client *http.Client) bool {
	if m.cookie != "" {
		return true
	}

	loginBody := strings.NewReader(fmt.Sprintf("username=%s&password=%s",
		url.QueryEscape(m.cfg.QBittorrentUser), url.QueryEscape(m.cfg.QBittorrentPass)))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		m.cfg.QBittorrentURL+"/api/v2/auth/login", loginBody)
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("qBittorrent auth failed", "error", err)
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 200 && strings.Contains(string(body), "Ok") {
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "SID" {
				m.cookie = cookie.Value
				break
			}
		}
		return true
	}
	return false
}

// IsAvailable checks if qBittorrent is reachable and we can authenticate.
func (m *QBittorrentMonitor) IsAvailable(ctx context.Context) bool {
	if !m.cfg.IsConfigured("qbittorrent") {
		return false
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return m.ensureAuth(ctx, client)
}

// TorrentStatus contains the key fields from a qBittorrent torrent.
type TorrentStatus struct {
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	Downloaded int64   `json:"downloaded"`
	ETA        int64   `json:"eta"`
	Seeds      int     `json:"seeds"`
	Hash       string  `json:"hash"`
}

// GetTorrentStatus returns the status of a torrent by its Sentinel tag.
// Returns nil if the torrent is not found or an error occurs.
func (m *QBittorrentMonitor) GetTorrentStatus(ctx context.Context, tag string) *TorrentStatus {
	client := &http.Client{Timeout: 15 * time.Second}
	if !m.ensureAuth(ctx, client) {
		return nil
	}

	reqURL := fmt.Sprintf("%s/api/v2/torrents/info?tag=%s",
		m.cfg.QBittorrentURL, url.QueryEscape(tag))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil
	}
	if m.cookie != "" {
		req.AddCookie(&http.Cookie{Name: "SID", Value: m.cookie})
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("qBittorrent status check failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	var torrents []map[string]any
	if err := json.Unmarshal(body, &torrents); err != nil || len(torrents) == 0 {
		return nil
	}

	t := torrents[0]
	return &TorrentStatus{
		State:      getString(t, "state", "unknown"),
		Progress:   getNumber(t, "progress"),
		Name:       getString(t, "name", ""),
		Size:       int64(getNumber(t, "size")),
		Downloaded: int64(getNumber(t, "downloaded")),
		ETA:        int64(getNumber(t, "eta")),
		Seeds:      int(getNumber(t, "num_seeds")),
		Hash:       getString(t, "hash", ""),
	}
}

// IsDownloadComplete checks if a tagged torrent has finished downloading.
// Returns: true=complete, false=still downloading, nil pointer means not found.
func (m *QBittorrentMonitor) IsDownloadComplete(ctx context.Context, tag string) *bool {
	status := m.GetTorrentStatus(ctx, tag)
	if status == nil {
		return nil
	}

	complete := completionStates[status.State] || status.Progress >= 1.0
	return &complete
}
