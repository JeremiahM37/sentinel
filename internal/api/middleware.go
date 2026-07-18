package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// CORS returns a middleware serving CORS headers for the configured origin.
// An empty origin disables CORS entirely (same-origin browsers only, which
// is the safe default); "*" allows any origin.
func CORS(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if origin == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key")
			if origin != "*" {
				w.Header().Add("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAPIKey returns a middleware that rejects requests lacking the
// configured API key, supplied either as an X-Api-Key header or as an
// Authorization: Bearer token. An empty key disables authentication.
func RequireAPIKey(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			supplied := r.Header.Get("X-Api-Key")
			if supplied == "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					supplied = strings.TrimPrefix(auth, "Bearer ")
				}
			}
			if subtle.ConstantTimeCompare([]byte(supplied), []byte(key)) != 1 {
				writeError(w, http.StatusUnauthorized, "Invalid or missing API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Logger logs every HTTP request with method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
