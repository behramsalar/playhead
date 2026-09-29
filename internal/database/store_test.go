package database_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"playhead/internal/database"
	"playhead/internal/media"
)

func newTestStore(t *testing.T) *database.Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return database.NewStore(db)
}

func TestNeedsProbe(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	needs, err := store.NeedsProbe(ctx, "id-1", 100, modTime)
	if err != nil {
		t.Fatalf("NeedsProbe error: %v", err)
	}
	if !needs {
		t.Fatal("expected NeedsProbe true for an unindexed file")
	}

	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, media.ProbeResult{
		DurationSeconds: 12.5, Width: 1920, Height: 1080, Container: "mov,mp4", VideoCodec: "h264", AudioCodec: "aac",
	}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	needs, err = store.NeedsProbe(ctx, "id-1", 100, modTime)
	if err != nil {
		t.Fatalf("NeedsProbe error: %v", err)
	}
	if needs {
		t.Fatal("expected NeedsProbe false for an unchanged, already-indexed file")
	}

	needs, err = store.NeedsProbe(ctx, "id-1", 200, modTime)
	if err != nil {
		t.Fatalf("NeedsProbe error: %v", err)
	}
	if !needs {
		t.Fatal("expected NeedsProbe true after a size change")
	}
}

// TestNeedsProbeTrueForPendingRowEvenWithMatchingSizeMtime guards the fix
// for "new videos never get a duration": a video's first row can be
// created by SetThumbOK/SetSpriteOK racing ahead of the indexer, with
// size/mtime already matching the real file but status left at its
// default "pending" (no duration/codec data). NeedsProbe must still say
// true for that row — otherwise the indexer would treat matching
// size/mtime alone as "already indexed" and skip it on every future scan.
func TestNeedsProbeTrueForPendingRowEvenWithMatchingSizeMtime(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mp4", 100, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}

	needs, err := store.NeedsProbe(ctx, "id-1", 100, modTime)
	if err != nil {
		t.Fatalf("NeedsProbe error: %v", err)
	}
	if !needs {
		t.Fatal("expected NeedsProbe true for a pending row even though size/mtime already match")
	}
}

// TestSetThumbOKRecordsAndPreservesDuration checks the other half of the
// same fix: SetThumbOK/SetSpriteOK can now persist a probed duration
// directly (closing the gap before the indexer would otherwise reach the
// file), and a later call with an unknown duration (nil, e.g. probing
// failed) must not clobber an already-recorded value.
func TestSetThumbOKRecordsAndPreservesDuration(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	duration := 42.5
	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mp4", 100, modTime.Unix(), &duration); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}
	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok || meta.DurationSeconds == nil || *meta.DurationSeconds != duration {
		t.Fatalf("expected duration %v recorded, got meta=%+v ok=%v err=%v", duration, meta, ok, err)
	}

	// A later call (e.g. a retry after a thumbnail cache eviction) with no
	// duration available must not erase the one already on record.
	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mp4", 100, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK (nil duration): %v", err)
	}
	meta, ok, err = store.GetMetadata(ctx, "id-1")
	if err != nil || !ok || meta.DurationSeconds == nil || *meta.DurationSeconds != duration {
		t.Fatalf("expected duration %v preserved after a nil-duration update, got meta=%+v ok=%v err=%v", duration, meta, ok, err)
	}
}

func TestGetMetadataAbsentUntilIndexed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, ok, err := store.GetMetadata(ctx, "unknown-id")
	if err != nil {
		t.Fatalf("GetMetadata error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for an unindexed video")
	}

	modTime := time.Unix(1_700_000_000, 0)
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, media.ProbeResult{
		DurationSeconds: 12.5, Width: 1920, Height: 1080, Container: "mov,mp4", VideoCodec: "h264", AudioCodec: "aac",
	}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil {
		t.Fatalf("GetMetadata error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for an indexed video")
	}
	if meta.Status != "ok" || meta.DurationSeconds == nil || *meta.DurationSeconds != 12.5 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestUpsertErrorRecordsNoMetadata(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	if err := store.UpsertError(ctx, "id-1", "main", "broken.mkv", 10, modTime, "ffprobe failed: invalid data"); err != nil {
		t.Fatalf("UpsertError error: %v", err)
	}

	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil {
		t.Fatalf("GetMetadata error: %v", err)
	}
	if !ok {
		t.Fatal("expected a row to exist even though the probe failed")
	}
	if meta.Status != "error" {
		t.Fatalf("status = %q, want error", meta.Status)
	}
	if meta.DurationSeconds != nil {
		t.Fatal("expected no duration for a failed probe")
	}

	// A failed probe must not be retried every scan for an unchanged file.
	needs, err := store.NeedsProbe(ctx, "id-1", 10, modTime)
	if err != nil {
		t.Fatalf("NeedsProbe error: %v", err)
	}
	if needs {
		t.Fatal("expected NeedsProbe false for an unchanged, already-attempted file")
	}
}

