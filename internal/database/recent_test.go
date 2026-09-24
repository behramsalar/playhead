package database_test

import (
	"context"
	"testing"
	"time"

	"playhead/internal/media"
)

// TestListRecentlyAddedOrdersNewestFirst inserts three videos with real
// (second-resolution) time gaps between them — first_indexed_at_unix is
// stamped from time.Now() inside UpsertOK, with no way to inject an
// arbitrary value through the public API, so this sleeps rather than
// faking timestamps.
func TestListRecentlyAddedOrdersNewestFirst(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"oldest", "middle", "newest"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mp4", 100, time.Unix(1_700_000_000, 0), probe); err != nil {
			t.Fatalf("UpsertOK(%s): %v", id, err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	rows, err := store.ListRecentlyAdded(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListRecentlyAdded error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	got := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	want := []string{"newest", "middle", "oldest"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestListRecentlyAddedScopesToRoot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}

	if err := store.UpsertOK(ctx, "id-a", "main", "a.mp4", 100, time.Unix(1_700_000_000, 0), probe); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertOK(ctx, "id-b", "other", "b.mp4", 100, time.Unix(1_700_000_000, 0), probe); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListRecentlyAdded(ctx, "main", 10)
	if err != nil {
		t.Fatalf("ListRecentlyAdded error: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "id-a" {
		t.Fatalf("expected only id-a scoped to root=main, got %+v", rows)
	}

	all, err := store.ListRecentlyAdded(ctx, "", 10)
	if err != nil {
		t.Fatalf("ListRecentlyAdded (all roots) error: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected both videos with no root filter, got %+v", all)
	}
}

func TestListRecentlyAddedRespectsLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}

	for _, id := range []string{"id-1", "id-2", "id-3"} {
		if err := store.UpsertOK(ctx, id, "main", id+".mp4", 100, time.Unix(1_700_000_000, 0), probe); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := store.ListRecentlyAdded(ctx, "", 2)
	if err != nil {
		t.Fatalf("ListRecentlyAdded error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected exactly 2 rows (limit), got %d", len(rows))
	}
}
