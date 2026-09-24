package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"playhead/internal/api"
	"playhead/internal/indexer"
	"playhead/internal/preview"
)

// newCacheTestServer is newTestServerWithPreviews, but also returns the
// data dir so tests can inspect DATA_DIR/cache/thumbs directly, per the
// Phase 5 acceptance criterion that orphan cleanup be verified against
// the actual files on disk, not just API responses.
func newCacheTestServer(t *testing.T, rootPath string) (handler http.Handler, dataDir string) {
	t.Helper()
	cfg, store, settingsStore := newTestConfigAndStore(t, rootPath)
	idx := indexer.New(store, cfg.Roots)
	previews := preview.New(t.Context(), store, cfg.Roots, cfg.DataDir, 2, 0)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	h := api.New(cfg, dist, store, idx, previews, settingsStore, t.Context())
	return withAuthCookie(t, h, settingsStore), cfg.DataDir
}

func TestHandleCleanOrphans(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"))
	handler, dataDir := newCacheTestServer(t, root)
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			ID string `json:"id"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 1 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+browse.Videos[0].ID+"/thumbnail", nil))
	if rec.Code != http.StatusOK {
		t.Skipf("thumbnail generation unavailable in this environment (status %d): %s", rec.Code, rec.Body.String())
	}

	// Plant an orphan: a file that doesn't correspond to any current
	// video's content hash, e.g. left behind by a since-changed file.
	// hashLen is 32 (see internal/preview/cache.go); using a computed
	// string avoids a hand-counted literal being silently the wrong length.
	orphan := filepath.Join(thumbsDir, strings.Repeat("0", 32)+".webp")
	if err := os.WriteFile(orphan, []byte("stale"), 0o644); err != nil {
		t.Fatalf("planting orphan file: %v", err)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/cache/clean-orphans", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("clean-orphans status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		FilesRemoved int `json:"filesRemoved"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if result.FilesRemoved != 1 {
		t.Fatalf("expected exactly the orphan removed, got filesRemoved=%d", result.FilesRemoved)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("expected the orphan file to be deleted")
	}

	// The legitimately-referenced thumbnail must survive.
	entries, err := os.ReadDir(thumbsDir)
	if err != nil {
		t.Fatalf("reading thumbs dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly the real thumbnail to remain, got %v", entries)
	}
}

func TestHandlePreviewsClear(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"))
	handler, dataDir := newCacheTestServer(t, root)
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			ID string `json:"id"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 1 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/videos/"+browse.Videos[0].ID+"/thumbnail", nil))
	if rec.Code != http.StatusOK {
		t.Skipf("thumbnail generation unavailable in this environment: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/previews/clear", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("previews/clear status = %d, body = %s", rec.Code, rec.Body.String())
	}

	entries, err := os.ReadDir(thumbsDir)
	if err != nil {
		t.Fatalf("reading thumbs dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected an empty thumbs dir after clearing, got %v", entries)
	}

	// The video's status should have reset to pending, so it's not stuck
	// reporting an already-generated thumbnail that no longer exists.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/thumbnails/status", nil))
	var status struct {
		Ready   int `json:"ready"`
		Pending int `json:"pending"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decoding thumbnail status: %v", err)
	}
	if status.Ready != 0 || status.Pending != 1 {
		t.Fatalf("expected status reset to pending after clear, got ready=%d pending=%d", status.Ready, status.Pending)
	}
}

func TestHandleThumbnailRetry(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"))
	handler, _ := newCacheTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var browse struct {
		Videos []struct {
			ID string `json:"id"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &browse); err != nil || len(browse.Videos) != 1 {
		t.Fatalf("unexpected browse response: %s (err %v)", rec.Body.String(), err)
	}
	id := browse.Videos[0].ID

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/videos/"+id+"/thumbnail/retry", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("retry status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCacheStatusAndEnforceLimit(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"))
	handler, dataDir := newCacheTestServer(t, root)
	thumbsDir := filepath.Join(dataDir, "cache", "thumbs")
	if err := os.MkdirAll(thumbsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thumbsDir, strings.Repeat("a", 32)+".webp"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/cache/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cache/status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status struct {
		TotalBytes int64 `json:"totalBytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if status.TotalBytes != 1000 {
		t.Fatalf("expected totalBytes=1000, got %d", status.TotalBytes)
	}

	// No CACHE_MAX_BYTES configured in this test server, so enforcement is
	// a no-op — confirm the endpoint still responds cleanly rather than
	// erroring on an unset limit.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/cache/enforce-limit", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("enforce-limit status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(thumbsDir, strings.Repeat("a", 32)+".webp")); err != nil {
		t.Fatal("expected the file to survive with no configured limit")
	}
}
