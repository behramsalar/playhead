package database_test

import (
	"context"
	"testing"
	"time"

	"playhead/internal/media"
)

// TestFirstIndexedAtSetOnceByUpsertOK confirms first_indexed_at_unix is
// set on a fresh row and never changes on a later re-probe — it means
// "when this video first appeared", not "when it was last indexed"
// (indexed_at_unix already means the latter, and does change).
func TestFirstIndexedAtSetOnceByUpsertOK(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}

	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, probe); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}
	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok {
		t.Fatalf("GetMetadata: ok=%v err=%v", ok, err)
	}
	firstSeen := meta.FirstIndexedAtUnix
	if firstSeen == 0 {
		t.Fatal("expected a non-zero first_indexed_at_unix after the first upsert")
	}

	// Re-probe (file changed): first_indexed_at_unix must stay the same,
	// even though indexed_at_unix itself would legitimately change.
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 200, modTime, probe); err != nil {
		t.Fatalf("UpsertOK (re-probe) error: %v", err)
	}
	meta2, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok {
		t.Fatalf("GetMetadata (2nd): ok=%v err=%v", ok, err)
	}
	if meta2.FirstIndexedAtUnix != firstSeen {
		t.Fatalf("first_indexed_at_unix changed on re-probe: was %d, now %d", firstSeen, meta2.FirstIndexedAtUnix)
	}
}

// TestFirstIndexedAtSurvivesLaterProbe confirms the same "set once" rule
// holds when the row is first created by a thumbnail job (a minimal row,
// indexed_at_unix=0 sentinel) and only probed by the indexer afterwards —
// first_indexed_at_unix should reflect the thumbnail job's timestamp, not
// get reset when UpsertOK later fills in the rest.
func TestFirstIndexedAtSurvivesLaterProbe(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SetThumbOK(ctx, "id-1", "main", "a.mp4", 100, 1_700_000_000); err != nil {
		t.Fatalf("SetThumbOK error: %v", err)
	}
	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok {
		t.Fatalf("GetMetadata: ok=%v err=%v", ok, err)
	}
	firstSeen := meta.FirstIndexedAtUnix
	if firstSeen == 0 {
		t.Fatal("expected a non-zero first_indexed_at_unix from the thumbnail-only row")
	}

	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, probe); err != nil {
		t.Fatalf("UpsertOK error: %v", err)
	}
	meta2, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok {
		t.Fatalf("GetMetadata (2nd): ok=%v err=%v", ok, err)
	}
	if meta2.FirstIndexedAtUnix != firstSeen {
		t.Fatalf("expected first_indexed_at_unix to survive the later probe: was %d, now %d", firstSeen, meta2.FirstIndexedAtUnix)
	}
}

func TestFirstIndexedAtSetByUpsertError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)

	if err := store.UpsertError(ctx, "id-1", "main", "a.mp4", 100, modTime, "boom"); err != nil {
		t.Fatalf("UpsertError error: %v", err)
	}
	meta, ok, err := store.GetMetadata(ctx, "id-1")
	if err != nil || !ok {
		t.Fatalf("GetMetadata: ok=%v err=%v", ok, err)
	}
	if meta.FirstIndexedAtUnix == 0 {
		t.Fatal("expected a non-zero first_indexed_at_unix even for a failed probe")
	}
}

func TestQueryVideosUnderPathIncludesFirstIndexedAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	modTime := time.Unix(1_700_000_000, 0)
	probe := media.ProbeResult{DurationSeconds: 5, VideoCodec: "h264", AudioCodec: "aac"}
	if err := store.UpsertOK(ctx, "id-1", "main", "a.mp4", 100, modTime, probe); err != nil {
		t.Fatal(err)
	}

	rows, err := store.QueryVideosUnderPath(ctx, "main", "")
	if err != nil {
		t.Fatalf("QueryVideosUnderPath error: %v", err)
	}
	if len(rows) != 1 || rows[0].Metadata.FirstIndexedAtUnix == 0 {
		t.Fatalf("expected 1 row with a non-zero FirstIndexedAtUnix, got %+v", rows)
	}
}
