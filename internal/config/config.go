// Package config provides all Sentinel configuration, loaded from environment variables.
package config

import (
	"os"
	"strconv"
)

// Config holds all Sentinel configuration. Every field can be set via an environment variable.
type Config struct {
	// Core
	Port     int
	DBPath   string
	LogLevel string

	// Jellyfin
	JellyfinURL    string
	JellyfinAPIKey string

	// Jellyseerr
	JellyseerrURL    string
	JellyseerrAPIKey string

	// Sonarr
	SonarrURL    string
	SonarrAPIKey string

	// Radarr
	RadarrURL    string
	RadarrAPIKey string

	// qBittorrent
	QBittorrentURL  string
	QBittorrentUser string
	QBittorrentPass string

	// Prowlarr
	ProwlarrURL    string
	ProwlarrAPIKey string

	// Audiobookshelf
	AudiobookshelfURL    string
	AudiobookshelfAPIKey string

	// Kavita
	KavitaURL  string
	KavitaUser string
	KavitaPass string

	// Calibre-Web OPDS
	CalibreOPDSURL string

	// Notifications
	DiscordWebhookURL      string
	NotificationWebhookURL string

	// Guardian behavior
	VerifyIntervalSeconds int
	VerifyMaxChecks       int
	MaxSourcesPerType     int
	TitleMatchThreshold   float64
}

// IsConfigured checks whether a given service has its required config populated.
func (c *Config) IsConfigured(service string) bool {
	switch service {
	case "jellyfin":
		return c.JellyfinURL != "" && c.JellyfinAPIKey != ""
	case "jellyseerr":
		return c.JellyseerrURL != "" && c.JellyseerrAPIKey != ""
	case "sonarr":
		return c.SonarrURL != "" && c.SonarrAPIKey != ""
	case "radarr":
		return c.RadarrURL != "" && c.RadarrAPIKey != ""
	case "qbittorrent":
		return c.QBittorrentURL != "" && c.QBittorrentPass != ""
	case "prowlarr":
		return c.ProwlarrURL != "" && c.ProwlarrAPIKey != ""
	case "audiobookshelf":
		return c.AudiobookshelfURL != "" && c.AudiobookshelfAPIKey != ""
	case "kavita":
		return c.KavitaURL != "" && c.KavitaPass != ""
	case "calibre":
		return c.CalibreOPDSURL != ""
	case "discord":
		return c.DiscordWebhookURL != ""
	case "webhook":
		return c.NotificationWebhookURL != ""
	default:
		return false
	}
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:     envInt("SENTINEL_PORT", 9200),
		DBPath:   envStr("SENTINEL_DB_PATH", "/data/sentinel.db"),
		LogLevel: envStr("SENTINEL_LOG_LEVEL", "INFO"),

		JellyfinURL:    envStr("JELLYFIN_URL", ""),
		JellyfinAPIKey: envStr("JELLYFIN_API_KEY", ""),

		JellyseerrURL:    envStr("JELLYSEERR_URL", ""),
		JellyseerrAPIKey: envStr("JELLYSEERR_API_KEY", ""),

		SonarrURL:    envStr("SONARR_URL", ""),
		SonarrAPIKey: envStr("SONARR_API_KEY", ""),

		RadarrURL:    envStr("RADARR_URL", ""),
		RadarrAPIKey: envStr("RADARR_API_KEY", ""),

		QBittorrentURL:  envStr("QBITTORRENT_URL", ""),
		QBittorrentUser: envStr("QBITTORRENT_USER", "admin"),
		QBittorrentPass: envStr("QBITTORRENT_PASS", ""),

		ProwlarrURL:    envStr("PROWLARR_URL", ""),
		ProwlarrAPIKey: envStr("PROWLARR_API_KEY", ""),

		AudiobookshelfURL:    envStr("AUDIOBOOKSHELF_URL", ""),
		AudiobookshelfAPIKey: envStr("AUDIOBOOKSHELF_API_KEY", ""),

		KavitaURL:  envStr("KAVITA_URL", ""),
		KavitaUser: envStr("KAVITA_USER", "admin"),
		KavitaPass: envStr("KAVITA_PASS", ""),

		CalibreOPDSURL: envStr("CALIBRE_OPDS_URL", ""),

		DiscordWebhookURL:      envStr("DISCORD_WEBHOOK_URL", ""),
		NotificationWebhookURL: envStr("NOTIFICATION_WEBHOOK_URL", ""),

		VerifyIntervalSeconds: envInt("VERIFY_INTERVAL_SECONDS", 60),
		VerifyMaxChecks:       envInt("VERIFY_MAX_CHECKS", 30),
		MaxSourcesPerType:     envInt("MAX_SOURCES_PER_TYPE", 5),
		TitleMatchThreshold:   envFloat("TITLE_MATCH_THRESHOLD", 0.7),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
