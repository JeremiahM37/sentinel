package api

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
)

// NewRouter creates the HTTP router with all routes and middleware.
func NewRouter(database *db.JobDB, g *guardian.Guardian) http.Handler {
	h := &Handlers{
		DB:       database,
		Guardian: g,
	}

	r := chi.NewRouter()
	r.Use(CORS)
	r.Use(Logger)

	// Health
	r.Get("/health", h.Health)

	// Jobs CRUD
	r.Post("/api/jobs", h.CreateJob)
	r.Get("/api/jobs", h.ListJobs)
	r.Get("/api/jobs/{jobID}", h.GetJob)
	r.Post("/api/jobs/{jobID}/cancel", h.CancelJob)
	r.Post("/api/jobs/{jobID}/retry", h.RetryJob)
	r.Delete("/api/jobs/{jobID}", h.DeleteJob)

	// Stats
	r.Get("/api/stats", h.GetStats)

	// Also respond to legacy /api/status path
	r.Get("/api/status", h.GetStats)

	// Manual verification
	r.Post("/api/verify", h.VerifyTitle)

	return r
}

// newUUID generates a random UUID v4.
func newUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
