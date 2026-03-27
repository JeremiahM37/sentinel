// Package db provides SQLite persistence for guardian jobs using pure-Go SQLite
// (modernc.org/sqlite). No CGO required.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/JeremiahM37/sentinel/internal/models"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    media_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    year INTEGER,
    author TEXT,
    imdb_id TEXT,
    tvdb_id INTEGER,
    source_attempts TEXT NOT NULL DEFAULT '[]',
    verification_checks TEXT NOT NULL DEFAULT '[]',
    current_download_id TEXT,
    verify_count INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_media_type ON jobs(media_type);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at);
`

// JobDB is the SQLite job store.
type JobDB struct {
	db *sql.DB
}

// Connect opens the database and ensures the schema exists.
func Connect(dbPath string) (*JobDB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	slog.Info("Database connected", "path", dbPath)
	return &JobDB{db: db}, nil
}

// Close closes the database connection.
func (d *JobDB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// CreateJob inserts a new job.
func (d *JobDB) CreateJob(ctx context.Context, job *models.Job) error {
	sa, _ := json.Marshal(job.SourceAttempts)
	vc, _ := json.Marshal(job.VerificationChecks)
	var completedAt *string
	if job.CompletedAt != nil {
		s := job.CompletedAt.Format(time.RFC3339Nano)
		completedAt = &s
	}

	_, err := d.db.ExecContext(ctx,
		`INSERT INTO jobs (id, title, media_type, status, year, author, imdb_id, tvdb_id,
			source_attempts, verification_checks, current_download_id, verify_count,
			created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Title, job.MediaType, job.Status,
		job.Year, nilIfEmpty(job.Author), nilIfEmpty(job.ImdbID), job.TvdbID,
		string(sa), string(vc), nilIfEmpty(job.CurrentDownloadID),
		job.VerifyCount,
		job.CreatedAt.Format(time.RFC3339Nano),
		job.UpdatedAt.Format(time.RFC3339Nano),
		completedAt,
	)
	return err
}

