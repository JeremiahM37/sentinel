package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
)

// testRouterWithConfig builds a router around a temp database with the given config.
func testRouterWithConfig(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Connect(dbPath)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	g := guardian.New(database, cfg)
	return NewRouter(database, g, cfg)
}

func doRequest(t *testing.T, router http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestCORSDisabledByDefault(t *testing.T) {
	router := testRouterWithConfig(t, &config.Config{})

	rr := doRequest(t, router, http.MethodGet, "/api/stats", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want unset when CORS disabled", got)
	}
}

func TestCORSConfiguredOrigin(t *testing.T) {
	router := testRouterWithConfig(t, &config.Config{CORSOrigin: "https://dash.example.com"})

	rr := doRequest(t, router, http.MethodGet, "/api/stats", nil)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://dash.example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}
	if got := rr.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin for a specific configured origin", got)
	}

	// Preflight short-circuits with 204.
	rr = doRequest(t, router, http.MethodOptions, "/api/jobs", nil)
	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", rr.Code)
	}
}

func TestCORSWildcardOrigin(t *testing.T) {
	router := testRouterWithConfig(t, &config.Config{CORSOrigin: "*"})

	rr := doRequest(t, router, http.MethodGet, "/api/stats", nil)
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := rr.Header().Get("Vary"); got == "Origin" {
		t.Error("Vary: Origin should not be set for wildcard origin")
	}
}

func TestAPIKeyDisabledByDefault(t *testing.T) {
	router := testRouterWithConfig(t, &config.Config{})

	rr := doRequest(t, router, http.MethodGet, "/api/jobs", nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 when no API key configured", rr.Code)
	}
}

func TestAPIKeyRequired(t *testing.T) {
	router := testRouterWithConfig(t, &config.Config{APIKey: "sekrit"})

	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		want    int
	}{
		{"read without key", http.MethodGet, "/api/jobs", nil, http.StatusUnauthorized},
		{"mutate without key", http.MethodDelete, "/api/jobs/some-id", nil, http.StatusUnauthorized},
		{"wrong key", http.MethodGet, "/api/jobs", map[string]string{"X-Api-Key": "wrong"}, http.StatusUnauthorized},
		{"wrong bearer", http.MethodGet, "/api/jobs", map[string]string{"Authorization": "Bearer wrong"}, http.StatusUnauthorized},
		{"x-api-key header", http.MethodGet, "/api/jobs", map[string]string{"X-Api-Key": "sekrit"}, http.StatusOK},
		{"bearer token", http.MethodGet, "/api/jobs", map[string]string{"Authorization": "Bearer sekrit"}, http.StatusOK},
		{"health stays open", http.MethodGet, "/health", nil, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := doRequest(t, router, tc.method, tc.path, tc.headers)
			if rr.Code != tc.want {
				t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rr.Code, tc.want)
			}
		})
	}
}
