package indexer_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/indexer"
	"playhead/internal/media"
)

// requireFFmpeg skips the test if ffmpeg/ffprobe aren't on PATH — these
// tests exercise real probing, not just the database layer.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if !media.FFprobeAvailable() {
		t.Skip("ffprobe not available")
	}
}

// generateClip writes a tiny real H.264/AAC mp4 to path using ffmpeg, so
// ffprobe has something genuine to parse.
func generateClip(t *testing.T, path string, seconds int) {
	t.Helper()
	dur := strconv.Itoa(seconds)
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration="+dur,
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+dur,
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p",
		"-y", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg failed: %v\n%s", err, out)
	}
}

func newTestStore(t *testing.T) *database.Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return database.NewStore(db)
}

func TestScanIndexesAndSkipsUnchanged(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	generateClip(t, filepath.Join(root, "a.mp4"), 1)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	idx := indexer.New(store, roots)

	idx.Scan(context.Background())
	status := idx.Status()
	if status.FilesFound != 1 || status.FilesProbed != 1 {
		t.Fatalf("first scan status = %+v, want 1 found/probed", status)
	}

	meta, ok, err := store.GetMetadata(context.Background(), idFor(t, "main", "a.mp4"))
	if err != nil || !ok {
		t.Fatalf("GetMetadata error=%v ok=%v", err, ok)
	}
	if meta.Status != "ok" || meta.DurationSeconds == nil || meta.VideoCodec == nil || *meta.VideoCodec != "h264" {
		t.Fatalf("unexpected metadata after indexing: %+v", meta)
	}

	// Second scan: nothing changed on disk, so it should be skipped, not
	// reprobed.
	idx.Scan(context.Background())
	status = idx.Status()
	if status.FilesFound != 1 || status.FilesSkipped != 1 || status.FilesProbed != 0 {
		t.Fatalf("second scan status = %+v, want 1 found/skipped, 0 probed", status)
	}
}

// TestScanProbesRowCreatedByThumbnailFirst reproduces the "new videos
// never get a duration" bug: preview generation can create a video's
// first database row (via Store.SetThumbOK/SetSpriteOK) before the
// indexer ever reaches that file, with the row's size/mtime already
// matching the real file but no duration/codec metadata. Without
// NeedsProbe also checking status == "pending", the indexer would treat
// that row as already-indexed (size/mtime match) and skip it forever,
// leaving the duration badge permanently missing for that file.
func TestScanProbesRowCreatedByThumbnailFirst(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	path := filepath.Join(root, "a.mp4")
	generateClip(t, path, 1)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	idx := indexer.New(store, roots)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	id := idFor(t, "main", "a.mp4")
	// Simulate a thumbnail job winning the race against the indexer: it
	// creates the row first, with the file's real size/mtime but no probe
	// result yet.
	if err := store.SetThumbOK(context.Background(), id, "main", "a.mp4", info.Size(), info.ModTime().Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}
	meta, ok, err := store.GetMetadata(context.Background(), id)
	if err != nil || !ok || meta.DurationSeconds != nil {
		t.Fatalf("expected a duration-less row to exist before scanning, got meta=%+v ok=%v err=%v", meta, ok, err)
	}

	idx.Scan(context.Background())
	status := idx.Status()
	if status.FilesProbed != 1 || status.FilesSkipped != 0 {
		t.Fatalf("scan status = %+v, want the pre-existing row still probed, not skipped", status)
	}

	meta, ok, err = store.GetMetadata(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetMetadata after scan: error=%v ok=%v", err, ok)
	}
	if meta.Status != "ok" || meta.DurationSeconds == nil {
		t.Fatalf("expected duration to be backfilled after scan, got meta=%+v", meta)
	}
}

func TestScanReprobesChangedFile(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	path := filepath.Join(root, "a.mp4")
	generateClip(t, path, 1)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	idx := indexer.New(store, roots)
	idx.Scan(context.Background())

	// Replace with a longer clip and bump mtime so size/mtime differ.
	generateClip(t, path, 2)
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	idx.Scan(context.Background())
	status := idx.Status()
	if status.FilesProbed != 1 {
		t.Fatalf("expected the changed file to be reprobed, status = %+v", status)
	}
}

func TestScanRemovesDeletedFiles(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	path := filepath.Join(root, "a.mp4")
	generateClip(t, path, 1)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	idx := indexer.New(store, roots)

	id := idFor(t, "main", "a.mp4")
	idx.Scan(context.Background())
	if _, ok, _ := store.GetMetadata(context.Background(), id); !ok {
		t.Fatal("expected file to be indexed before deletion")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("removing file: %v", err)
	}

	idx.Scan(context.Background())
	if _, ok, _ := store.GetMetadata(context.Background(), id); ok {
		t.Fatal("expected deleted file's row to be removed")
	}
	if idx.Status().FilesRemoved != 1 {
		t.Fatalf("FilesRemoved = %d, want 1", idx.Status().FilesRemoved)
	}
}

func TestScanHandlesUnprobeableFileWithoutCrashing(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.mkv"), []byte("not a real video"), 0o644); err != nil {
		t.Fatalf("writing broken file: %v", err)
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	idx := indexer.New(store, roots)

	idx.Scan(context.Background())
	status := idx.Status()
	if status.FilesErrored != 1 {
		t.Fatalf("status = %+v, want 1 errored", status)
	}

	id := idFor(t, "main", "broken.mkv")
	meta, ok, err := store.GetMetadata(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("GetMetadata error=%v ok=%v", err, ok)
	}
	if meta.Status != "error" {
		t.Fatalf("status = %q, want error", meta.Status)
	}
}

func TestIndexIsDisposable(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	generateClip(t, filepath.Join(root, "a.mp4"), 1)
	id := idFor(t, "main", "a.mp4")

	dbPath := filepath.Join(t.TempDir(), "index.db")
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}

	db1, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	store1 := database.NewStore(db1)
	indexer.New(store1, roots).Scan(context.Background())
	db1.Close()

	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("removing database: %v", err)
	}

	db2, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("reopening database: %v", err)
	}
	defer db2.Close()
	store2 := database.NewStore(db2)
	indexer.New(store2, roots).Scan(context.Background())

	meta, ok, err := store2.GetMetadata(context.Background(), id)
	if err != nil || !ok || meta.Status != "ok" {
		t.Fatalf("rebuilt index did not reproduce id %q: meta=%+v ok=%v err=%v", id, meta, ok, err)
	}
}

func idFor(t *testing.T, rootID, relPath string) string {
	t.Helper()
	return filesystem.EncodeVideoID(rootID, relPath)
}
