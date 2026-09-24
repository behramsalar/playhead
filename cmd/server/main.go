// Command server runs the video browser's HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"playhead/internal/api"
	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/indexer"
	"playhead/internal/preview"
	"playhead/internal/settings"
	"playhead/web"
)

// previewWorkers bounds concurrent FFmpeg processes across thumbnail and
// sprite generation combined ("1-2 bounded FFmpeg workers" was the Phase 3
// spec — Phase 4 shares the same pool rather than adding a second one;
// see ARCHITECTURE.md).
const previewWorkers = 2

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	filesystem.Configure(cfg.ExtraHiddenNames, cfg.ExcludedPaths, cfg.ExcludedExtensions)

	// settingsStore holds onboarding/auth/per-root display state
	// (Phase 7), deliberately separate from index.db (see invariant 5 in
	// ARCHITECTURE.md) — see internal/settings. Load is a no-op, not an
	// error, when config.json doesn't exist yet: "not configured" is a
	// real, expected first-run state.
	settingsStore := settings.NewStore(cfg.DataDir)
	if err := settingsStore.Load(); err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	discovered, err := config.DiscoverRoots(cfg.MediaRoot)
	if err != nil {
		return fmt.Errorf("discovering media roots: %w", err)
	}
	var settingsCfg settings.Config
	if loaded, ok := settingsStore.Get(); ok {
		settingsCfg = loaded
	}
	cfg.Roots = settings.MergeRoots(discovered, settingsCfg)

	dist, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return fmt.Errorf("preparing embedded frontend: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "index.db")
	db, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening index database: %w", err)
	}
	defer db.Close()
	store := database.NewStore(db)

	idx := indexer.New(store, cfg.Roots)
	previews := preview.New(ctx, store, cfg.Roots, cfg.DataDir, previewWorkers, cfg.CacheMaxBytes)

	// A rescan's removed/changed files leave their old cache assets
	// orphaned; clean those up right after, not just via the manual
	// endpoint (Phase 5: "triggered how — on scan completion? a separate
	// endpoint? both?").
	idx.AfterScan = func(scanCtx context.Context) {
		removed, err := previews.CleanOrphans(scanCtx)
		if err != nil {
			slog.Error("post-scan orphan cleanup failed", "error", err)
			return
		}
		if removed > 0 {
			slog.Info("post-scan orphan cleanup", "filesRemoved", removed)
		}
		// Requeue bounded-retry-eligible failures before pregenerating, so
		// a batch that failed for an environmental reason (e.g. a
		// permissions mismatch during deployment) gets swept up in the
		// same pregenerate pass below rather than waiting for a second
		// scan — see RequeueFailedForRetry.
		if requeued, err := previews.RequeueFailedForRetry(scanCtx); err != nil {
			slog.Error("post-scan retry requeue failed", "error", err)
		} else if requeued > 0 {
			slog.Info("post-scan retry requeue", "count", requeued)
		}
		if started, err := previews.Pregenerate(scanCtx); err != nil {
			slog.Error("post-scan thumbnail pregeneration failed", "error", err)
		} else if started > 0 {
			slog.Info("post-scan thumbnail pregeneration started", "count", started)
		}
		if started, err := previews.PregenerateSprites(scanCtx); err != nil {
			slog.Error("post-scan sprite pregeneration failed", "error", err)
		} else if started > 0 {
			slog.Info("post-scan sprite pregeneration started", "count", started)
		}
	}
	idx.TryStartScan(ctx)

	if removedFiles, removedBytes, err := previews.EnforceCacheLimit(ctx); err != nil {
		slog.Error("startup cache limit enforcement failed", "error", err)
	} else if removedFiles > 0 {
		slog.Info("startup cache limit enforcement", "filesRemoved", removedFiles, "bytesRemoved", removedBytes)
	}

	handler := api.New(cfg, dist, store, idx, previews, settingsStore, ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: long video streams must not be cut off.
	}

	for _, root := range cfg.Roots {
		slog.Info("configured media root", "id", root.ID, "name", root.Name)
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("starting server", "addr", cfg.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
