package database_test

import (
	"context"
	"testing"
	"time"

	"playhead/internal/media"
)

func TestResumePositionSaveLoadClear(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, ok, err := store.GetResumePosition(ctx, "video-1"); err != nil {
		t.Fatalf("GetResumePosition error: %v", err)
	} else if ok {
		t.Fatal("expected no saved position for a video that was never saved")
	}

	if err := store.SetResumePosition(ctx, "video-1", 123.5); err != nil {
		t.Fatalf("SetResumePosition error: %v", err)
	}
	pos, ok, err := store.GetResumePosition(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetResumePosition error: %v", err)
	}
	if !ok || pos != 123.5 {
		t.Fatalf("got (%v, %v), want (123.5, true)", pos, ok)
	}

	// Saving again for the same video updates rather than duplicating.
	if err := store.SetResumePosition(ctx, "video-1", 456.75); err != nil {
		t.Fatalf("SetResumePosition (update) error: %v", err)
	}
	pos, ok, err = store.GetResumePosition(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetResumePosition error: %v", err)
	}
	if !ok || pos != 456.75 {
		t.Fatalf("got (%v, %v), want (456.75, true)", pos, ok)
	}

	if err := store.ClearResumePosition(ctx, "video-1"); err != nil {
		t.Fatalf("ClearResumePosition error: %v", err)
	}
	if _, ok, err := store.GetResumePosition(ctx, "video-1"); err != nil {
		t.Fatalf("GetResumePosition error: %v", err)
	} else if ok {
		t.Fatal("expected no saved position after clearing")
	}
}

// TestResumePositionSurvivesReindex covers an invariant the Phase 5
// assignment called out explicitly: resume position is user data, not
// part of the disposable media index, so a full reindex (which removes
// and recreates videos rows) must never silently drop it.
func TestResumePositionSurvivesReindex(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	if err := store.UpsertOK(ctx, "video-1", "main", "a.mp4", 100, modTime, media.ProbeResult{DurationSeconds: 60}); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}
	if err := store.SetResumePosition(ctx, "video-1", 30); err != nil {
		t.Fatalf("SetResumePosition error: %v", err)
	}

	// Simulate a full reindex: every videos row for the root disappears
	// (DeleteStale with an empty keep-list) and gets reprobed/reinserted.
	if _, err := store.DeleteStale(ctx, "main", nil); err != nil {
		t.Fatalf("DeleteStale error: %v", err)
	}
	if err := store.UpsertOK(ctx, "video-1", "main", "a.mp4", 100, modTime, media.ProbeResult{DurationSeconds: 60}); err != nil {
		t.Fatalf("UpsertOK (reindex) error: %v", err)
	}

	pos, ok, err := store.GetResumePosition(ctx, "video-1")
	if err != nil {
		t.Fatalf("GetResumePosition error: %v", err)
	}
	if !ok || pos != 30 {
		t.Fatalf("resume position lost across reindex: got (%v, %v), want (30, true)", pos, ok)
	}
}
