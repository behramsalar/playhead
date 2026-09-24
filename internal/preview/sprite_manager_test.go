package preview_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"playhead/internal/config"
	"playhead/internal/filesystem"
	"playhead/internal/preview"
)

func TestManagerGeneratesSprite(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	clip := filepath.Join(root, "a.mp4")
	generateClip(t, clip, 10)

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	dataDir := t.TempDir()
	mgr := preview.New(context.Background(), store, roots, dataDir, 2, 0)

	id := filesystem.EncodeVideoID("main", "a.mp4")
	result, err := mgr.RequestSprite(context.Background(), id, "main", "a.mp4", clip, true)
	if err != nil {
		t.Fatalf("RequestSprite error: %v", err)
	}
	if !result.Ready || result.ImagePath == "" || result.MetaPath == "" {
		t.Fatalf("expected a ready sprite, got %+v", result)
	}

	if info, err := os.Stat(result.ImagePath); err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty sprite image at %s: %v", result.ImagePath, err)
	}
	metaBytes, err := os.ReadFile(result.MetaPath)
	if err != nil {
		t.Fatalf("reading sprite metadata: %v", err)
	}
	var meta preview.SpriteMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("decoding sprite metadata: %v", err)
	}
	if len(meta.Frames) < 20 {
		t.Fatalf("expected at least 20 frames, got %d", len(meta.Frames))
	}
	if meta.Frames[0].Time != 0 {
		t.Fatalf("first frame time = %v, want 0", meta.Frames[0].Time)
	}

	// Second request is a cache hit — same paths, no regeneration.
	result2, err := mgr.RequestSprite(context.Background(), id, "main", "a.mp4", clip, false)
	if err != nil {
		t.Fatalf("second RequestSprite error: %v", err)
	}
	if !result2.Ready || result2.ImagePath != result.ImagePath {
		t.Fatalf("expected cache hit reusing %s, got %+v", result.ImagePath, result2)
	}
}

func TestManagerSpriteDoesNotRetryPermanentFailure(t *testing.T) {
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

	first, err := mgr.RequestSprite(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("RequestSprite error: %v", err)
	}
	if !first.Failed {
		t.Fatalf("expected a broken file to fail sprite generation, got %+v", first)
	}

	status, spriteErr, ok, err := store.GetSpriteStatus(context.Background(), id)
	if err != nil || !ok || status != "error" || spriteErr == nil {
		t.Fatalf("expected persisted error status, got status=%q ok=%v err=%v (store err=%v)", status, ok, spriteErr, err)
	}

	start := time.Now()
	second, err := mgr.RequestSprite(context.Background(), id, "main", "broken.mkv", broken, true)
	if err != nil {
		t.Fatalf("second RequestSprite error: %v", err)
	}
	if !second.Failed {
		t.Fatalf("expected the second request to also report Failed, got %+v", second)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("second request took %v, expected an immediate short-circuit", elapsed)
	}
}

// TestManagerSharesWorkerPoolAcrossThumbnailsAndSprites is the concrete
// check behind "total FFmpeg concurrency must stay bounded across both
// job kinds": firing thumbnail and sprite requests for several videos at
// once, at a pool bounded well below the total job count, must still let
// every job complete rather than deadlocking or spawning unboundedly.
func TestManagerSharesWorkerPoolAcrossThumbnailsAndSprites(t *testing.T) {
	requireWebP(t)

	root := t.TempDir()
	const n = 3
	clips := make([]string, n)
	ids := make([]string, n)
	relPaths := make([]string, n)
	for i := range clips {
		relPaths[i] = string(rune('a'+i)) + ".mp4"
		clips[i] = filepath.Join(root, relPaths[i])
		generateClip(t, clips[i], 5)
		ids[i] = filesystem.EncodeVideoID("main", relPaths[i])
	}

	store := newTestStore(t)
	roots := []config.Root{{ID: "main", Name: "Videos", Path: root}}
	const workers = 2 // fewer than n*2 (thumbnail+sprite) jobs below
	mgr := preview.New(context.Background(), store, roots, t.TempDir(), workers, 0)

	var wg sync.WaitGroup
	for i := range clips {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if _, err := mgr.Request(context.Background(), ids[i], "main", relPaths[i], clips[i], true); err != nil {
				t.Errorf("Request(%d) error: %v", i, err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if _, err := mgr.RequestSprite(context.Background(), ids[i], "main", relPaths[i], clips[i], true); err != nil {
				t.Errorf("RequestSprite(%d) error: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	thumbStatus, err := mgr.Status(context.Background())
	if err != nil {
		t.Fatalf("Status error: %v", err)
	}
	if thumbStatus.Ready != n {
		t.Fatalf("thumbnail Ready = %d, want %d", thumbStatus.Ready, n)
	}

	spriteStatus, err := mgr.SpriteStatus(context.Background())
	if err != nil {
		t.Fatalf("SpriteStatus error: %v", err)
	}
	if spriteStatus.Ready != n {
		t.Fatalf("sprite Ready = %d, want %d", spriteStatus.Ready, n)
	}
}
