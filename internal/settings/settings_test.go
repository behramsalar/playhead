package settings_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"playhead/internal/config"
	"playhead/internal/settings"
)

func TestNotConfiguredUntilFirstSave(t *testing.T) {
	dataDir := t.TempDir()
	store := settings.NewStore(dataDir)

	if err := store.Load(); err != nil {
		t.Fatalf("Load on a fresh DATA_DIR: %v", err)
	}
	if store.Configured() {
		t.Fatal("expected Configured()=false before any config.json exists")
	}
	if _, ok := store.Get(); ok {
		t.Fatal("expected Get() ok=false before any config.json exists")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	store := settings.NewStore(dataDir)

	cfg := settings.Config{
		ServerName:       "My Homelab",
		AuthUsername:     "admin",
		AuthPasswordHash: "$2a$10$fakehash",
		SessionSecret:    "fakesecret",
		Roots: map[string]settings.RootOverride{
			"anime": {DisplayName: "Anime"},
			"misc":  {Hidden: true},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, ok := store.Get()
	if !ok {
		t.Fatal("expected Configured()=true immediately after Save")
	}
	if got.ServerName != cfg.ServerName || got.AuthUsername != cfg.AuthUsername {
		t.Fatalf("Get() after Save = %+v, want %+v", got, cfg)
	}

	// A fresh Store reading the same DATA_DIR must see the same content —
	// this is the "restart the container" scenario.
	store2 := settings.NewStore(dataDir)
	if err := store2.Load(); err != nil {
		t.Fatalf("Load on second store: %v", err)
	}
	got2, ok := store2.Get()
	if !ok {
		t.Fatal("expected the second Store to see the persisted config")
	}
	if got2.ServerName != cfg.ServerName || got2.Roots["misc"].Hidden != true {
		t.Fatalf("second store's Get() = %+v, want %+v", got2, cfg)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dataDir := t.TempDir()
	store := settings.NewStore(dataDir)

	if err := store.Save(settings.Config{ServerName: "First"}); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// No stray temp file left behind after a successful save.
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("expected only config.json in DATA_DIR after Save, found %q", e.Name())
		}
	}

	path := filepath.Join(dataDir, "config.json")
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected a non-empty config.json: %v", err)
	}
}

// TestConcurrentSaveAndGet exercises Save racing Get from many goroutines
// — go test -race must find nothing, and Get must never observe a
// half-written struct (it either sees ok=false or a fully-formed Config,
// never a torn read, since Save swaps the in-memory pointer only after a
// successful write).
func TestConcurrentSaveAndGet(t *testing.T) {
	dataDir := t.TempDir()
	store := settings.NewStore(dataDir)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = store.Save(settings.Config{ServerName: "Server"})
		}(i)
		go func() {
			defer wg.Done()
			if cfg, ok := store.Get(); ok && cfg.ServerName != "Server" {
				t.Errorf("Get() returned a corrupted config: %+v", cfg)
			}
		}()
	}
	wg.Wait()
}

func TestMergeRootsAppliesOverrides(t *testing.T) {
	discovered := []config.Root{
		{ID: "anime", Name: "anime", Path: "/media/anime"},
		{ID: "misc", Name: "misc", Path: "/media/misc"},
		{ID: "tv", Name: "tv", Path: "/media/tv"},
	}
	cfg := settings.Config{
		Roots: map[string]settings.RootOverride{
			"anime": {DisplayName: "Anime Collection"},
			"misc":  {Hidden: true},
			// "tv" has no override at all.
		},
	}

	merged := settings.MergeRoots(discovered, cfg)

	if len(merged) != 2 {
		t.Fatalf("expected misc to be excluded (hidden), got %d roots: %+v", len(merged), merged)
	}
	byID := map[string]config.Root{}
	for _, r := range merged {
		byID[r.ID] = r
	}
	if byID["anime"].Name != "Anime Collection" {
		t.Fatalf("expected anime's display name override to apply, got %q", byID["anime"].Name)
	}
	if byID["tv"].Name != "tv" {
		t.Fatalf("expected tv (no override) to default to its folder name, got %q", byID["tv"].Name)
	}
	if _, present := byID["misc"]; present {
		t.Fatal("expected misc (hidden) to be excluded from the merged list")
	}
}

// TestMergeRootsPreservesOverrideForMissingRoot mirrors the "a removed
// root's override is preserved but the root doesn't appear" requirement:
// MergeRoots only ever operates on discovered roots, so an override for a
// root that isn't currently discovered simply has no effect — but nothing
// here deletes it from cfg.Roots, so it's still there if the root comes
// back.
func TestMergeRootsPreservesOverrideForMissingRoot(t *testing.T) {
	cfg := settings.Config{
		Roots: map[string]settings.RootOverride{
			"unplugged-drive": {DisplayName: "My Archive"},
		},
	}
	merged := settings.MergeRoots(nil, cfg)
	if len(merged) != 0 {
		t.Fatalf("expected no roots when nothing is discovered, got %+v", merged)
	}
	if cfg.Roots["unplugged-drive"].DisplayName != "My Archive" {
		t.Fatal("expected the override for a currently-undiscovered root to remain in cfg.Roots")
	}
}
