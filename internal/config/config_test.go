package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear all relevant env vars
	envVars := []string{
		"SENTINEL_PORT", "SENTINEL_DB_PATH", "SENTINEL_LOG_LEVEL",
		"JELLYFIN_URL", "JELLYFIN_API_KEY",
		"JELLYSEERR_URL", "JELLYSEERR_API_KEY",
		"SONARR_URL", "SONARR_API_KEY",
		"RADARR_URL", "RADARR_API_KEY",
		"QBITTORRENT_URL", "QBITTORRENT_USER", "QBITTORRENT_PASS",
		"PROWLARR_URL", "PROWLARR_API_KEY",
		"AUDIOBOOKSHELF_URL", "AUDIOBOOKSHELF_API_KEY",
		"KAVITA_URL", "KAVITA_USER", "KAVITA_PASS",
		"CALIBRE_OPDS_URL",
		"DISCORD_WEBHOOK_URL", "NOTIFICATION_WEBHOOK_URL",
		"VERIFY_INTERVAL_SECONDS", "VERIFY_MAX_CHECKS",
		"MAX_SOURCES_PER_TYPE", "TITLE_MATCH_THRESHOLD",
	}
	for _, key := range envVars {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Port != 9200 {
		t.Errorf("Port = %d, want 9200", cfg.Port)
	}
	if cfg.DBPath != "/data/sentinel.db" {
		t.Errorf("DBPath = %q, want /data/sentinel.db", cfg.DBPath)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want INFO", cfg.LogLevel)
	}
	if cfg.QBittorrentUser != "admin" {
		t.Errorf("QBittorrentUser = %q, want admin", cfg.QBittorrentUser)
	}
	if cfg.KavitaUser != "admin" {
		t.Errorf("KavitaUser = %q, want admin", cfg.KavitaUser)
	}
	if cfg.VerifyIntervalSeconds != 60 {
		t.Errorf("VerifyIntervalSeconds = %d, want 60", cfg.VerifyIntervalSeconds)
	}
	if cfg.VerifyMaxChecks != 30 {
		t.Errorf("VerifyMaxChecks = %d, want 30", cfg.VerifyMaxChecks)
	}
	if cfg.MaxSourcesPerType != 5 {
		t.Errorf("MaxSourcesPerType = %d, want 5", cfg.MaxSourcesPerType)
	}
	if cfg.TitleMatchThreshold != 0.7 {
		t.Errorf("TitleMatchThreshold = %f, want 0.7", cfg.TitleMatchThreshold)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("SENTINEL_PORT", "8080")
	t.Setenv("SENTINEL_DB_PATH", "/tmp/test.db")
	t.Setenv("SENTINEL_LOG_LEVEL", "DEBUG")
	t.Setenv("JELLYFIN_URL", "http://jellyfin:8096")
	t.Setenv("JELLYFIN_API_KEY", "jf-api-key")
	t.Setenv("VERIFY_INTERVAL_SECONDS", "120")
	t.Setenv("TITLE_MATCH_THRESHOLD", "0.85")

	cfg := Load()

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.LogLevel != "DEBUG" {
		t.Errorf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.JellyfinURL != "http://jellyfin:8096" {
		t.Errorf("JellyfinURL = %q", cfg.JellyfinURL)
	}
	if cfg.JellyfinAPIKey != "jf-api-key" {
		t.Errorf("JellyfinAPIKey = %q", cfg.JellyfinAPIKey)
	}
	if cfg.VerifyIntervalSeconds != 120 {
		t.Errorf("VerifyIntervalSeconds = %d", cfg.VerifyIntervalSeconds)
	}
	if cfg.TitleMatchThreshold != 0.85 {
		t.Errorf("TitleMatchThreshold = %f", cfg.TitleMatchThreshold)
	}
}

func TestLoadInvalidEnvValues(t *testing.T) {
	t.Setenv("SENTINEL_PORT", "not_a_number")
	t.Setenv("TITLE_MATCH_THRESHOLD", "not_a_float")
	t.Setenv("VERIFY_INTERVAL_SECONDS", "")

	cfg := Load()

	// Should fall back to defaults
	if cfg.Port != 9200 {
		t.Errorf("Port = %d, want fallback 9200", cfg.Port)
	}
	if cfg.TitleMatchThreshold != 0.7 {
		t.Errorf("TitleMatchThreshold = %f, want fallback 0.7", cfg.TitleMatchThreshold)
	}
	if cfg.VerifyIntervalSeconds != 60 {
		t.Errorf("VerifyIntervalSeconds = %d, want fallback 60", cfg.VerifyIntervalSeconds)
	}
}

func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		service  string
		cfg      Config
		expected bool
	}{
		{
			name:    "jellyfin configured",
			service: "jellyfin",
			cfg:     Config{JellyfinURL: "http://localhost:8096", JellyfinAPIKey: "key123"},
			expected: true,
		},
		{
			name:    "jellyfin missing key",
			service: "jellyfin",
			cfg:     Config{JellyfinURL: "http://localhost:8096"},
			expected: false,
		},
		{
			name:    "jellyfin missing url",
			service: "jellyfin",
			cfg:     Config{JellyfinAPIKey: "key123"},
			expected: false,
		},
		{
			name:    "sonarr configured",
			service: "sonarr",
			cfg:     Config{SonarrURL: "http://localhost:8989", SonarrAPIKey: "key"},
			expected: true,
		},
		{
			name:    "sonarr not configured",
			service: "sonarr",
			cfg:     Config{},
			expected: false,
		},
		{
			name:    "radarr configured",
			service: "radarr",
			cfg:     Config{RadarrURL: "http://localhost:7878", RadarrAPIKey: "key"},
			expected: true,
		},
		{
			name:    "qbittorrent configured",
			service: "qbittorrent",
			cfg:     Config{QBittorrentURL: "http://localhost:8080", QBittorrentPass: "pass"},
			expected: true,
		},
		{
			name:    "qbittorrent missing password",
			service: "qbittorrent",
			cfg:     Config{QBittorrentURL: "http://localhost:8080"},
			expected: false,
		},
		{
			name:    "prowlarr configured",
			service: "prowlarr",
			cfg:     Config{ProwlarrURL: "http://localhost:9696", ProwlarrAPIKey: "key"},
			expected: true,
		},
		{
			name:    "audiobookshelf configured",
			service: "audiobookshelf",
			cfg:     Config{AudiobookshelfURL: "http://localhost:13378", AudiobookshelfAPIKey: "key"},
			expected: true,
		},
		{
			name:    "kavita configured",
			service: "kavita",
			cfg:     Config{KavitaURL: "http://localhost:5005", KavitaPass: "pass"},
			expected: true,
		},
		{
			name:    "kavita missing password",
			service: "kavita",
			cfg:     Config{KavitaURL: "http://localhost:5005"},
			expected: false,
		},
		{
			name:    "calibre configured",
			service: "calibre",
			cfg:     Config{CalibreOPDSURL: "http://localhost:8083/opds"},
			expected: true,
		},
		{
			name:    "calibre not configured",
			service: "calibre",
			cfg:     Config{},
			expected: false,
		},
		{
			name:    "discord configured",
			service: "discord",
			cfg:     Config{DiscordWebhookURL: "https://discord.com/api/webhooks/123/abc"},
			expected: true,
		},
		{
			name:    "discord not configured",
			service: "discord",
			cfg:     Config{},
			expected: false,
		},
		{
			name:    "webhook configured",
			service: "webhook",
			cfg:     Config{NotificationWebhookURL: "http://localhost:9999/hook"},
			expected: true,
		},
		{
			name:    "jellyseerr configured",
			service: "jellyseerr",
			cfg:     Config{JellyseerrURL: "http://localhost:5055", JellyseerrAPIKey: "key"},
			expected: true,
		},
		{
			name:    "unknown service",
			service: "unknown_service",
			cfg:     Config{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.IsConfigured(tc.service)
			if got != tc.expected {
				t.Errorf("IsConfigured(%q) = %v, want %v", tc.service, got, tc.expected)
			}
		})
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Run("envStr returns env value", func(t *testing.T) {
		t.Setenv("TEST_STR", "hello")
		if v := envStr("TEST_STR", "default"); v != "hello" {
			t.Errorf("got %q", v)
		}
	})

	t.Run("envStr returns fallback", func(t *testing.T) {
		os.Unsetenv("TEST_STR_MISSING")
		if v := envStr("TEST_STR_MISSING", "fallback"); v != "fallback" {
			t.Errorf("got %q", v)
		}
	})

	t.Run("envInt returns env value", func(t *testing.T) {
		t.Setenv("TEST_INT", "42")
		if v := envInt("TEST_INT", 0); v != 42 {
			t.Errorf("got %d", v)
		}
	})

	t.Run("envInt returns fallback on invalid", func(t *testing.T) {
		t.Setenv("TEST_INT_BAD", "xyz")
		if v := envInt("TEST_INT_BAD", 99); v != 99 {
			t.Errorf("got %d", v)
		}
	})

	t.Run("envFloat returns env value", func(t *testing.T) {
		t.Setenv("TEST_FLOAT", "3.14")
		if v := envFloat("TEST_FLOAT", 0.0); v != 3.14 {
			t.Errorf("got %f", v)
		}
	})

	t.Run("envFloat returns fallback on invalid", func(t *testing.T) {
		t.Setenv("TEST_FLOAT_BAD", "xyz")
		if v := envFloat("TEST_FLOAT_BAD", 1.5); v != 1.5 {
			t.Errorf("got %f", v)
		}
	})
}
