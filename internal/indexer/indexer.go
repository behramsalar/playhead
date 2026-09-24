// Package indexer walks the configured media roots and populates the
// SQLite index with ffprobe-derived metadata, without ever blocking
// browsing: it reuses filesystem.Browse for traversal (the same hidden-
// entry filtering and symlink safety browsing uses) and writes one file
// at a time so rows appear incrementally as they're probed.
package indexer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/media"
)

// Status is a snapshot of the indexer's current or most recent scan.
type Status struct {
	State        string    `json:"state"` // "idle" or "scanning"
	StartedAt    time.Time `json:"startedAt,omitempty"`
	FinishedAt   time.Time `json:"finishedAt,omitempty"`
	FilesFound   int       `json:"filesFound"`
	FilesProbed  int       `json:"filesProbed"`
	FilesSkipped int       `json:"filesSkipped"`
	FilesErrored int       `json:"filesErrored"`
	FilesRemoved int       `json:"filesRemoved"`
	LastError    string    `json:"lastError,omitempty"`
}

// Indexer scans configured roots and keeps the database in sync with the
// filesystem.
type Indexer struct {
	store            *database.Store
	roots            []config.Root
	ffprobeAvailable bool

	// AfterScan, if set, runs once a scan finishes (success or not) —
	// wired to preview.Manager.CleanOrphans in main.go, so a rescan's
	// removed/changed files also get their now-orphaned cache assets
	// cleaned up automatically, not just via the manual endpoint.
	// Indexer deliberately doesn't import the preview package itself;
	// this keeps the two decoupled.
	AfterScan func(ctx context.Context)

	mu     sync.Mutex
	status Status
}

func New(store *database.Store, roots []config.Root) *Indexer {
	available := media.FFprobeAvailable()
	if !available {
		slog.Warn("ffprobe not found on PATH; files will be indexed without metadata")
	}
	return &Indexer{
		store:            store,
		roots:            roots,
		ffprobeAvailable: available,
		status:           Status{State: "idle"},
	}
}

// Status returns a snapshot safe to serialize for the scan-status endpoint.
func (idx *Indexer) Status() Status {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.status
}

// SetRoots replaces the set of roots future scans walk (Phase 7: roots
// can change at runtime via a settings-page rescan, not just at startup).
// It doesn't itself trigger a scan — callers that want newly-discovered
// content indexed immediately should follow up with TryStartScan.
func (idx *Indexer) SetRoots(roots []config.Root) {
	idx.mu.Lock()
	idx.roots = roots
	idx.mu.Unlock()
}

// TryStartScan starts a scan in the background unless one is already
// running. It returns false if a scan was already in progress.
func (idx *Indexer) TryStartScan(ctx context.Context) bool {
	idx.mu.Lock()
	if idx.status.State == "scanning" {
		idx.mu.Unlock()
		return false
	}
	idx.status = Status{State: "scanning", StartedAt: time.Now()}
	idx.mu.Unlock()

	go idx.Scan(ctx)
	return true
}

// Scan walks every configured root synchronously. Callers that want a
// background scan should use TryStartScan instead.
func (idx *Indexer) Scan(ctx context.Context) {
	idx.mu.Lock()
	if idx.status.State != "scanning" {
		idx.status = Status{State: "scanning", StartedAt: time.Now()}
	}
	idx.mu.Unlock()

	idx.mu.Lock()
	roots := idx.roots
	idx.mu.Unlock()

	var lastErr error
	for _, root := range roots {
		if err := idx.scanRoot(ctx, root); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				slog.Info("scan cancelled", "root", root.ID)
			} else {
				slog.Error("scanning root failed", "root", root.ID, "error", err)
			}
			lastErr = err
		}
	}

	idx.mu.Lock()
	idx.status.State = "idle"
	idx.status.FinishedAt = time.Now()
	if lastErr != nil {
		idx.status.LastError = lastErr.Error()
	}
	idx.mu.Unlock()

	if idx.AfterScan != nil {
		idx.AfterScan(ctx)
	}
}

func (idx *Indexer) scanRoot(ctx context.Context, root config.Root) error {
	var seen []string
	if err := idx.walkFolder(ctx, root, "", &seen); err != nil {
		return err
	}

	removed, err := idx.store.DeleteStale(ctx, root.ID, seen)
	if err != nil {
		return err
	}
	idx.mu.Lock()
	idx.status.FilesRemoved += int(removed)
	idx.mu.Unlock()
	return nil
}

func (idx *Indexer) walkFolder(ctx context.Context, root config.Root, relPath string, seen *[]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	result, err := filesystem.Browse(root.ID, root.Name, root.Path, relPath, root.HiddenChildren...)
	if err != nil {
		return err
	}

	for _, v := range result.Videos {
		if err := ctx.Err(); err != nil {
			return err
		}
		*seen = append(*seen, v.ID)
		idx.indexOne(ctx, root, v)
	}

	for _, f := range result.Folders {
		if err := idx.walkFolder(ctx, root, f.Path, seen); err != nil {
			return err
		}
	}
	return nil
}

func (idx *Indexer) indexOne(ctx context.Context, root config.Root, v filesystem.VideoEntry) {
	idx.mu.Lock()
	idx.status.FilesFound++
	idx.mu.Unlock()

	needsProbe, err := idx.store.NeedsProbe(ctx, v.ID, v.Size, v.Modified)
	if err != nil {
		slog.Error("checking index state failed", "id", v.ID, "error", err)
		return
	}
	if !needsProbe {
		idx.mu.Lock()
		idx.status.FilesSkipped++
		idx.mu.Unlock()
		return
	}

	if !idx.ffprobeAvailable {
		idx.recordError(ctx, root, v, "ffprobe not available")
		return
	}

	absPath, err := filesystem.Resolve(root.Path, v.Path)
	if err != nil {
		idx.recordError(ctx, root, v, err.Error())
		return
	}

	probe, err := media.Probe(ctx, absPath)
	if err != nil {
		idx.recordError(ctx, root, v, err.Error())
		return
	}

	if err := idx.store.UpsertOK(ctx, v.ID, root.ID, v.Path, v.Size, v.Modified, probe); err != nil {
		slog.Error("recording probe result failed", "id", v.ID, "error", err)
		return
	}
	idx.mu.Lock()
	idx.status.FilesProbed++
	idx.mu.Unlock()
}

func (idx *Indexer) recordError(ctx context.Context, root config.Root, v filesystem.VideoEntry, probeErr string) {
	if err := idx.store.UpsertError(ctx, v.ID, root.ID, v.Path, v.Size, v.Modified, probeErr); err != nil {
		slog.Error("recording probe error failed", "id", v.ID, "error", err)
		return
	}
	idx.mu.Lock()
	idx.status.FilesErrored++
	idx.mu.Unlock()
}
