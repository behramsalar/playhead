package config

import (
	"os"
	"path/filepath"
	"testing"
)

// withMockDeviceIDs replaces deviceIDFunc for the duration of the test
// with one driven by a path->device map; any path not in the map falls
// back to the real deviceID (so a genuinely-missing/unreadable path still
// behaves like ok=false, exactly as production code would see it). Real
// separate mounts can't be created in a unit test without OS-level
// privileges, so this is the seam that makes the promotion logic
// testable at all.
func withMockDeviceIDs(t *testing.T, devices map[string]uint64) {
	t.Helper()
	orig := deviceIDFunc
	t.Cleanup(func() { deviceIDFunc = orig })
	deviceIDFunc = func(path string) (uint64, bool) {
		if dev, ok := devices[path]; ok {
			return dev, true
		}
		return deviceID(path)
	}
}

func mustMkdirT(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

// TestDiscoverRootsPromotesDistinctMount is the core scenario this
// design exists for: MEDIA_ROOT itself has one device, but one
// subdirectory (a genuinely separate Docker volume, e.g. `-v
// host:/media/Movies`) reports a different one — it must become its own
// promoted root, while a sibling on the same device as MEDIA_ROOT stays
// merged into "main".
func TestDiscoverRootsPromotesDistinctMount(t *testing.T) {
	parent := t.TempDir()
	moviesPath := filepath.Join(parent, "Movies")
	miscPath := filepath.Join(parent, "misc")
	mustMkdirT(t, moviesPath)
	mustMkdirT(t, miscPath)
	if err := os.WriteFile(filepath.Join(miscPath, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	withMockDeviceIDs(t, map[string]uint64{
		parent:     1,
		moviesPath: 2, // distinct — a separate mount
		miscPath:   1, // same as parent — just a subfolder
	})

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}

	byID := map[string]Root{}
	for _, r := range roots {
		byID[r.ID] = r
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (main + Movies), got %+v", roots)
	}
	if _, ok := byID["Movies"]; !ok {
		t.Fatalf("expected Movies promoted to its own root, got %+v", roots)
	}
	if byID["Movies"].Path != moviesPath {
		t.Fatalf("Movies.Path = %q, want %q", byID["Movies"].Path, moviesPath)
	}

	main, ok := byID["main"]
	if !ok {
		t.Fatalf("expected a main root for the leftover (misc, same device as MEDIA_ROOT), got %+v", roots)
	}
	if main.Path != parent {
		t.Fatalf("main.Path = %q, want %q", main.Path, parent)
	}
	// The whole point of HiddenChildren: main's own Path is `parent`,
	// which filesystem-wise still contains Movies/ — it must be excluded
	// from main's browse view, or Movies' files would be reachable under
	// two different video IDs.
	if len(main.HiddenChildren) != 1 || main.HiddenChildren[0] != "Movies" {
		t.Fatalf("main.HiddenChildren = %v, want [\"Movies\"]", main.HiddenChildren)
	}
}

// TestDiscoverRootsAllPromotedOmitsMain confirms that when every
// subdirectory is a genuinely separate mount and MEDIA_ROOT has no loose
// files of its own, no synthetic "main" root is created at all — there's
// nothing left for it to show.
func TestDiscoverRootsAllPromotedOmitsMain(t *testing.T) {
	parent := t.TempDir()
	moviesPath := filepath.Join(parent, "Movies")
	tvPath := filepath.Join(parent, "TV")
	mustMkdirT(t, moviesPath)
	mustMkdirT(t, tvPath)

	withMockDeviceIDs(t, map[string]uint64{
		parent:     1,
		moviesPath: 2,
		tvPath:     3,
	})

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected exactly 2 promoted roots and no main, got %+v", roots)
	}
	for _, r := range roots {
		if r.ID == "main" {
			t.Fatalf("expected no main root when everything was promoted and there are no loose files, got %+v", roots)
		}
	}
}

// TestDiscoverRootsLooseFileForcesMain confirms that even if every
// subdirectory is promoted, a loose file directly in MEDIA_ROOT still
// earns it a "main" root (there's real content there to show) — with no
// HiddenChildren needed for a symlinked/promoted-free root, since there's
// nothing browsable left for main to duplicate.
func TestDiscoverRootsLooseFileForcesMain(t *testing.T) {
	parent := t.TempDir()
	moviesPath := filepath.Join(parent, "Movies")
	mustMkdirT(t, moviesPath)
	if err := os.WriteFile(filepath.Join(parent, "readme.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	withMockDeviceIDs(t, map[string]uint64{
		parent:     1,
		moviesPath: 2,
	})

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	byID := map[string]Root{}
	for _, r := range roots {
		byID[r.ID] = r
	}
	if _, ok := byID["main"]; !ok {
		t.Fatalf("expected a main root for the loose file, got %+v", roots)
	}
	if _, ok := byID["Movies"]; !ok {
		t.Fatalf("expected Movies still promoted, got %+v", roots)
	}
}

// TestDiscoverRootsIgnoresEmptySameDeviceDirs is a real-world regression
// guard: Alpine's base image (this app's own Docker image) ships
// pre-existing empty /media/cdrom, /media/floppy, /media/usb directories
// (a standard FHS removable-media convention), and /media is exactly the
// path this project's own docs recommend mounting media at. An empty
// same-device subdirectory must not single-handedly force a spurious
// "main" root into existence alongside genuinely promoted roots.
func TestDiscoverRootsIgnoresEmptySameDeviceDirs(t *testing.T) {
	parent := t.TempDir()
	videosPath := filepath.Join(parent, "videos")
	videos2Path := filepath.Join(parent, "videos2")
	emptyPath := filepath.Join(parent, "cdrom")
	mustMkdirT(t, videosPath)
	mustMkdirT(t, videos2Path)
	mustMkdirT(t, emptyPath) // same device as parent, and empty

	withMockDeviceIDs(t, map[string]uint64{
		parent:      1,
		videosPath:  2,
		videos2Path: 3,
		emptyPath:   1,
	})

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	if len(roots) != 2 {
		t.Fatalf("expected only the 2 promoted roots (empty cdrom/ ignored), got %+v", roots)
	}
	for _, r := range roots {
		if r.ID == mainRootID {
			t.Fatalf("expected no spurious main root for an empty same-device directory, got %+v", roots)
		}
	}
}

// TestDiscoverRootsNonEmptySameDeviceDirStillForcesMain confirms the
// empty-directory exception above doesn't over-apply: a same-device
// subdirectory that actually has content still earns "main".
func TestDiscoverRootsNonEmptySameDeviceDirStillForcesMain(t *testing.T) {
	parent := t.TempDir()
	videosPath := filepath.Join(parent, "videos")
	miscPath := filepath.Join(parent, "misc")
	mustMkdirT(t, videosPath)
	mustMkdirT(t, miscPath)
	if err := os.WriteFile(filepath.Join(miscPath, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	withMockDeviceIDs(t, map[string]uint64{
		parent:     1,
		videosPath: 2,
		miscPath:   1,
	})

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	byID := map[string]Root{}
	for _, r := range roots {
		byID[r.ID] = r
	}
	if _, ok := byID[mainRootID]; !ok {
		t.Fatalf("expected main for the non-empty misc/ subdirectory, got %+v", roots)
	}
}

// TestDiscoverRootsFallsBackWithoutDeviceInfo confirms that when device
// detection isn't available at all (parent's own device can't be
// determined), nothing is promoted — the safe, conservative fallback:
// never silently split libraries when we can't actually tell whether a
// subdirectory is a separate mount.
func TestDiscoverRootsFallsBackWithoutDeviceInfo(t *testing.T) {
	orig := deviceIDFunc
	t.Cleanup(func() { deviceIDFunc = orig })
	deviceIDFunc = func(path string) (uint64, bool) { return 0, false }

	parent := t.TempDir()
	animeDir := filepath.Join(parent, "anime")
	mustMkdirT(t, animeDir)
	mustMkdirT(t, filepath.Join(parent, "misc"))
	if err := os.WriteFile(filepath.Join(animeDir, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != "main" {
		t.Fatalf("expected a single main root when device detection is unavailable, got %+v", roots)
	}
}
