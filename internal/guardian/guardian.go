// Package guardian implements the background job runner that drives jobs through
// the request -> download -> verify pipeline. This is the heart of Sentinel.
package guardian

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/libraries"
	"github.com/JeremiahM37/sentinel/internal/models"
	"github.com/JeremiahM37/sentinel/internal/notify"
	"github.com/JeremiahM37/sentinel/internal/sources"
)

// Verifier orchestrates library verification across all configured checkers.
type Verifier struct {
	cfg      *config.Config
	checkers []libraries.LibraryChecker
}

// NewVerifier creates a verifier with all known library checkers.
func NewVerifier(cfg *config.Config) *Verifier {
	return &Verifier{
		cfg: cfg,
		checkers: []libraries.LibraryChecker{
			&libraries.JellyfinChecker{},
			&libraries.AudiobookshelfChecker{},
			&libraries.KavitaChecker{},
			&libraries.SonarrChecker{},
			&libraries.RadarrChecker{},
		},
	}
}

// GetAvailableCheckers returns names of checkers that are configured and reachable.
func (v *Verifier) GetAvailableCheckers(ctx context.Context) []string {
	var available []string
	for _, c := range v.checkers {
		if c.IsAvailable(ctx, v.cfg) {
			available = append(available, c.Name())
		}
	}
	if available == nil {
		available = []string{}
	}
	return available
}

// Verify runs all applicable checkers for the given job.
func (v *Verifier) Verify(ctx context.Context, job *models.Job) []models.VerificationProof {
	var results []models.VerificationProof

	for _, checker := range v.checkers {
		if !libraries.SupportsMediaType(checker, job.MediaType) {
			continue
		}

		if !checker.IsAvailable(ctx, v.cfg) {
			continue
		}

		proof := checker.Verify(ctx, job, v.cfg)
		results = append(results, proof)

		slog.Info("Checker result",
			"checker", checker.Name(),
			"status", proof.Status,
			"title", job.Title,
			"job_id", job.ID,
		)

		// Short-circuit on found
		if proof.Status == models.VerificationFound {
			break
		}
	}

	return results
}

// HasProof returns true if any result contains definitive proof of existence.
func HasProof(results []models.VerificationProof) bool {
	for _, r := range results {
		if r.Status == models.VerificationFound {
			return true
		}
	}
	return false
}

// Guardian is the background task manager that drives jobs to completion.
type Guardian struct {
	db       *db.JobDB
	cfg      *config.Config
	verifier *Verifier
	qbit     *sources.QBittorrentMonitor
	sources  []sources.Source
	discord  *notify.DiscordNotifier
	webhook  *notify.WebhookNotifier

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// New creates a new Guardian.
func New(database *db.JobDB, cfg *config.Config) *Guardian {
	return &Guardian{
		db:       database,
		cfg:      cfg,
		verifier: NewVerifier(cfg),
		qbit:     sources.NewQBittorrentMonitor(cfg),
		sources: []sources.Source{
			&sources.JellyseerrSource{},
			&sources.ProwlarrSource{},
		},
		discord: notify.NewDiscordNotifier(cfg),
		webhook: notify.NewWebhookNotifier(cfg),
	}
}

// IsRunning returns whether the guardian loop is active.
func (g *Guardian) IsRunning() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.running
}

// Verifier returns the guardian's verifier for use by API handlers.
func (g *Guardian) Verifier() *Verifier {
	return g.verifier
}

// Start begins the guardian background loop.
func (g *Guardian) Start() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.running {
		return
	}
	g.running = true
	g.done = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	g.cancel = cancel

	go g.loop(ctx)
	slog.Info("Guardian started", "interval_seconds", g.cfg.VerifyIntervalSeconds)
}

// Stop halts the guardian background loop and waits for it to finish.
func (g *Guardian) Stop() {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return
	}
	g.running = false
	g.cancel()
	done := g.done
	g.mu.Unlock()

	<-done
	slog.Info("Guardian stopped")
}

func (g *Guardian) loop(ctx context.Context) {
	defer close(g.done)

	ticker := time.NewTicker(time.Duration(g.cfg.VerifyIntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Run immediately on start
	g.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.tick(ctx)
		}
	}
}

func (g *Guardian) tick(ctx context.Context) {
	jobs, err := g.db.GetActiveJobs(ctx)
	if err != nil {
		slog.Error("Guardian: failed to get active jobs", "error", err)
		return
	}

	for i := range jobs {
		if ctx.Err() != nil {
			return
		}
		if err := g.processJob(ctx, &jobs[i]); err != nil {
			slog.Error("Guardian: error processing job", "job_id", jobs[i].ID, "error", err)
		}
	}
}

func (g *Guardian) processJob(ctx context.Context, job *models.Job) error {
	switch job.Status {
	case models.JobStatusPending:
		return g.handlePending(ctx, job)
	case models.JobStatusSearching:
		return g.handleSearching(ctx, job)
	case models.JobStatusDownloading:
		return g.handleDownloading(ctx, job)
	case models.JobStatusVerifying:
		return g.handleVerifying(ctx, job)
	}
	return nil
}

func (g *Guardian) handlePending(ctx context.Context, job *models.Job) error {
	job.Status = models.JobStatusSearching
	if err := g.db.UpdateJob(ctx, job); err != nil {
		return err
	}
	g.sendNotification(job, "searching", nil)

	return g.handleSearching(ctx, job)
}

