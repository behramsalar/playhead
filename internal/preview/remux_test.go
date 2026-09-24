package preview_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playhead/internal/config"
	"playhead/internal/filesystem"
	"playhead/internal/media"
	"playhead/internal/preview"
)

// requireFFmpeg skips the test unless ffmpeg is on PATH. Unlike
// requireWebP (used by thumbnail/sprite tests), remuxing doesn't need the
// libwebp encoder — a plain h264/aac clip via generateClip is enough.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
}

func TestNeedsRemux(t *testing.T) {
	cases := []struct {
		name       string
		fileName   string
		videoCodec string
		audioCodec string
		want       bool
	}{
		{"h264/aac in mkv needs remux", "movie.mkv", "h264", "aac", true},
		{"hevc/mp3 in avi needs remux", "movie.AVI", "hevc", "mp3", true}, // extension case-insensitive
		{"h264/aac in ts needs remux", "clip.ts", "h264", "aac", true},
		{"h264/aac in mp4 does not need remux", "movie.mp4", "h264", "aac", false},
		{"h264/aac in webm does not need remux", "movie.webm", "h264", "aac", false},
		{"mpeg2video in mkv is a codec problem, not container", "movie.mkv", "mpeg2video", "aac", false},
		{"vc1 video in ts is a codec problem", "clip.ts", "vc1", "aac", false},
		{"flac audio in mkv is a codec problem", "movie.mkv", "h264", "flac", false},
		{"no extension", "movie", "h264", "aac", false},
		{"codec names are case-insensitive", "movie.mkv", "H264", "AAC", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := preview.NeedsRemux(tc.fileName, tc.videoCodec, tc.audioCodec); got != tc.want {
				t.Fatalf("NeedsRemux(%q, %q, %q) = %v, want %v", tc.fileName, tc.videoCodec, tc.audioCodec, got, tc.want)
			}
		})
	}
}

func TestManagerRemuxGeneratesPlayableMP4(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mkv")
	generateClip(t, clip, 2) // h264/aac in a container browsers won't direct-play

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	dataDir := t.TempDir()
	mgr := preview.New(context.Background(), store, roots, dataDir, 2, 0)

	id := filesystem.EncodeVideoID("main", "a.mkv")
	result, err := mgr.RequestRemux(context.Background(), id, "main", "a.mkv", clip, true)
	if err != nil {
		t.Fatalf("RequestRemux error: %v", err)
	}
	if !result.Ready || result.Path == "" {
		t.Fatalf("expected a ready remux, got %+v", result)
	}
	if !strings.HasPrefix(result.Path, filepath.Join(dataDir, "cache", "remux")) {
		t.Fatalf("remux written outside DATA_DIR/cache/remux: %s", result.Path)
	}

	// The output must actually be a browser-compatible MP4 carrying the
	// same codecs, not just a non-empty file — ffprobe it back.
	probe, err := media.Probe(context.Background(), result.Path)
	if err != nil {
		t.Fatalf("probing remuxed file: %v", err)
	}
	if probe.Container != "mov,mp4,m4a,3gp,3g2,mj2" {
		t.Fatalf("remuxed container = %q, want an mp4-family container", probe.Container)
	}
	if probe.VideoCodec != "h264" {
		t.Fatalf("remuxed video codec = %q, want h264", probe.VideoCodec)
	}
	if probe.AudioCodec != "aac" {
		t.Fatalf("remuxed audio codec = %q, want aac", probe.AudioCodec)
	}

	// Second request should be an instant cache hit (no regeneration).
	result2, err := mgr.RequestRemux(context.Background(), id, "main", "a.mkv", clip, false)
	if err != nil {
		t.Fatalf("second RequestRemux error: %v", err)
	}
	if !result2.Ready || result2.Path != result.Path {
		t.Fatalf("expected cache hit reusing %s, got %+v", result.Path, result2)
	}
}

