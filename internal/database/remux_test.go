package database_test

import (
	"context"
	"testing"
	"time"

	"playhead/internal/media"
)

// TestRemuxStatusLifecycle mirrors TestThumbStatusLifecycle: a row-less
// video reports ok=false/pending, a freshly-indexed row starts pending,
// SetRemuxOK/SetRemuxError flip it, and a file change (re-probe) resets it
// back to pending so a stale remux isn't served after the source changes.
func TestRemuxStatusLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	status, remuxErr, ok, err := store.GetRemuxStatus(ctx, "unknown-id")
	if err != nil {
		t.Fatalf("GetRemuxStatus error: %v", err)
	}
	if ok || status != "" || remuxErr != nil {
		t.Fatalf("expected ok=false for an unindexed video, got status=%q err=%v ok=%v", status, remuxErr, ok)
	}

	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "matroska,webm", VideoCodec: "h264", AudioCodec: "aac"}
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mkv", 100, modTime, probe); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}

	status, _, ok, err = store.GetRemuxStatus(ctx, "id-1")
	if err != nil || !ok || status != "pending" {
		t.Fatalf("expected fresh row to be remux-pending, got status=%q ok=%v err=%v", status, ok, err)
	}

	if err := store.SetRemuxOK(ctx, "id-1", "main", "a.mkv", 100, modTime.Unix()); err != nil {
		t.Fatalf("SetRemuxOK error: %v", err)
	}
	status, _, _, _ = store.GetRemuxStatus(ctx, "id-1")
	if status != "ok" {
		t.Fatalf("status = %q, want ok", status)
	}

	// Re-probing (file changed) must reset remux status back to pending.
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mkv", 200, modTime, probe); err != nil {
		t.Fatalf("UpsertOK (update) error: %v", err)
	}
	status, _, _, _ = store.GetRemuxStatus(ctx, "id-1")
	if status != "pending" {
		t.Fatalf("status after file change = %q, want pending", status)
	}

	if err := store.SetRemuxError(ctx, "id-1", "main", "a.mkv", 200, modTime.Unix(), "ffmpeg failed"); err != nil {
		t.Fatalf("SetRemuxError error: %v", err)
	}
	status, remuxErr, _, _ = store.GetRemuxStatus(ctx, "id-1")
	if status != "error" || remuxErr == nil || *remuxErr != "ffmpeg failed" {
		t.Fatalf("unexpected error state: status=%q err=%v", status, remuxErr)
	}
}

// TestSetRemuxStatusWithoutIndexRow mirrors TestSetThumbStatusWithoutIndexRow:
// a permanent failure must be recordable (and later retrievable) even for a
// file the indexer hasn't probed yet, so it isn't retried on every request.
func TestSetRemuxStatusWithoutIndexRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mtime := time.Unix(1_700_000_000, 0).Unix()

	if err := store.SetRemuxError(ctx, "never-indexed", "main", "clip.mkv", 42, mtime, "ffmpeg failed"); err != nil {
		t.Fatalf("SetRemuxError on a row-less video: %v", err)
	}

	status, remuxErr, ok, err := store.GetRemuxStatus(ctx, "never-indexed")
	if err != nil {
		t.Fatalf("GetRemuxStatus error: %v", err)
	}
	if !ok || status != "error" || remuxErr == nil || *remuxErr != "ffmpeg failed" {
		t.Fatalf("expected a persisted error row, got status=%q ok=%v err=%v", status, ok, remuxErr)
	}
}

