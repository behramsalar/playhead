package preview_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/preview"
)

// requireWebP skips the test unless ffmpeg is present AND its libwebp
// encoder is actually usable — a plain "ffmpeg exists" check isn't enough
// since some builds (e.g. Homebrew's default formula) omit libwebp.
func requireWebP(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	out, err := exec.Command("ffmpeg", "-hide_banner", "-h", "encoder=libwebp").CombinedOutput()
	if err != nil || bytes.Contains(out, []byte("is not recognized")) || bytes.Contains(out, []byte("Unknown encoder")) {
		t.Skip("ffmpeg build has no libwebp encoder")
	}
}

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
		t.Fatalf("ffmpeg (source clip) failed: %v\n%s", err, out)
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

func TestSeekTime(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	cases := []struct {
		name     string
		duration *float64
		want     float64
	}{
		{"unknown duration", nil, 1},
		{"zero duration", f(0), 1},
		{"very short clip", f(2), 1},
		{"normal clip", f(100), 25},
		{"near-boundary clip stays in range", f(1), 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := preview.SeekTime(tc.duration)
			if got != tc.want {
				t.Fatalf("SeekTime(%v) = %v, want %v", tc.duration, got, tc.want)
			}
			// A zero/negative duration is degenerate (no valid in-range
			// seek point exists); the bounds check only makes sense for
			// an actual positive duration.
			if tc.duration != nil && *tc.duration > 0 && (got < 0 || got >= *tc.duration) {
				t.Fatalf("SeekTime(%v) = %v is out of [0, duration)", *tc.duration, got)
			}
		})
	}
}

func TestFileNameChangesWithContent(t *testing.T) {
	a := preview.FileName("id-1", 100, 1000)
	b := preview.FileName("id-1", 200, 1000) // size changed
	c := preview.FileName("id-1", 100, 2000) // mtime changed
	if a == b || a == c || b == c {
		t.Fatalf("expected distinct filenames for distinct content, got a=%s b=%s c=%s", a, b, c)
	}
	if !strings.HasSuffix(a, ".webp") {
		t.Fatalf("expected .webp suffix, got %s", a)
	}
}

func TestManagerGeneratesAndCaches(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mp4")
	generateClip(t, clip, 2)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	dataDir := t.TempDir()
	mgr := preview.New(context.Background(), store, roots, dataDir, 2, 0)

	id := filesystem.EncodeVideoID("main", "a.mp4")
	result, err := mgr.Request(context.Background(), id, "main", "a.mp4", clip, true)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if !result.Ready || result.Path == "" {
		t.Fatalf("expected a ready thumbnail, got %+v", result)
	}
	info, err := os.Stat(result.Path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty thumbnail file at %s: %v", result.Path, err)
	}
	if !strings.HasPrefix(result.Path, filepath.Join(dataDir, "cache", "thumbs")) {
		t.Fatalf("thumbnail written outside DATA_DIR/cache/thumbs: %s", result.Path)
	}

	// Second request should be an instant cache hit (no regeneration).
	result2, err := mgr.Request(context.Background(), id, "main", "a.mp4", clip, false)
	if err != nil {
		t.Fatalf("second Request error: %v", err)
	}
	if !result2.Ready || result2.Path != result.Path {
		t.Fatalf("expected cache hit reusing %s, got %+v", result.Path, result2)
	}
}

func TestManagerRegeneratesOnFileChange(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mp4")
	generateClip(t, clip, 2)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 2, 0)
	id := filesystem.EncodeVideoID("main", "a.mp4")

	first, err := mgr.Request(context.Background(), id, "main", "a.mp4", clip, true)
	if err != nil || !first.Ready {
		t.Fatalf("first Request: ready=%v err=%v", first.Ready, err)
	}

	generateClip(t, clip, 3) // different content -> different size/mtime
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(clip, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second, err := mgr.Request(context.Background(), id, "main", "a.mp4", clip, true)
	if err != nil || !second.Ready {
		t.Fatalf("second Request: ready=%v err=%v", second.Ready, err)
	}
	if second.Path == first.Path {
		t.Fatalf("expected a new thumbnail path after the file changed, got the same: %s", first.Path)
	}
}

