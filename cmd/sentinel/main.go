// Sentinel — Media download guardian and library verification for the *arr ecosystem.
//
// Single binary, zero CGO, all config from environment variables.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JeremiahM37/sentinel/internal/api"
	"github.com/JeremiahM37/sentinel/internal/config"
	"github.com/JeremiahM37/sentinel/internal/db"
	"github.com/JeremiahM37/sentinel/internal/guardian"
)

// Version is set at build time via ldflags.
var Version = "0.1.0"

func main() {
	cfg := config.Load()

	// Configure structured logging
	level := slog.LevelInfo
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("Sentinel starting", "version", Version, "port", cfg.Port)

	// Connect database
	database, err := db.Connect(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to connect database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Start guardian
	g := guardian.New(database, cfg)
	g.Start()
	defer g.Stop()

	// Set API version
	api.Version = Version

	// Create HTTP server
	router := api.NewRouter(database, g)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("Received signal, shutting down", "signal", sig)
	case err := <-errCh:
		if err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	slog.Info("Sentinel stopped")
}
