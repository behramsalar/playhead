package preview_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playhead/internal/config"
	"playhead/internal/media"
	"playhead/internal/preview"
)

// validHashFor derives the same content hash the manager would compute for
// a video with these size/mtime, via the exported FileName helper (which
// is hash+".webp") — avoiding a dependency on the package's unexported
// contentHash from this external test package.
func validHashFor(videoID string, size, mtimeUnix int64) string {
	return strings.TrimSuffix(preview.FileName(videoID, size, mtimeUnix), ".webp")
}

func TestCleanOrphans(t *testing.T) {
	dataDir := t.TempDir()
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")
	spritesDir := filepath.Join(dataDir, "cache", "sprites")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(spritesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	if err := store.UpsertOK(ctx, "video-1", "main", "a.mp4", 100, modTime, media.ProbeResult{DurationSeconds: 10}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	validHash := validHashFor("video-1", 100, modTime.Unix())
	validThumb := filepath.Join(thumbsDir, validHash+".webp")
	orphanThumb := filepath.Join(thumbsDir, strings.Repeat("f", 32)+".webp")
	validSpriteImg := filepath.Join(spritesDir, validHash+".sprite.webp")
	orphanSpriteImg := filepath.Join(spritesDir, strings.Repeat("f", 32)+".sprite.webp")

	for _, p := range []string{validThumb, orphanThumb, validSpriteImg, orphanSpriteImg} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mgr := preview.New(ctx, store, []config.Root{{ID: "main", Name: "Videos", Path: t.TempDir()}}, dataDir, 1, 0)

	removed, err := mgr.CleanOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanOrphans error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 orphans removed, got %d", removed)
	}
	for _, p := range []string{validThumb, validSpriteImg} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected valid file %s to survive, got: %v", p, err)
		}
	}
	for _, p := range []string{orphanThumb, orphanSpriteImg} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected orphan file %s to be removed", p)
		}
	}
}

func TestEnforceCacheLimit(t *testing.T) {
	dataDir := t.TempDir()
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Three 100-byte files with distinct mtimes, oldest first.
	names := []string{
		strings.Repeat("a", 32) + ".webp",
		strings.Repeat("b", 32) + ".webp",
		strings.Repeat("c", 32) + ".webp",
	}
	base := time.Now().Add(-1 * time.Hour)
	for i, name := range names {
		p := filepath.Join(thumbsDir, name)
		if err := os.WriteFile(p, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
		mtime := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	store := newTestStore(t)
	ctx := context.Background()
	// Cap at 150 bytes: 300 bytes total currently, so eviction must kick in
	// and should remove the oldest file(s) first.
	mgr := preview.New(ctx, store, []config.Root{{ID: "main", Name: "Videos", Path: t.TempDir()}}, dataDir, 1, 150)

	removedFiles, removedBytes, err := mgr.EnforceCacheLimit(ctx)
	if err != nil {
		t.Fatalf("EnforceCacheLimit error: %v", err)
	}
	if removedFiles == 0 || removedBytes == 0 {
		t.Fatalf("expected eviction to remove something, got removedFiles=%d removedBytes=%d", removedFiles, removedBytes)
	}

	// The oldest file must be gone; the newest must survive.
	if _, err := os.Stat(filepath.Join(thumbsDir, names[0])); !os.IsNotExist(err) {
		t.Error("expected the oldest cache file to be evicted first")
	}
	if _, err := os.Stat(filepath.Join(thumbsDir, names[2])); err != nil {
		t.Error("expected the newest cache file to survive")
	}

	status, err := mgr.CacheStatus()
	if err != nil {
		t.Fatalf("CacheStatus error: %v", err)
	}
	if status.TotalBytes > 150 {
		t.Errorf("expected usage back under the 150-byte limit, got %d", status.TotalBytes)
	}
}

func TestEnforceCacheLimitNoopWhenUnderLimit(t *testing.T) {
	dataDir := t.TempDir()
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thumbsDir, strings.Repeat("a", 32)+".webp"), make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	ctx := context.Background()
	mgr := preview.New(ctx, store, []config.Root{{ID: "main", Name: "Videos", Path: t.TempDir()}}, dataDir, 1, 1_000_000)

	removedFiles, removedBytes, err := mgr.EnforceCacheLimit(ctx)
	if err != nil {
		t.Fatalf("EnforceCacheLimit error: %v", err)
	}
	if removedFiles != 0 || removedBytes != 0 {
		t.Fatalf("expected no-op when under the limit, got removedFiles=%d removedBytes=%d", removedFiles, removedBytes)
	}
}

func TestClearAll(t *testing.T) {
	dataDir := t.TempDir()
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	if err := store.UpsertOK(ctx, "video-1", "main", "a.mp4", 100, modTime, media.ProbeResult{DurationSeconds: 10}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}
	if err := store.SetThumbOK(ctx, "video-1", "main", "a.mp4", 100, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK error: %v", err)
	}

	validHash := validHashFor("video-1", 100, modTime.Unix())
	if err := os.WriteFile(filepath.Join(thumbsDir, validHash+".webp"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := preview.New(ctx, store, []config.Root{{ID: "main", Name: "Videos", Path: t.TempDir()}}, dataDir, 1, 0)

	removed, err := mgr.ClearAll(ctx)
	if err != nil {
		t.Fatalf("ClearAll error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 file removed, got %d", removed)
	}

	entries, err := os.ReadDir(thumbsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected an empty thumbs dir, got %v", entries)
	}

	status, _, ok, err := store.GetThumbStatus(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetThumbStatus error: %v", err)
	}
	if !ok || status != "pending" {
		t.Fatalf("expected thumb_status reset to pending, got %q (ok=%v)", status, ok)
	}
}
