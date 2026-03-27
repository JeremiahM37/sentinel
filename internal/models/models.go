// Package models defines the core data types for Sentinel jobs, source attempts,
// verification proofs, and API request/response schemas.
package models

import (
	"encoding/json"
	"time"
)

// MediaType represents the type of media being tracked.
type MediaType string

const (
	MediaTypeMovie     MediaType = "movie"
	MediaTypeTV        MediaType = "tv"
	MediaTypeAudiobook MediaType = "audiobook"
	MediaTypeEbook     MediaType = "ebook"
	MediaTypeComic     MediaType = "comic"
	MediaTypeBook      MediaType = "book"  // alias for ebook
	MediaTypeManga     MediaType = "manga" // alias for comic
	MediaTypeGame      MediaType = "game"
	MediaTypeROM       MediaType = "rom"
)

// ValidMediaTypes is the set of all recognised media types.
var ValidMediaTypes = map[MediaType]bool{
	MediaTypeMovie:     true,
	MediaTypeTV:        true,
	MediaTypeAudiobook: true,
	MediaTypeEbook:     true,
	MediaTypeComic:     true,
	MediaTypeBook:      true,
	MediaTypeManga:     true,
	MediaTypeGame:      true,
	MediaTypeROM:       true,
}

// NormalizeMediaType maps aliases to canonical types.
func NormalizeMediaType(mt MediaType) MediaType {
	switch mt {
	case "book":
		return MediaTypeEbook
	case "manga":
		return MediaTypeComic
	default:
		return mt
	}
}

// JobStatus represents the lifecycle status of a guardian job.
type JobStatus string

const (
	JobStatusPending     JobStatus = "pending"
	JobStatusSearching   JobStatus = "searching"
	JobStatusDownloading JobStatus = "downloading"
	JobStatusVerifying   JobStatus = "verifying"
	JobStatusCompleted   JobStatus = "completed"
	JobStatusFailed      JobStatus = "failed"
	JobStatusCancelled   JobStatus = "cancelled"
)

// IsTerminal returns true if the status is a terminal state.
func (s JobStatus) IsTerminal() bool {
	return s == JobStatusCompleted || s == JobStatusFailed || s == JobStatusCancelled
}

// IsActive returns true if the job needs guardian attention.
func (s JobStatus) IsActive() bool {
	return s == JobStatusPending || s == JobStatusSearching ||
		s == JobStatusDownloading || s == JobStatusVerifying
}

// VerificationStatus is the result of a single verification check.
type VerificationStatus string

const (
	VerificationFound    VerificationStatus = "found"
	VerificationNotFound VerificationStatus = "not_found"
	VerificationError    VerificationStatus = "error"
)

// VerificationProof contains concrete evidence that content exists in a library.
type VerificationProof struct {
	Library        string             `json:"library"`
	Status         VerificationStatus `json:"status"`
	TitleMatched   string             `json:"title_matched,omitempty"`
	FilePath       string             `json:"file_path,omitempty"`
	RuntimeSeconds *float64           `json:"runtime_seconds,omitempty"`
	PageCount      *int               `json:"page_count,omitempty"`
	AudioFileCount *int               `json:"audio_file_count,omitempty"`
	Extra          map[string]any     `json:"extra,omitempty"`
	CheckedAt      time.Time          `json:"checked_at"`
	ErrorMessage   string             `json:"error_message,omitempty"`
}