func TestDeleteStale(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 1, Width: 1, Height: 1, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"keep-1", "keep-2", "remove-1"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mp4", 1, modTime, probe); err != nil {
			t.Fatalf("UpsertOK(%s) error: %v", id, err)
		}
	}

	removed, err := store.DeleteStale(ctx, "main", []string{"keep-1", "keep-2"})
	if err != nil {
		t.Fatalf("DeleteStale error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	if _, ok, _ := store.GetMetadata(ctx, "remove-1"); ok {
		t.Fatal("expected remove-1 to be deleted")
	}
	if _, ok, _ := store.GetMetadata(ctx, "keep-1"); !ok {
		t.Fatal("expected keep-1 to remain")
	}
}

func TestThumbStatusLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// No row at all: treated as pending, same as an explicit pending row.
	status, thumbErr, ok, err := store.GetThumbStatus(ctx, "unknown-id")
	if err != nil {
		t.Fatalf("GetThumbStatus error: %v", err)
	}
	if ok || status != "" || thumbErr != nil {
		t.Fatalf("expected ok=false for an unindexed video, got status=%q err=%v ok=%v", status, thumbErr, ok)
	}

	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, probe); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	status, _, ok, err = store.GetThumbStatus(ctx, "id-1")
	if err != nil || !ok || status != "pending" {
		t.Fatalf("expected fresh row to be thumb-pending, got status=%q ok=%v err=%v", status, ok, err)
	}

	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mp4", 100, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK error: %v", err)
	}
	status, _, _, _ = store.GetThumbStatus(ctx, "id-1")
	if status != "ok" {
		t.Fatalf("status = %q, want ok", status)
	}

	// Re-probing (file changed) must reset thumb status back to pending.
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 200, modTime, probe); err != nil {
		t.Fatalf("UpsertOK (update) error: %v", err)
	}
	status, _, _, _ = store.GetThumbStatus(ctx, "id-1")
	if status != "pending" {
		t.Fatalf("status after file change = %q, want pending", status)
	}

	if err := store.SetThumbError(ctx, "id-1", "main", "a.mp4", 200, modTime.Unix(), "ffmpeg failed"); err != nil {
		t.Fatalf("SetThumbError error: %v", err)
	}
	status, thumbErr, _, _ = store.GetThumbStatus(ctx, "id-1")
	if status != "error" || thumbErr == nil || *thumbErr != "ffmpeg failed" {
		t.Fatalf("unexpected error state: status=%q err=%v", status, thumbErr)
	}
}

// TestSetThumbStatusWithoutIndexRow covers the case that matters most for
// thumbnails: a file the indexer hasn't probed yet (no row exists at
// all). Generation must still be able to record its outcome, not silently
// no-op — otherwise a permanently-failing file would be retried on every
// single request forever.
func TestSetThumbStatusWithoutIndexRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mtime := time.Unix(1_700_000_000, 0).Unix()

	if err := store.SetThumbError(ctx, "never-indexed", "main", "clip.mp4", 42, mtime, "ffmpeg failed"); err != nil {
		t.Fatalf("SetThumbError on a row-less video: %v", err)
	}

	status, thumbErr, ok, err := store.GetThumbStatus(ctx, "never-indexed")
	if err != nil {
		t.Fatalf("GetThumbStatus error: %v", err)
	}
	if !ok || status != "error" || thumbErr == nil || *thumbErr != "ffmpeg failed" {
		t.Fatalf("expected a persisted error row, got status=%q ok=%v err=%v", status, ok, thumbErr)
	}
}