func (g *Guardian) handleSearching(ctx context.Context, job *models.Job) error {
	sourcesForType := g.getSourcesForType(job.MediaType)
	attemptedSources := make(map[string]bool)
	for _, a := range job.SourceAttempts {
		attemptedSources[a.SourceName] = true
	}

	for _, src := range sourcesForType {
		if attemptedSources[src.Name()] {
			continue
		}
		if len(job.SourceAttempts) >= g.cfg.MaxSourcesPerType {
			break
		}
		if !src.IsAvailable(ctx, g.cfg) {
			continue
		}

		slog.Info("Trying source",
			"job_id", shortID(job.ID), "source", src.Name(), "title", job.Title)

		attempt := src.SearchAndDownload(ctx, job, g.cfg)
		job.SourceAttempts = append(job.SourceAttempts, attempt)

		if attempt.Success && attempt.DownloadID != "" {
			job.CurrentDownloadID = attempt.DownloadID
			job.Status = models.JobStatusDownloading
			if err := g.db.UpdateJob(ctx, job); err != nil {
				return err
			}
			g.sendNotification(job, "downloading", map[string]any{
				"source":      src.Name(),
				"download_id": attempt.DownloadID,
			})
			return nil
		}
	}

	// All sources exhausted
	hasSuccess := false
	for _, a := range job.SourceAttempts {
		if a.Success {
			hasSuccess = true
			break
		}
	}

	if !hasSuccess {
		now := time.Now().UTC()
		job.Status = models.JobStatusFailed
		job.CompletedAt = &now
		if err := g.db.UpdateJob(ctx, job); err != nil {
			return err
		}
		g.sendNotification(job, "failed", map[string]any{
			"reason":   "All sources exhausted",
			"attempts": len(job.SourceAttempts),
		})
	}

	return nil
}

func (g *Guardian) handleDownloading(ctx context.Context, job *models.Job) error {
	if job.CurrentDownloadID == "" {
		job.Status = models.JobStatusVerifying
		return g.db.UpdateJob(ctx, job)
	}

	if g.qbit.IsAvailable(ctx) {
		isComplete := g.qbit.IsDownloadComplete(ctx, job.CurrentDownloadID)

		if isComplete == nil {
			// Torrent not found -- might have been imported already
			slog.Warn("Torrent not found",
				"job_id", shortID(job.ID), "download_id", job.CurrentDownloadID)
			job.Status = models.JobStatusVerifying
			return g.db.UpdateJob(ctx, job)
		}

		if *isComplete {
			slog.Info("Download complete, moving to verification",
				"job_id", shortID(job.ID))
			job.Status = models.JobStatusVerifying
			if err := g.db.UpdateJob(ctx, job); err != nil {
				return err
			}
			g.sendNotification(job, "download_complete", nil)
			return nil
		}

		// Still downloading
		return nil
	}

	// No qBit configured -- skip to verification
	job.Status = models.JobStatusVerifying
	return g.db.UpdateJob(ctx, job)
}

func (g *Guardian) handleVerifying(ctx context.Context, job *models.Job) error {
	job.VerifyCount++

	results := g.verifier.Verify(ctx, job)
	job.VerificationChecks = append(job.VerificationChecks, results...)

	if HasProof(results) {
		now := time.Now().UTC()
		job.Status = models.JobStatusCompleted
		job.CompletedAt = &now
		if err := g.db.UpdateJob(ctx, job); err != nil {
			return err
		}

		// Find the proof for notification
		for _, r := range results {
			if r.Status == models.VerificationFound {
				g.sendNotification(job, "completed", map[string]any{
					"library":       r.Library,
					"file_path":     r.FilePath,
					"title_matched": r.TitleMatched,
				})
				slog.Info("VERIFIED in library",
					"job_id", shortID(job.ID),
					"library", r.Library,
					"title_matched", r.TitleMatched,
					"file_path", r.FilePath,
				)
				break
			}
		}
		return nil
	}

	// Not found yet
	if job.VerifyCount >= g.cfg.VerifyMaxChecks {
		slog.Info("Max verify checks reached, trying next source",
			"job_id", shortID(job.ID), "max_checks", g.cfg.VerifyMaxChecks)
		job.VerifyCount = 0
		job.CurrentDownloadID = ""
		job.Status = models.JobStatusSearching
		if err := g.db.UpdateJob(ctx, job); err != nil {
			return err
		}
		return g.handleSearching(ctx, job)
	}

	return g.db.UpdateJob(ctx, job)
}

func (g *Guardian) getSourcesForType(mt models.MediaType) []sources.Source {
	var result []sources.Source
	for _, s := range g.sources {
		if sources.SupportsMediaType(s, mt) {
			result = append(result, s)
		}
	}
	return result
}

func (g *Guardian) sendNotification(job *models.Job, event string, details map[string]any) {
	if g.discord.IsConfigured() {
		if err := g.discord.Notify(job, event, details); err != nil {
			slog.Warn("Discord notification failed", "job_id", job.ID)
		}
	}
	if g.webhook.IsConfigured() {
		if err := g.webhook.Notify(job, event, details); err != nil {
			slog.Warn("Webhook notification failed", "job_id", job.ID)
		}
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