// SourceAttempt records one source being tried for a job.
type SourceAttempt struct {
	SourceName   string     `json:"source_name"`
	Query        string     `json:"query"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Success      bool       `json:"success"`
	DownloadID   string     `json:"download_id,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// Job tracks content from request to library verification.
type Job struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	MediaType          MediaType           `json:"media_type"`
	Status             JobStatus           `json:"status"`
	Year               *int                `json:"year,omitempty"`
	Author             string              `json:"author,omitempty"`
	ImdbID             string              `json:"imdb_id,omitempty"`
	TvdbID             *int                `json:"tvdb_id,omitempty"`
	SourceAttempts     []SourceAttempt     `json:"source_attempts"`
	VerificationChecks []VerificationProof `json:"verification_checks"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	CompletedAt        *time.Time          `json:"completed_at,omitempty"`
	CurrentDownloadID  string              `json:"current_download_id,omitempty"`
	VerifyCount        int                 `json:"verify_count"`
}

// JobCreateRequest is the request body for creating a new guardian job.
type JobCreateRequest struct {
	Title     string    `json:"title"`
	MediaType MediaType `json:"media_type"`
	Year      *int      `json:"year,omitempty"`
	Author    string    `json:"author,omitempty"`
	ImdbID    string    `json:"imdb_id,omitempty"`
	TvdbID    *int      `json:"tvdb_id,omitempty"`
}

// Validate checks that the request is well-formed.
func (r *JobCreateRequest) Validate() string {
	if r.Title == "" {
		return "title is required"
	}
	if !ValidMediaTypes[r.MediaType] {
		return "invalid media_type"
	}
	return ""
}

// JobResponse is the API response for a single job.
type JobResponse struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title"`
	MediaType          MediaType           `json:"media_type"`
	Status             JobStatus           `json:"status"`
	Year               *int                `json:"year,omitempty"`
	Author             string              `json:"author,omitempty"`
	SourceAttempts     []SourceAttempt     `json:"source_attempts"`
	VerificationChecks []VerificationProof `json:"verification_checks"`
	VerifyCount        int                 `json:"verify_count"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	CompletedAt        *time.Time          `json:"completed_at,omitempty"`
}

// JobToResponse converts a Job to a JobResponse.
func JobToResponse(j *Job) JobResponse {
	sa := j.SourceAttempts
	if sa == nil {
		sa = []SourceAttempt{}
	}
	vc := j.VerificationChecks
	if vc == nil {
		vc = []VerificationProof{}
	}
	return JobResponse{
		ID:                 j.ID,
		Title:              j.Title,
		MediaType:          j.MediaType,
		Status:             j.Status,
		Year:               j.Year,
		Author:             j.Author,
		SourceAttempts:     sa,
		VerificationChecks: vc,
		VerifyCount:        j.VerifyCount,
		CreatedAt:          j.CreatedAt,
		UpdatedAt:          j.UpdatedAt,
		CompletedAt:        j.CompletedAt,
	}
}

// JobListResponse is the API response for listing jobs.
type JobListResponse struct {
	Jobs  []JobResponse `json:"jobs"`
	Total int           `json:"total"`
}

// StatsResponse is the API response for guardian statistics.
type StatsResponse struct {
	TotalJobs   int `json:"total_jobs"`
	Pending     int `json:"pending"`
	Searching   int `json:"searching"`
	Downloading int `json:"downloading"`
	Verifying   int `json:"verifying"`
	Completed   int `json:"completed"`
	Failed      int `json:"failed"`
	Cancelled   int `json:"cancelled"`
}

// HealthResponse is the API response for the health check.
type HealthResponse struct {
	Status             string   `json:"status"`
	Version            string   `json:"version"`
	ConfiguredServices []string `json:"configured_services"`
	GuardianRunning    bool     `json:"guardian_running"`
}

// VerifyRequest is the request body for manual verification.
type VerifyRequest struct {
	Title     string    `json:"title"`
	MediaType MediaType `json:"media_type"`
	Year      *int      `json:"year,omitempty"`
	Author    string    `json:"author,omitempty"`
}

// VerifyResponse is the API response for manual verification.
type VerifyResponse struct {
	Title     string              `json:"title"`
	MediaType MediaType           `json:"media_type"`
	Found     bool                `json:"found"`
	Proofs    []VerificationProof `json:"proofs"`
}

// MustMarshalJSON marshals v to JSON, returning "null" on error.
func MustMarshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
