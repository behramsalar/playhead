package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"playhead/internal/api"
	"playhead/internal/filesystem"
	"playhead/internal/indexer"
	"playhead/internal/preview"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
}

func writeClip(t *testing.T, path string, seconds int) {
	t.Helper()
	dur := fmt.Sprint(seconds)
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration="+dur,
		"-f", "lavfi", "-i", "sine=frequency=440:duration="+dur,
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p",
		"-y", path,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg (source clip) failed: %v\n%s", err, out)
	}
}

// newTestServerWithIndexedPreviews wires store+indexer+previews together
// and runs a scan, so GetMetadata has real codec info by the time
// handleStream needs to decide remux eligibility.
func newTestServerWithIndexedPreviews(t *testing.T, rootPath string) http.Handler {
	t.Helper()
	cfg, store, settingsStore := newTestConfigAndStore(t, rootPath)
	idx := indexer.New(store, cfg.Roots)
	idx.Scan(t.Context())
	previews := preview.New(t.Context(), store, cfg.Roots, cfg.DataDir, 2, 0)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	handler := api.New(cfg, dist, store, idx, previews, settingsStore, t.Context())
	return withAuthCookie(t, handler, settingsStore)
}

// TestHandleStreamRemuxesIncompatibleContainer confirms a browser-
// incompatible container (h264/aac in .mkv) is transparently served as a
// cached MP4 remux — different bytes, different (correct) Content-Type,
// but still a normal 200 GET through the same /stream endpoint.
func TestHandleStreamRemuxesIncompatibleContainer(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mkv")
	writeClip(t, clip, 2)

	handler := newTestServerWithIndexedPreviews(t, root)
	id := filesystem.EncodeVideoID("main", "movie.mkv")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q, want video/mp4 (the remuxed container, not the source .mkv)", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected a non-empty remuxed body")
	}
	original, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() == len(original) {
		t.Fatalf("expected the remuxed body to differ in size from the original .mkv bytes")
	}

	// Range requests must keep working against the remuxed file, exactly
	// like the existing non-remux stream path.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil)
	req.Header.Set("Range", "bytes=0-99")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range request status = %d, want 206", rec.Code)
	}
	if rec.Body.Len() != 100 {
		t.Fatalf("range body len = %d, want 100", rec.Body.Len())
	}
}

// TestHandleStreamServesNativeMP4Unchanged is a regression check: a video
// that's already browser-compatible (h264/aac in .mp4) must never enter
// the remux branch — same byte-for-byte behavior as before Phase 6.
func TestHandleStreamServesNativeMP4Unchanged(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mp4")
	writeClip(t, clip, 1)
	original, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}

	handler := newTestServerWithIndexedPreviews(t, root)
	id := filesystem.EncodeVideoID("main", "movie.mp4")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("Content-Type = %q, want video/mp4", got)
	}
	if rec.Body.Len() != len(original) {
		t.Fatalf("body len = %d, want %d (original bytes, untouched)", rec.Body.Len(), len(original))
	}
}

// TestHandleStreamRemuxFailure confirms a video that's remux-eligible by
// extension/codec metadata but whose bytes ffmpeg can't actually process
// reports a clear error instead of a silent empty/broken stream.
func TestHandleStreamRemuxFailure(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mkv")
	writeClip(t, clip, 1)

	handler := newTestServerWithIndexedPreviews(t, root)
	id := filesystem.EncodeVideoID("main", "movie.mkv")

	// Corrupt the file after indexing (codec metadata is already recorded)
	// so eligibility still says "yes, remux this" but the actual remux
	// attempt fails.
	if err := os.WriteFile(clip, []byte("not a real video anymore"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body.String())
	}
}