// GetJob fetches a single job by ID. Returns nil, nil if not found.
func (d *JobDB) GetJob(ctx context.Context, jobID string) (*models.Job, error) {
	row := d.db.QueryRowContext(ctx,
		"SELECT * FROM jobs WHERE id = ?", jobID)
	j, err := scanJob(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// UpdateJob updates an existing job, setting updated_at to now.
func (d *JobDB) UpdateJob(ctx context.Context, job *models.Job) error {
	job.UpdatedAt = time.Now().UTC()
	sa, _ := json.Marshal(job.SourceAttempts)
	vc, _ := json.Marshal(job.VerificationChecks)
	var completedAt *string
	if job.CompletedAt != nil {
		s := job.CompletedAt.Format(time.RFC3339Nano)
		completedAt = &s
	}

	_, err := d.db.ExecContext(ctx,
		`UPDATE jobs SET title=?, media_type=?, status=?, year=?, author=?, imdb_id=?,
			tvdb_id=?, source_attempts=?, verification_checks=?, current_download_id=?,
			verify_count=?, created_at=?, updated_at=?, completed_at=?
		WHERE id=?`,
		job.Title, job.MediaType, job.Status,
		job.Year, nilIfEmpty(job.Author), nilIfEmpty(job.ImdbID), job.TvdbID,
		string(sa), string(vc), nilIfEmpty(job.CurrentDownloadID),
		job.VerifyCount,
		job.CreatedAt.Format(time.RFC3339Nano),
		job.UpdatedAt.Format(time.RFC3339Nano),
		completedAt,
		job.ID,
	)
	return err
}

// ListJobs returns jobs with optional filters. Returns (jobs, total_count, error).
func (d *JobDB) ListJobs(ctx context.Context, status, mediaType string, limit, offset int) ([]models.Job, int, error) {
	where := ""
	var args []any

	if status != "" && mediaType != "" {
		where = "WHERE status = ? AND media_type = ?"
		args = append(args, status, mediaType)
	} else if status != "" {
		where = "WHERE status = ?"
		args = append(args, status)
	} else if mediaType != "" {
		where = "WHERE media_type = ?"
		args = append(args, mediaType)
	}

	// Count
	var total int
	countQuery := "SELECT COUNT(*) FROM jobs " + where
	if err := d.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch
	fetchQuery := fmt.Sprintf("SELECT * FROM jobs %s ORDER BY created_at DESC LIMIT ? OFFSET ?", where)
	fetchArgs := append(args, limit, offset)
	rows, err := d.db.QueryContext(ctx, fetchQuery, fetchArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		job, err := scanJobRows(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *job)
	}
	if jobs == nil {
		jobs = []models.Job{}
	}
	return jobs, total, rows.Err()
}

// GetActiveJobs returns all jobs that need guardian attention.
func (d *JobDB) GetActiveJobs(ctx context.Context) ([]models.Job, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT * FROM jobs WHERE status IN (?, ?, ?, ?)
		ORDER BY created_at ASC`,
		models.JobStatusPending, models.JobStatusSearching,
		models.JobStatusDownloading, models.JobStatusVerifying,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		job, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

// GetStats returns job counts by status.
func (d *JobDB) GetStats(ctx context.Context) (models.StatsResponse, error) {
	rows, err := d.db.QueryContext(ctx,
		"SELECT status, COUNT(*) as cnt FROM jobs GROUP BY status")
	if err != nil {
		return models.StatsResponse{}, err
	}
	defer rows.Close()

	stats := models.StatsResponse{}
	for rows.Next() {
		var status string
		var cnt int
		if err := rows.Scan(&status, &cnt); err != nil {
			return stats, err
		}
		stats.TotalJobs += cnt
		switch models.JobStatus(status) {
		case models.JobStatusPending:
			stats.Pending = cnt
		case models.JobStatusSearching:
			stats.Searching = cnt
		case models.JobStatusDownloading:
			stats.Downloading = cnt
		case models.JobStatusVerifying:
			stats.Verifying = cnt
		case models.JobStatusCompleted:
			stats.Completed = cnt
		case models.JobStatusFailed:
			stats.Failed = cnt
		case models.JobStatusCancelled:
			stats.Cancelled = cnt
		}
	}
	return stats, rows.Err()
}

// DeleteJob deletes a job. Returns true if a row was deleted.
func (d *JobDB) DeleteJob(ctx context.Context, jobID string) (bool, error) {
	res, err := d.db.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", jobID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// scanner is an interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanJobFromScanner(s scanner) (*models.Job, error) {
	var j models.Job
	var (
		year, tvdbID                                          sql.NullInt64
		author, imdbID, currentDLID, completedAt              sql.NullString
		sourceAttemptsJSON, verificationChecksJSON            string
		createdAtStr, updatedAtStr                            string
		mediaType, status                                     string
	)

	err := s.Scan(
		&j.ID, &j.Title, &mediaType, &status,
		&year, &author, &imdbID, &tvdbID,
		&sourceAttemptsJSON, &verificationChecksJSON,
		&currentDLID, &j.VerifyCount,
		&createdAtStr, &updatedAtStr, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	j.MediaType = models.MediaType(mediaType)
	j.Status = models.JobStatus(status)

	if year.Valid {
		y := int(year.Int64)
		j.Year = &y
	}
	if author.Valid {
		j.Author = author.String
	}
	if imdbID.Valid {
		j.ImdbID = imdbID.String
	}
	if tvdbID.Valid {
		t := int(tvdbID.Int64)
		j.TvdbID = &t
	}
	if currentDLID.Valid {
		j.CurrentDownloadID = currentDLID.String
	}

	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAtStr)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAtStr)
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
		j.CompletedAt = &t
	}

	// Unmarshal JSON arrays
	if sourceAttemptsJSON != "" {
		_ = json.Unmarshal([]byte(sourceAttemptsJSON), &j.SourceAttempts)
	}
	if j.SourceAttempts == nil {
		j.SourceAttempts = []models.SourceAttempt{}
	}

	if verificationChecksJSON != "" {
		_ = json.Unmarshal([]byte(verificationChecksJSON), &j.VerificationChecks)
	}
	if j.VerificationChecks == nil {
		j.VerificationChecks = []models.VerificationProof{}
	}

	return &j, nil
}

func scanJob(row *sql.Row) (*models.Job, error) {
	return scanJobFromScanner(row)
}

func scanJobRows(rows *sql.Rows) (*models.Job, error) {
	return scanJobFromScanner(rows)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
