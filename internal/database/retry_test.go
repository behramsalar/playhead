package database_test

import (
	"context"
	"testing"
	"time"
)

// exhaustThumbRetries fails id's thumbnail, then requeues+fails it
// maxRetries more times, leaving it in a permanently-errored (for now)
// state — the retry budget genuinely exhausted, not just a single
// failure. Returns the final RequeueRetryableThumbnails count (must be 0).
func exhaustThumbRetries(t *testing.T, store interface {
	SetThumbError(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, thumbErr string) error
	RequeueRetryableThumbnails(ctx context.Context, maxRetries int) (int64, error)
}, ctx context.Context, id string, maxRetries int) {
	t.Helper()
	if err := store.SetThumbError(ctx, id, "main", "a.mp4", 100, 1_700_000_000, "boom"); err != nil {
		t.Fatalf("SetThumbError: %v", err)
	}
	for i := 0; i < maxRetries; i++ {
		n, err := store.RequeueRetryableThumbnails(ctx, maxRetries)
		if err != nil {
			t.Fatalf("RequeueRetryableThumbnails (cycle %d): %v", i, err)
		}
		if n != 1 {
			t.Fatalf("cycle %d: expected 1 requeued, got %d", i, n)
		}
		if err := store.SetThumbError(ctx, id, "main", "a.mp4", 100, 1_700_000_000, "boom again"); err != nil {
			t.Fatalf("SetThumbError (cycle %d): %v", i, err)
		}
	}
	n, err := store.RequeueRetryableThumbnails(ctx, maxRetries)
	if err != nil {
		t.Fatalf("RequeueRetryableThumbnails (final): %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 requeued once the retry budget is exhausted, got %d", n)
	}
}

func TestRequeueRetryableThumbnailsRespectsCap(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const maxRetries = 3

	exhaustThumbRetries(t, store, ctx, "id-1", maxRetries)

	status, _, ok, err := store.GetThumbStatus(ctx, "id-1")
	if err != nil || !ok || status != "error" {
		t.Fatalf("expected the video to remain errored once retries are exhausted, got status=%q ok=%v err=%v", status, ok, err)
	}
}

func TestRequeueRetryableThumbnailsIgnoresNonErrorRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SetThumbOK(ctx, "ok-video", "main", "a.mp4", 100, 1_700_000_000); err != nil {
		t.Fatalf("SetThumbOK: %v", err)
	}
	n, err := store.RequeueRetryableThumbnails(ctx, 3)
	if err != nil {
		t.Fatalf("RequeueRetryableThumbnails: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected a successfully-generated video to never be requeued, got %d", n)
	}
}

func TestResetThumbStatusRestoresRetryBudget(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const maxRetries = 3

	exhaustThumbRetries(t, store, ctx, "id-1", maxRetries)

	// The manual per-video retry control (e.g. the frontend's "retry"
	// button) must give the video a fresh automatic-retry budget too —
	// otherwise a video a user explicitly retried would immediately stop
	// being eligible for the *automatic* sweep again on its very next
	// failure, silently reverting to "stuck until another manual click".
	if err := store.ResetThumbStatus(ctx, "id-1"); err != nil {
		t.Fatalf("ResetThumbStatus: %v", err)
	}
	if err := store.SetThumbError(ctx, "id-1", "main", "a.mp4", 100, 1_700_000_000, "boom once more"); err != nil {
		t.Fatalf("SetThumbError: %v", err)
	}
	n, err := store.RequeueRetryableThumbnails(ctx, maxRetries)
	if err != nil {
		t.Fatalf("RequeueRetryableThumbnails: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected the manually-reset video to be retryable again, got %d requeued", n)
	}
}

func TestUpsertResetsRetryBudgetOnFileChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const maxRetries = 3

	exhaustThumbRetries(t, store, ctx, "id-1", maxRetries)

	// A changed file (new size) is effectively a different thumbnail job
	// — it deserves a fresh retry budget, not to inherit a cap already hit
	// by the file's previous content.
	if err := store.UpsertError(ctx, "id-1", "main", "a.mp4", 999, time.Unix(1_700_000_000, 0), "probe failed"); err != nil {
		t.Fatalf("UpsertError: %v", err)
	}
	if err := store.SetThumbError(ctx, "id-1", "main", "a.mp4", 999, 1_700_000_000, "boom on new content"); err != nil {
		t.Fatalf("SetThumbError: %v", err)
	}
	n, err := store.RequeueRetryableThumbnails(ctx, maxRetries)
	if err != nil {
		t.Fatalf("RequeueRetryableThumbnails: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected the changed file to be retryable again, got %d requeued", n)
	}
}

func TestResetAllPreviewStatusRestoresRetryBudgets(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const maxRetries = 3

	exhaustThumbRetries(t, store, ctx, "id-1", maxRetries)

	if err := store.ResetAllPreviewStatus(ctx); err != nil {
		t.Fatalf("ResetAllPreviewStatus: %v", err)
	}
	if err := store.SetThumbError(ctx, "id-1", "main", "a.mp4", 100, 1_700_000_000, "boom after rebuild"); err != nil {
		t.Fatalf("SetThumbError: %v", err)
	}
	n, err := store.RequeueRetryableThumbnails(ctx, maxRetries)
	if err != nil {
		t.Fatalf("RequeueRetryableThumbnails: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected a full rebuild to restore the retry budget, got %d requeued", n)
	}
}

// TestRequeueRetryableSpritesAndRemux is a lighter mirror of the
// thumbnail tests above, just enough to catch a copy-paste mistake in
// the parallel sprite/remux SQL (see RequeueRetryableSprites/
// RequeueRetryableRemux) rather than re-proving the whole mechanism a
// third and fourth time.
func TestRequeueRetryableSpritesAndRemux(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SetSpriteError(ctx, "id-1", "main", "a.mp4", 100, 1_700_000_000, "boom"); err != nil {
		t.Fatalf("SetSpriteError: %v", err)
	}
	if err := store.SetRemuxError(ctx, "id-1", "main", "a.mp4", 100, 1_700_000_000, "boom"); err != nil {
		t.Fatalf("SetRemuxError: %v", err)
	}

	if n, err := store.RequeueRetryableSprites(ctx, 3); err != nil || n != 1 {
		t.Fatalf("RequeueRetryableSprites: n=%d err=%v, want n=1", n, err)
	}
	if n, err := store.RequeueRetryableRemux(ctx, 3); err != nil || n != 1 {
		t.Fatalf("RequeueRetryableRemux: n=%d err=%v, want n=1", n, err)
	}

	spriteStatus, _, _, err := store.GetSpriteStatus(ctx, "id-1")
	if err != nil || spriteStatus != "pending" {
		t.Fatalf("sprite status after requeue = %q, want pending (err=%v)", spriteStatus, err)
	}
	remuxStatus, _, _, err := store.GetRemuxStatus(ctx, "id-1")
	if err != nil || remuxStatus != "pending" {
		t.Fatalf("remux status after requeue = %q, want pending (err=%v)", remuxStatus, err)
	}
}
