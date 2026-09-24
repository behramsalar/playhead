package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"playhead/internal/api"
	"playhead/internal/auth"
	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/indexer"
	"playhead/internal/preview"
	"playhead/internal/settings"
)

// testUsername/testPassword are the credentials newTestConfigAndStore
// provisions every test server with — Phase 7 requires a valid session on
// almost every route now, so the existing (pre-Phase-7) test suite needs
// one to keep exercising the behavior it was actually written to check,
// not auth itself (see auth_test.go/settings_test.go for that).
const (
	testUsername = "test-admin"
	testPassword = "test-password"
)

// newTestServer builds a server with no thumbnail manager (nil): most
// tests don't exercise previews, and handleBrowse's lazy background
// thumbnail generation would otherwise race the test's own DB-close/
// tempdir cleanup for no reason relevant to what's being tested.
func newTestServer(t *testing.T, rootPath string) http.Handler {
	t.Helper()
	handler, _ := newTestServerWithIndexer(t, rootPath)
	return handler
}

func newTestServerWithIndexer(t *testing.T, rootPath string) (http.Handler, *indexer.Indexer) {
	t.Helper()
	cfg, store, settingsStore := newTestConfigAndStore(t, rootPath)
	idx := indexer.New(store, cfg.Roots)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	handler := api.New(cfg, dist, store, idx, nil, settingsStore, t.Context())
	return withAuthCookie(t, handler, settingsStore), idx
}

// newTestServerWithPreviews wires a real preview.Manager for tests
// that exercise the thumbnail endpoints directly; those endpoints block
// on generation (wait=true) rather than firing background jobs that could
// outlive the test, so this doesn't have the race newTestServer avoids.
func newTestServerWithPreviews(t *testing.T, rootPath string) (http.Handler, *preview.Manager) {
	t.Helper()
	cfg, store, settingsStore := newTestConfigAndStore(t, rootPath)
	idx := indexer.New(store, cfg.Roots)
	previews := preview.New(t.Context(), store, cfg.Roots, cfg.DataDir, 2, 0)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	handler := api.New(cfg, dist, store, idx, previews, settingsStore, t.Context())
	return withAuthCookie(t, handler, settingsStore), previews
}