func TestFilterNeedingRemux(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "matroska,webm", VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"pending-1", "ok-1", "error-1"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mkv", 1, modTime, probe); err != nil {
			t.Fatalf("UpsertOK(%s): %v", id, err)
		}
	}
	if err := store.SetRemuxOK(ctx, "ok-1", "main", "ok-1.mkv", 1, modTime.Unix()); err != nil {
		t.Fatalf("SetRemuxOK: %v", err)
	}
	if err := store.SetRemuxError(ctx, "error-1", "main", "error-1.mkv", 1, modTime.Unix(), "boom"); err != nil {
		t.Fatalf("SetRemuxError: %v", err)
	}

	need, err := store.FilterNeedingRemux(ctx, []string{"pending-1", "ok-1", "error-1", "never-indexed"})
	if err != nil {
		t.Fatalf("FilterNeedingRemux error: %v", err)
	}

	got := map[string]bool{}
	for _, id := range need {
		got[id] = true
	}
	if !got["pending-1"] || !got["never-indexed"] {
		t.Fatalf("expected pending-1 and never-indexed to need remuxing, got %v", need)
	}
	if got["ok-1"] || got["error-1"] {
		t.Fatalf("expected ok-1 and error-1 to be excluded, got %v", need)
	}
}

func TestGetRemuxCounts(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "matroska,webm", VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"pending-1", "pending-2", "ok-1", "error-1"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mkv", 1, modTime, probe); err != nil {
			t.Fatalf("UpsertOK(%s): %v", id, err)
		}
	}
	if err := store.SetRemuxOK(ctx, "ok-1", "main", "ok-1.mkv", 1, modTime.Unix()); err != nil {
		t.Fatalf("SetRemuxOK: %v", err)
	}
	if err := store.SetRemuxError(ctx, "error-1", "main", "error-1.mkv", 1, modTime.Unix(), "boom"); err != nil {
		t.Fatalf("SetRemuxError: %v", err)
	}

	ok, errored, pending, err := store.GetRemuxCounts(ctx)
	if err != nil {
		t.Fatalf("GetRemuxCounts error: %v", err)
	}
	if ok != 1 || errored != 1 || pending != 2 {
		t.Fatalf("GetRemuxCounts = (ok=%d errored=%d pending=%d), want (1, 1, 2)", ok, errored, pending)
	}
}

func TestListPendingRemux(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "matroska,webm", VideoCodec: "h264", AudioCodec: "aac"}

	if err := store.UpsertOK(ctx, "pending-1", "main", "a.mkv", 1, modTime, probe); err != nil {
		t.Fatalf("UpsertOK: %v", err)
	}
	if err := store.UpsertOK(ctx, "done-1", "main", "b.mkv", 1, modTime, probe); err != nil {
		t.Fatalf("UpsertOK: %v", err)
	}
	if err := store.SetRemuxOK(ctx, "done-1", "main", "b.mkv", 1, modTime.Unix()); err != nil {
		t.Fatalf("SetRemuxOK: %v", err)
	}

	targets, err := store.ListPendingRemux(ctx)
	if err != nil {
		t.Fatalf("ListPendingRemux error: %v", err)
	}
	if len(targets) != 1 || targets[0].ID != "pending-1" {
		t.Fatalf("targets = %+v, want just pending-1", targets)
	}
}

// TestResetRemuxStatus confirms the targeted-retry reset (RetryRemux's
// store-level primitive) flips a permanently-failed video back to pending
// without touching its thumb/sprite status.
func TestResetRemuxStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, Width: 10, Height: 10, Container: "matroska,webm", VideoCodec: "h264", AudioCodec: "aac"}

	if err := store.UpsertOK(ctx, "id-1", "main", "a.mkv", 1, modTime, probe); err != nil {
		t.Fatalf("UpsertOK: %v", err)
	}
	if err := store.SetRemuxError(ctx, "id-1", "main", "a.mkv", 1, modTime.Unix(), "boom"); err != nil {
		t.Fatalf("SetRemuxError: %v", err)
	}
	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mkv", 1, modTime.Unix(), nil); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}

	if err := store.ResetRemuxStatus(ctx, "id-1"); err != nil {
		t.Fatalf("ResetRemuxStatus error: %v", err)
	}

	status, remuxErr, _, _ := store.GetRemuxStatus(ctx, "id-1")
	if status != "pending" || remuxErr != nil {
		t.Fatalf("expected remux status reset to pending, got status=%q err=%v", status, remuxErr)
	}
	thumbStatus, _, _, _ := store.GetThumbStatus(ctx, "id-1")
	if thumbStatus != "ok" {
		t.Fatalf("expected thumb_status untouched by ResetRemuxStatus, got %q", thumbStatus)
	}
}