func TestFilterNeedingThumbnail(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"pending-1", "ok-1", "error-1"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mp4", 1, modTime, probe); err != nil {
			t.Fatalf("UpsertOK(%s): %v", id, err)
		}
	}
	if err := store.SetThumbOK(ctx, "ok-1", "main", "ok-1.mp4", 1, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}
	if err := store.SetThumbError(ctx, "error-1", "main", "error-1.mp4", 1, modTime.Unix(), "boom"); err != nil {
		t.Fatalf("SetThumbError: %v", err)
	}

	need, err := store.FilterNeedingThumbnail(ctx, []string{"pending-1", "ok-1", "error-1", "never-indexed"})
	if err != nil {
		t.Fatalf("FilterNeedingThumbnail error: %v", err)
	}

	got := map[string]bool{}
	for _, id := range need {
		got[id] = true
	}
	if !got["pending-1"] || !got["never-indexed"] {
		t.Fatalf("expected pending-1 and never-indexed to need thumbnails, got %v", need)
	}
	if got["ok-1"] || got["error-1"] {
		t.Fatalf("expected ok-1 and error-1 to be excluded, got %v", need)
	}
}

func TestListPendingThumbnails(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

	if err := store.UpsertOK(ctx, "pending-1", "main", "a.mp4", 1, modTime, probe); err != nil {
		t.Fatalf("UpsertOK: %v", err)
	}
	if err := store.UpsertOK(ctx, "done-1", "main", "b.mp4", 1, modTime, probe); err != nil {
		t.Fatalf("UpsertOK: %v", err)
	}
	if err := store.SetThumbOK(ctx, "done-1", "main", "b.mp4", 1, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}

	targets, err := store.ListPendingThumbnails(ctx)
	if err != nil {
		t.Fatalf("ListPendingThumbnails error: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "pending-1" {
		t.Fatalf("targets = %+v, want just pending-1", targets)
	}
	if targets[0].RootID != "main" || targets[0].RelPath != "a.mp4" {
		t.Fatalf("unexpected target: %+v", targets[0])
	}
}

func TestQueryVideosUnderPath(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac"}

	paths := []string{
		"Shows/Season 1/ep1.mp4",
		"Shows/Season 1/ep2.mp4",
		"Shows/Season_10/ep1.mp4", // underscore must not act as a LIKE wildcard
		"Shows/Season 2/ep1.mp4",
		"top-level.mp4",
	}
	for i, p := range paths {
		id := "id-" + p
		if err := store.UpsertOK(ctx, id, "main", p, int64(i), modTime, probe); err != nil {
			t.Fatalf("UpsertOK(%s): %v", p, err)
		}
	}

	t.Run("root prefix returns everything", func(t *testing.T) {
		rows, err := store.QueryVideosUnderPath(ctx, "main", "")
		if err != nil {
			t.Fatalf("QueryVideosUnderPath error: %v", err)
		}
		if len(rows) != len(paths) {
			t.Fatalf("got %d rows, want %d", len(rows), len(paths))
		}
	})

	t.Run("nested prefix", func(t *testing.T) {
		rows, err := store.QueryVideosUnderPath(ctx, "main", "Shows/Season 1")
		if err != nil {
			t.Fatalf("QueryVideosUnderPath error: %v", err)
		}
		if len(rows) != 2 {
			t.Fatalf("got %d rows, want 2 (underscore variant must not match): %+v", len(rows), rows)
		}
	})

	t.Run("prefix does not falsely match a sibling with shared prefix", func(t *testing.T) {
		rows, err := store.QueryVideosUnderPath(ctx, "main", "Shows/Season 1")
		if err != nil {
			t.Fatalf("QueryVideosUnderPath error: %v", err)
		}
		for _, r := range rows {
			if strings.Contains(r.RelPath, "Season_10") {
				t.Fatalf("Season_10 incorrectly matched prefix Shows/Season 1: %+v", rows)
			}
		}
	})
}