// newTestConfigAndStore also provisions a completed settings.Store (Phase
// 7: almost every route now requires a valid session) with known
// credentials, so withAuthCookie can sign a session against it.
func newTestConfigAndStore(t *testing.T, rootPath string) (*config.Config, *database.Store, *settings.Store) {
	t.Helper()
	cfg := &config.Config{
		Addr:    ":0",
		DataDir: t.TempDir(),
		Roots: []config.Root{
			{ID: "main", Name: "Videos", Path: rootPath},
		},
	}
	db, err := database.Open(filepath.Join(cfg.DataDir, "index.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	settingsStore := settings.NewStore(cfg.DataDir)
	passwordHash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("hashing test password: %v", err)
	}
	secret, err := auth.GenerateSecret()
	if err != nil {
		t.Fatalf("generating test session secret: %v", err)
	}
	if err := settingsStore.Save(settings.Config{
		ServerName:       "Test Server",
		AuthUsername:     testUsername,
		AuthPasswordHash: passwordHash,
		SessionSecret:    secret,
	}); err != nil {
		t.Fatalf("saving test settings: %v", err)
	}

	return cfg, database.NewStore(db), settingsStore
}

// withAuthCookie wraps handler so every request it receives already
// carries a valid session cookie for settingsStore's credentials — the
// rest of the (pre-Phase-7) test suite constructs requests with
// httptest.NewRequest directly and has no reason to know about auth at
// all; this keeps that true instead of threading a cookie through every
// call site.
func withAuthCookie(t *testing.T, handler http.Handler, settingsStore *settings.Store) http.Handler {
	t.Helper()
	cfg, ok := settingsStore.Get()
	if !ok {
		t.Fatal("withAuthCookie requires an already-configured settings.Store")
	}
	token, err := auth.SignSession(cfg.SessionSecret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("signing test session token: %v", err)
	}
	return &autoAuthHandler{inner: handler, cookie: &http.Cookie{Name: auth.CookieName, Value: token}}
}

type autoAuthHandler struct {
	inner  http.Handler
	cookie *http.Cookie
}

func (h *autoAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.AddCookie(h.cookie)
	h.inner.ServeHTTP(w, r)
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestHandleBrowse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)

	t.Run("valid listing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown root", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=bogus&path=", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=..%2F..%2Fetc", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
		}
		var body errorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decoding error body: %v", err)
		}
		if body.Error.Code == "" {
			t.Fatalf("expected error code in body, got %s", rec.Body.String())
		}
	})

	t.Run("missing folder", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=nope", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestHandleStream(t *testing.T) {
	root := t.TempDir()
	content := make([]byte, 1000)
	for i := range content {
		content[i] = byte(i % 256)
	}
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)

	videoID := filesystem.EncodeVideoID("main", "movie.mp4")
	streamURL := fmt.Sprintf("/api/videos/%s/stream", videoID)

	t.Run("full GET", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, streamURL, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec.Body.Len() != len(content) {
			t.Fatalf("body len = %d, want %d", rec.Body.Len(), len(content))
		}
		if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
			t.Fatalf("Accept-Ranges = %q", got)
		}
		if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
			t.Fatalf("Content-Type = %q", got)
		}
	})

	t.Run("HEAD", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, streamURL, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("HEAD body should be empty, got %d bytes", rec.Body.Len())
		}
		if got := rec.Header().Get("Content-Length"); got != fmt.Sprint(len(content)) {
			t.Fatalf("Content-Length = %q", got)
		}
	})

	t.Run("range bytes=0-99", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, streamURL, nil)
		req.Header.Set("Range", "bytes=0-99")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", rec.Code)
		}
		if rec.Body.Len() != 100 {
			t.Fatalf("body len = %d, want 100", rec.Body.Len())
		}
		if got, want := rec.Header().Get("Content-Range"), "bytes 0-99/1000"; got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
		if got := rec.Header().Get("Content-Length"); got != "100" {
			t.Fatalf("Content-Length = %q, want 100", got)
		}
		if !bytesEqual(rec.Body.Bytes(), content[0:100]) {
			t.Fatalf("unexpected body content")
		}
	})

	t.Run("range bytes=100-", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, streamURL, nil)
		req.Header.Set("Range", "bytes=100-")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", rec.Code)
		}
		wantLen := len(content) - 100
		if rec.Body.Len() != wantLen {
			t.Fatalf("body len = %d, want %d", rec.Body.Len(), wantLen)
		}
		if got, want := rec.Header().Get("Content-Range"), fmt.Sprintf("bytes 100-999/%d", len(content)); got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
	})

	t.Run("range bytes=-100 (suffix)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, streamURL, nil)
		req.Header.Set("Range", "bytes=-100")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("status = %d, want 206", rec.Code)
		}
		if rec.Body.Len() != 100 {
			t.Fatalf("body len = %d, want 100", rec.Body.Len())
		}
		if got, want := rec.Header().Get("Content-Range"), "bytes 900-999/1000"; got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
		if !bytesEqual(rec.Body.Bytes(), content[900:1000]) {
			t.Fatalf("unexpected body content")
		}
	})

	t.Run("unsatisfiable range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, streamURL, nil)
		req.Header.Set("Range", "bytes=5000-6000")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestedRangeNotSatisfiable {
			t.Fatalf("status = %d, want 416", rec.Code)
		}
		if got, want := rec.Header().Get("Content-Range"), "bytes */1000"; got != want {
			t.Fatalf("Content-Range = %q, want %q", got, want)
		}
	})

	t.Run("non-video extension returns 404", func(t *testing.T) {
		id := filesystem.EncodeVideoID("main", "notes.txt")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("missing file returns 404", func(t *testing.T) {
		id := filesystem.EncodeVideoID("main", "ghost.mp4")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/stream", id), nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestHandleHealth(t *testing.T) {
	root := t.TempDir()
	handler := newTestServer(t, root)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestBrowseMetadataAbsentUntilIndexed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "movie.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	videos := body["videos"].([]any)
	if len(videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(videos))
	}
	video := videos[0].(map[string]any)
	for _, field := range []string{"duration", "width", "height", "container", "videoCodec", "audioCodec"} {
		if _, present := video[field]; present {
			t.Fatalf("expected field %q to be absent for an unindexed video, got %v", field, video[field])
		}
	}
}

func TestBrowseIncludesMetadataAfterIndexing(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p",
		"-y", clip,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg failed: %v\n%s", err, out)
	}

	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	videos := body["videos"].([]any)
	video := videos[0].(map[string]any)
	if video["videoCodec"] != "h264" {
		t.Fatalf("videoCodec = %v, want h264", video["videoCodec"])
	}
	if _, ok := video["duration"]; !ok {
		t.Fatal("expected duration field to be present after indexing")
	}
}

func TestScanEndpoints(t *testing.T) {
	root := t.TempDir()
	handler, _ := newTestServerWithIndexer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/scan/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/scan/status status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/scan", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/scan status = %d, want 202", rec.Code)
	}
}