func TestManagerDoesNotRetryPermanentFailure(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	broken := filepath.Join(root, "broken.mkv")
	if err := os.WriteFile(broken, []byte("not a real video"), 0o644); err != nil {
		t.Fatalf("writing broken file: %v", err)
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 1, 0)
	id := filesystem.EncodeVideoID("main", "broken.mkv")

	first, err := mgr.Request(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if !first.Failed {
		t.Fatalf("expected a broken file to fail generation, got %+v", first)
	}

	status, thumbErr, ok, err := store.GetThumbStatus(context.Background(), id)
	if err != nil || !ok || status != "error" || thumbErr == nil {
		t.Fatalf("expected persisted error status, got status=%q ok=%v err=%v (store err=%v)", status, ok, thumbErr, err)
	}

	// A second request must not block waiting on the semaphore/timeout —
	// it should short-circuit immediately via the persisted error status.
	start := time.Now()
	second, err := mgr.Request(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("second Request error: %v", err)
	}
	if !second.Failed {
		t.Fatalf("expected the second request to also report Failed, got %+v", second)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("second request took %v, expected an immediate short-circuit", elapsed)
	}
}

// TestManagerHandlesConcurrentRequests fires several concurrent requests
// at a pool bounded to fewer workers than jobs, and checks every one
// still completes successfully — i.e. the bound serializes work rather
// than dropping or corrupting it. It doesn't assert the bound is never
// exceeded (that needs a hook into job execution this package doesn't
// expose); the true concurrency-limiting behavior is spot-checked
// manually against a running server instead.
func TestManagerHandlesConcurrentRequests(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	const n = 4
	clips := make([]string, n)
	ids := make([]string, n)
	relPaths := make([]string, n)
	for i := range clips {
		relPaths[i] = strconv.Itoa(i) + ".mp4"
		clips[i] = filepath.Join(root, relPaths[i])
		generateClip(t, clips[i], 1)
		ids[i] = filesystem.EncodeVideoID("main", relPaths[i])
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	const workers = 2
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), workers, 0)

	var wg sync.WaitGroup
	for i := range clips {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := mgr.Request(context.Background(), ids[i], "main", relPaths[i], clips[i], true); err != nil {
				t.Errorf("Request(%d) error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	status, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if status.Ready != n {
		t.Fatalf("Status.Ready = %d, want %d", status.Ready, n)
	}
}

// TestJobTimeoutKillsHungGeneration verifies a job is bounded by its
// configured timeout rather than being able to hold a worker slot
// indefinitely — the fix for "generation became extremely slow with
// nothing else running", which turned out to be a single stalled FFmpeg
// invocation never getting killed, not a CPU-scheduling problem. Uses a
// perfectly valid clip (not a broken file — TestManagerDoesNotRetryPermanentFailure
// already covers that path) with the timeout overridden to a few
// milliseconds, short enough that FFmpeg's own process-spawn overhead
// alone guarantees the deadline fires before generation can complete —
// deterministic, not a race against real generation time.
func TestJobTimeoutKillsHungGeneration(t *testing.T) {
	requireWebP(t)

	restore := preview.SetThumbnailJobTimeoutForTesting(time.Millisecond)
	t.Cleanup(restore)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mp4")
	generateClip(t, clip, 2)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 1, 0)
	id := filesystem.EncodeVideoID("main", "a.mp4")

	start := time.Now()
	result, err := mgr.Request(context.Background(), id, "main", "a.mp4", clip, true)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}
	if !result.Failed {
		t.Fatalf("expected the 1ms-timeout job to fail, got %+v", result)
	}
	// requestWaitTimeout is several seconds; a job actually bounded by its
	// own 1ms timeout should report failure far sooner than that.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("job took %v to fail — timeout doesn't appear to be enforced", elapsed)
	}

	status, thumbErr, ok, err := store.GetThumbStatus(context.Background(), id)
	if err != nil || !ok || status != "error" || thumbErr == nil {
		t.Fatalf("expected a persisted error status after timeout, got status=%q ok=%v err=%v (store err=%v)", status, ok, thumbErr, err)
	}
}

// TestRequeueFailedForRetry covers the auto-retry sweep end to end
// through the Manager (not just the Store layer): a video that fails
// generation gets picked back up by RequeueFailedForRetry, is retried on
// the next Request, and — once it exhausts its retry budget against a
// file that will never succeed — stops being requeued at all, staying in
// its error state rather than being retried forever.
func TestRequeueFailedForRetry(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	broken := filepath.Join(root, "broken.mkv")
	if err := os.WriteFile(broken, []byte("not a real video"), 0o644); err != nil {
		t.Fatalf("writing broken file: %v", err)
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 1, 0)
	id := filesystem.EncodeVideoID("main", "broken.mkv")
	ctx := context.Background()

	first, err := mgr.Request(ctx, id, "main", "broken.mkv", broken, true)
	if err != nil || !first.Failed {
		t.Fatalf("expected the broken file to fail on the first attempt: failed=%v err=%v", first.Failed, err)
	}

	// maxAutoRetries is 3 (see manager.go): three requeue-and-fail-again
	// cycles should each successfully requeue exactly this one video.
	for i := 0; i < 3; i++ {
		requeued, err := mgr.RequeueFailedForRetry(ctx)
		if err != nil {
			t.Fatalf("RequeueFailedForRetry (cycle %d): %v", i, err)
		}
		if requeued != 1 {
			t.Fatalf("cycle %d: expected 1 requeued, got %d", i, requeued)
		}
		status, _, ok, err := store.GetThumbStatus(ctx, id)
		if err != nil || !ok || status != "pending" {
			t.Fatalf("cycle %d: expected pending after requeue, got status=%q ok=%v err=%v", i, status, ok, err)
		}

		retried, err := mgr.Request(ctx, id, "main", "broken.mkv", broken, true)
		if err != nil || !retried.Failed {
			t.Fatalf("cycle %d: expected the retried attempt against a still-broken file to fail: failed=%v err=%v", i, retried.Failed, err)
		}
	}

	// Retry budget exhausted: a further sweep must leave it alone.
	requeued, err := mgr.RequeueFailedForRetry(ctx)
	if err != nil {
		t.Fatalf("RequeueFailedForRetry after budget exhausted: %v", err)
	}
	if requeued != 0 {
		t.Fatalf("expected 0 requeued once the retry budget is exhausted, got %d", requeued)
	}
	status, _, ok, err := store.GetThumbStatus(ctx, id)
	if err != nil || !ok || status != "error" {
		t.Fatalf("expected the video to remain in error status once retries are exhausted, got status=%q ok=%v err=%v", status, ok, err)
	}
}
