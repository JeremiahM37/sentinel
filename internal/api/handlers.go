package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
	"github.com/JeremiahM37/sentinel/internal/models"
)

// Version is set at build time.
var Version = "0.1.0"

// Handlers holds the dependencies for all HTTP handlers.
type Handlers struct {
	DB       *db.JobDB
	Guardian *guardian.Guardian
}

// Health returns the health status of the service.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	available := h.Guardian.Verifier().GetAvailableCheckers(ctx)

	writeJSON(w, http.StatusOK, models.HealthResponse{
		Status:             "ok",
		Version:            Version,
		ConfiguredServices: available,
		GuardianRunning:    h.Guardian.IsRunning(),
	})
}

// CreateJob creates a new guardian job.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	var req models.JobCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	if msg := req.Validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	now := time.Now().UTC()
	job := &models.Job{
		ID:                 newUUID(),
		Title:              req.Title,
		MediaType:          req.MediaType,
		Status:             models.JobStatusPending,
		Year:               req.Year,
		Author:             req.Author,
		ImdbID:             req.ImdbID,
		TvdbID:             req.TvdbID,
		SourceAttempts:     []models.SourceAttempt{},
		VerificationChecks: []models.VerificationProof{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	ctx := r.Context()
	if err := h.DB.CreateJob(ctx, job); err != nil {
		slog.Error("Failed to create job", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	slog.Info("Created job",
		"job_id", shortID(job.ID), "title", job.Title, "media_type", job.MediaType)

	writeJSON(w, http.StatusCreated, models.JobToResponse(job))
}

// ListJobs lists guardian jobs with optional filters.
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	mediaType := r.URL.Query().Get("media_type")
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	ctx := r.Context()
	jobs, total, err := h.DB.ListJobs(ctx, status, mediaType, limit, offset)
	if err != nil {
		slog.Error("Failed to list jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	responses := make([]models.JobResponse, len(jobs))
	for i := range jobs {
		responses[i] = models.JobToResponse(&jobs[i])
	}

	writeJSON(w, http.StatusOK, models.JobListResponse{
		Jobs:  responses,
		Total: total,
	})
}

// GetJob returns details for a specific job.
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	ctx := r.Context()
	job, err := h.DB.GetJob(ctx, jobID)
	if err != nil {
		slog.Error("Failed to get job", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	writeJSON(w, http.StatusOK, models.JobToResponse(job))
}

// CancelJob cancels an active job.
func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	ctx := r.Context()
	job, err := h.DB.GetJob(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status.IsTerminal() {
		writeError(w, http.StatusBadRequest,
			"Job is already in terminal state: "+string(job.Status))
		return
	}

	now := time.Now().UTC()
	job.Status = models.JobStatusCancelled
	job.CompletedAt = &now
	if err := h.DB.UpdateJob(ctx, job); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	slog.Info("Cancelled job", "job_id", shortID(job.ID), "title", job.Title)
	writeJSON(w, http.StatusOK, models.JobToResponse(job))
}

// RetryJob retries a failed or cancelled job from scratch.
func (h *Handlers) RetryJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	ctx := r.Context()
	job, err := h.DB.GetJob(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	if job.Status != models.JobStatusFailed && job.Status != models.JobStatusCancelled {
		writeError(w, http.StatusBadRequest,
			"Can only retry failed or cancelled jobs (current: "+string(job.Status)+")")
		return
	}

	job.Status = models.JobStatusPending
	job.SourceAttempts = []models.SourceAttempt{}
	job.VerificationChecks = []models.VerificationProof{}
	job.VerifyCount = 0
	job.CurrentDownloadID = ""
	job.CompletedAt = nil
	if err := h.DB.UpdateJob(ctx, job); err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	slog.Info("Retrying job", "job_id", shortID(job.ID), "title", job.Title)
	writeJSON(w, http.StatusOK, models.JobToResponse(job))
}

// DeleteJob permanently deletes a job.
func (h *Handlers) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")

	ctx := r.Context()
	deleted, err := h.DB.DeleteJob(ctx, jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "Job not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetStats returns job statistics by status.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.DB.GetStats(ctx)
	if err != nil {
		slog.Error("Failed to get stats", "error", err)
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// VerifyTitle manually verifies whether a title exists in libraries.
func (h *Handlers) VerifyTitle(w http.ResponseWriter, r *http.Request) {
	var req models.VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	job := &models.Job{
		ID:        "manual",
		Title:     req.Title,
		MediaType: req.MediaType,
		Year:      req.Year,
		Author:    req.Author,
	}

	ctx := r.Context()
	results := h.Guardian.Verifier().Verify(ctx, job)
	found := guardian.HasProof(results)

	writeJSON(w, http.StatusOK, models.VerifyResponse{
		Title:     req.Title,
		MediaType: req.MediaType,
		Found:     found,
		Proofs:    results,
	})
}

// helpers

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func queryInt(r *http.Request, key string, fallback int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