func TestBrowseFlatten(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "top.mp4"))
	mustMkdir(t, filepath.Join(root, "Shows", "Season 1"))
	mustWriteFile(t, filepath.Join(root, "Shows", "Season 1", "ep1.mp4"))
	mustMkdir(t, filepath.Join(root, "Shows", "Season 2"))
	mustWriteFile(t, filepath.Join(root, "Shows", "Season 2", "ep1.mp4"))

	handler, idx := newTestServerWithIndexer(t, root)
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=Shows&flatten=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Flatten bool  `json:"flatten"`
		Folders []any `json:"folders"`
		Videos  []struct {
			Name      string `json:"name"`
			Subfolder string `json:"subfolder"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if !body.Flatten {
		t.Fatal("expected flatten=true in response")
	}
	if len(body.Folders) != 0 {
		t.Fatalf("expected no folders in flattened view, got %v", body.Folders)
	}
	if len(body.Videos) != 2 {
		t.Fatalf("expected 2 nested videos, got %d: %+v", len(body.Videos), body.Videos)
	}
	subfolders := map[string]bool{}
	for _, v := range body.Videos {
		subfolders[v.Subfolder] = true
	}
	if !subfolders["Season 1"] || !subfolders["Season 2"] {
		t.Fatalf("expected subfolders Season 1 and Season 2, got %+v", body.Videos)
	}

	// A video that hasn't been indexed yet doesn't crash flatten mode —
	// it just doesn't appear until a scan reaches it, since this reads
	// from the index rather than walking the filesystem live.
	mustWriteFile(t, filepath.Join(root, "Shows", "Season 1", "ep2-not-indexed.mp4"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=Shows&flatten=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status after adding unindexed file = %d", rec.Code)
	}
}

func TestBrowseSort(t *testing.T) {
	root := t.TempDir()
	mustWriteFileSize(t, filepath.Join(root, "b.mp4"), 300)
	mustWriteFileSize(t, filepath.Join(root, "a.mp4"), 100)
	mustWriteFileSize(t, filepath.Join(root, "c.mp4"), 200)

	handler := newTestServer(t, root)

	t.Run("sort=size", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=&sort=size", nil))
		names := videoNames(t, rec.Body.Bytes())
		if want := []string{"a.mp4", "c.mp4", "b.mp4"}; !equalSlices(names, want) {
			t.Fatalf("sort=size order = %v, want %v", names, want)
		}
	})

	t.Run("sort=name (default)", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
		names := videoNames(t, rec.Body.Bytes())
		if want := []string{"a.mp4", "b.mp4", "c.mp4"}; !equalSlices(names, want) {
			t.Fatalf("default sort order = %v, want %v", names, want)
		}
	})
}

func TestThumbnailEndpoint(t *testing.T) {
	requireWebPEncoder(t)

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p",
		"-y", clip,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg failed: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.mkv"), []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler, _ := newTestServerWithPreviews(t, root)

	id := filesystem.EncodeVideoID("main", "movie.mp4")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/thumbnail", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("Content-Type = %q, want image/webp", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected a non-empty thumbnail body")
	}

	brokenID := filesystem.EncodeVideoID("main", "broken.mkv")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/thumbnail", brokenID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("broken file thumbnail status = %d, want 404", rec.Code)
	}

	missingID := filesystem.EncodeVideoID("main", "ghost.mp4")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/thumbnail", missingID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing file thumbnail status = %d, want 404", rec.Code)
	}
}

func TestThumbnailStatusEndpoints(t *testing.T) {
	root := t.TempDir()
	handler, _ := newTestServerWithPreviews(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/thumbnails/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/thumbnails/status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/thumbnails/pregenerate", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/thumbnails/pregenerate = %d, want 202", rec.Code)
	}
}

func TestThumbnailEndpointUnavailableWithoutManager(t *testing.T) {
	root := t.TempDir()
	handler := newTestServer(t, root) // nil preview manager

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/thumbnails/status", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func requireWebPEncoder(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	out, lookErr := exec.Command("ffmpeg", "-hide_banner", "-h", "encoder=libwebp").CombinedOutput()
	if lookErr != nil || !bytes.Contains(out, []byte("Encoder libwebp")) {
		t.Skip("ffmpeg build has no libwebp encoder")
	}
}

func TestSpriteEndpoints(t *testing.T) {
	requireWebPEncoder(t)

	root := t.TempDir()
	clip := filepath.Join(root, "movie.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=10:duration=5",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=5",
		"-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p",
		"-y", clip,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg failed: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.mkv"), []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler, _ := newTestServerWithPreviews(t, root)
	id := filesystem.EncodeVideoID("main", "movie.mp4")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/sprite", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sprite image status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/webp" {
		t.Fatalf("sprite image Content-Type = %q, want image/webp", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected a non-empty sprite image body")
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/sprite.json", id), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sprite meta status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("sprite meta Content-Type = %q, want application/json", got)
	}
	var meta struct {
		Frames []struct {
			Time float64 `json:"time"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatalf("decoding sprite meta: %v", err)
	}
	if len(meta.Frames) < 20 {
		t.Fatalf("expected at least 20 frames, got %d", len(meta.Frames))
	}

	brokenID := filesystem.EncodeVideoID("main", "broken.mkv")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/videos/%s/sprite", brokenID), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("broken file sprite status = %d, want 404", rec.Code)
	}
}

func TestSpriteStatusEndpoints(t *testing.T) {
	root := t.TempDir()
	handler, _ := newTestServerWithPreviews(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sprites/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sprites/status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sprites/pregenerate", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/sprites/pregenerate = %d, want 202", rec.Code)
	}
}

func TestSpriteEndpointUnavailableWithoutManager(t *testing.T) {
	root := t.TempDir()
	handler := newTestServer(t, root) // nil preview manager

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sprites/status", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	mustWriteFileSize(t, path, 4)
}

func mustWriteFileSize(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, size), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func videoNames(t *testing.T, body []byte) []string {
	t.Helper()
	var parsed struct {
		Videos []struct {
			Name string `json:"name"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	names := make([]string, len(parsed.Videos))
	for i, v := range parsed.Videos {
		names[i] = v.Name
	}
	return names
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