func TestManagerRemuxRegeneratesOnFileChange(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mkv")
	generateClip(t, clip, 2)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 2, 0)
	id := filesystem.EncodeVideoID("main", "a.mkv")

	first, err := mgr.RequestRemux(context.Background(), id, "main", "a.mkv", clip, true)
	if err != nil || !first.Ready {
		t.Fatalf("first RequestRemux: ready=%v err=%v", first.Ready, err)
	}

	generateClip(t, clip, 3) // different content -> different size/mtime
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(clip, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second, err := mgr.RequestRemux(context.Background(), id, "main", "a.mkv", clip, true)
	if err != nil || !second.Ready {
		t.Fatalf("second RequestRemux: ready=%v err=%v", second.Ready, err)
	}
	if second.Path == first.Path {
		t.Fatalf("expected a new remux path after the file changed, got the same: %s", first.Path)
	}
}

func TestManagerRemuxDoesNotRetryPermanentFailure(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	broken := filepath.Join(root, "broken.mkv")
	if err := os.WriteFile(broken, []byte("not a real video"), 0o644); err != nil {
		t.Fatalf("writing broken file: %v", err)
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), 1, 0)
	id := filesystem.EncodeVideoID("main", "broken.mkv")

	first, err := mgr.RequestRemux(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("RequestRemux error: %v", err)
	}
	if !first.Failed {
		t.Fatalf("expected a broken file to fail remuxing, got %+v", first)
	}

	status, remuxErr, ok, err := store.GetRemuxStatus(context.Background(), id)
	if err != nil || !ok || status != "error" || remuxErr == nil {
		t.Fatalf("expected persisted error status, got status=%q ok=%v err=%v (store err=%v)", status, ok, remuxErr, err)
	}

	start := time.Now()
	second, err := mgr.RequestRemux(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("second RequestRemux error: %v", err)
	}
	if !second.Failed {
		t.Fatalf("expected the second request to also report Failed, got %+v", second)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("second request took %v, expected an immediate short-circuit", elapsed)
	}
}

// TestCleanOrphansIncludesRemux confirms the shared orphan-cleanup sweep
// (extended to cache/remux/ in this phase) removes a stale remuxed file
// the same way it already does for thumbnails/sprites, while leaving a
// remux file that still matches a current index row untouched.
func TestCleanOrphansIncludesRemux(t *testing.T) {
	dataDir := t.TempDir()
	remuxDir := filepath.Join(dataDir, "cache", "remux")
	if err := os.MkdirAll(remuxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	if err := store.UpsertOK(ctx, "video-1", "main", "a.mkv", 100, modTime, media.ProbeResult{DurationSeconds: 10}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	validHash := validHashFor("video-1", 100, modTime.Unix())
	validRemux := filepath.Join(remuxDir, validHash+".remux.mp4")
	orphanRemux := filepath.Join(remuxDir, strings.Repeat("f", 32)+".remux.mp4")
	for _, p := range []string{validRemux, orphanRemux} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mgr := preview.New(ctx, store, []config.Root{{ID: "main", Name: "Videos", Path: t.TempDir()}}, dataDir, 1, 0)

	removed, err := mgr.CleanOrphans(ctx)
	if err != nil {
		t.Fatalf("CleanOrphans error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 orphan removed, got %d", removed)
	}
	if _, err := os.Stat(validRemux); err != nil {
		t.Errorf("expected valid remux file to survive: %v", err)
	}
	if _, err := os.Stat(orphanRemux); !os.IsNotExist(err) {
		t.Error("expected orphan remux file to be removed")
	}
}

// TestClearAllResetsRemuxStatus confirms the "rebuild previews" action
// wipes cached remux files and resets remux_status back to pending, same
// as it already does for thumb_status/sprite_status.
func TestClearAllResetsRemuxStatus(t *testing.T) {
	dataDir := t.TempDir()
	remuxDir := filepath.Join(dataDir, "cache", "remux")
	if err := os.MkdirAll(remuxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	if err := store.SetRemuxOK(ctx, "video-1", "main", "a.mkv", 100, modTime.Unix()); err != nil {
		t.Fatalf("SetRemuxOK error: %v", err)
	}

	validHash := validHashFor("video-1", 100, modTime.Unix())
	if err := os.WriteFile(filepath.Join(remuxDir, validHash+".remux.mp4"), []byte("x"), 0o644); err != nil {
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

	status, _, ok, err := store.GetRemuxStatus(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetRemuxStatus error: %v", err)
	}
	if !ok || status != "pending" {
		t.Fatalf("expected remux_status reset to pending, got %q (ok=%v)", status, ok)
	}
}

// TestEnqueueFolderSkipsIneligibleAndUnindexed confirms EnqueueFolder only
// starts remux jobs for videos that are both indexed (codecs known) and
// eligible (NeedsRemux) — an unindexed file or a native-compatible
// container must not produce a queued remux job.
func TestEnqueueFolderSkipsIneligibleAndUnindexed(t *testing.T) {
	requireFFmpeg(t)

	root := t.TempDir()
	eligibleClip := filepath.Join(root, "eligible.mkv")
	generateClip(t, eligibleClip, 1)
	nativeClip := filepath.Join(root, "native.mp4")
	generateClip(t, nativeClip, 1)
	unindexedClip := filepath.Join(root, "unindexed.mkv")
	generateClip(t, unindexedClip, 1)

	store := newTestStore(t)
	ctx := context.Background()
	eligibleID := filesystem.EncodeVideoID("main", "eligible.mkv")
	nativeID := filesystem.EncodeVideoID("main", "native.mp4")
	unindexedID := filesystem.EncodeVideoID("main", "unindexed.mkv")

	modTime := time.Unix(1_700_000_000, 0)
	if err := store.UpsertOK(ctx, eligibleID, "main", "eligible.mkv", 100, modTime, media.ProbeResult{VideoCodec: "h264", AudioCodec: "aac"}); err != nil {
		t.Fatalf("UpsertOK eligible: %v", err)
	}
	if err := store.UpsertOK(ctx, nativeID, "main", "native.mp4", 100, modTime, media.ProbeResult{VideoCodec: "h264", AudioCodec: "aac"}); err != nil {
		t.Fatalf("UpsertOK native: %v", err)
	}
	// unindexedID deliberately has no index row at all.

	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	dataDir := t.TempDir()
	mgr := preview.New(ctx, store, roots, dataDir, 2, 0)

	videos := []filesystem.VideoEntry{
		{ID: eligibleID, Name: "eligible.mkv", Path: "eligible.mkv", Size: 100, Modified: modTime},
		{ID: nativeID, Name: "native.mp4", Path: "native.mp4", Size: 100, Modified: modTime},
		{ID: unindexedID, Name: "unindexed.mkv", Path: "unindexed.mkv", Size: 100, Modified: modTime},
	}
	mgr.EnqueueFolder(ctx, roots[0], videos)

	// Give the background job a moment to finish (bounded wait via polling
	// rather than a fixed sleep, since generation time isn't fixed).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, _, ok, _ := store.GetRemuxStatus(ctx, eligibleID)
		if ok && status == "ok" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	eligibleStatus, _, ok, err := store.GetRemuxStatus(ctx, eligibleID)
	if err != nil || !ok || eligibleStatus != "ok" {
		t.Fatalf("expected eligible.mkv to be remuxed, got status=%q ok=%v err=%v", eligibleStatus, ok, err)
	}

	// native.mp4 and unindexed.mkv may each still end up with a row (e.g.
	// EnqueueFolder's unrelated thumbnail job upserts a minimal row for
	// every video, regardless of remux eligibility — see SetThumbOK), but
	// remux_status specifically must never flip to "ok" for either, since
	// no remux job should ever have run for them.
	if status, _, ok, _ := store.GetRemuxStatus(ctx, nativeID); ok && status == "ok" {
		t.Fatalf("native.mp4 (already browser-compatible) should never be remuxed, got status=%q", status)
	}
	if status, _, ok, _ := store.GetRemuxStatus(ctx, unindexedID); ok && status == "ok" {
		t.Fatalf("unindexed.mkv (not yet probed) should never be remuxed, got status=%q", status)
	}

	// The remux cache directory itself must contain only the one file
	// generated for the eligible video.
	remuxDir := filepath.Join(dataDir, "cache", "remux")
	entries, err := os.ReadDir(remuxDir)
	if err != nil {
		t.Fatalf("reading remux cache dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 cached remux file, got %d: %v", len(entries), entries)
	}
}
