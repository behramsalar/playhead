package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"playhead/internal/config"
)

// A real t.TempDir() and its real subdirectories always share the same
// filesystem device (no actual separate mount involved), so these
// black-box tests exercise DiscoverRoots' default "everything on one
// mount collapses into a single 'main' root, browsed as one folder tree"
// behavior — the common single-volume case. The device-based promotion
// path (genuinely separate mounts) needs mock device IDs and is covered
// by mount_test.go instead, which can see the unexported test seam.
func TestDiscoverRootsMergesSameDeviceContentIntoMain(t *testing.T) {
	parent := t.TempDir()
	mustMkdir(t, filepath.Join(parent, "anime"))
	mustMkdir(t, filepath.Join(parent, "misc"))
	mustMkdir(t, filepath.Join(parent, ".hidden"))
	mustMkdir(t, filepath.Join(parent, "@eaDir"))
	if err := os.WriteFile(filepath.Join(parent, "not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := config.DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}

	if len(roots) != 1 {
		t.Fatalf("expected a single merged 'main' root (anime/misc share MEDIA_ROOT's own device), got %+v", roots)
	}
	main := roots[0]
	if main.ID != "main" {
		t.Fatalf("ID = %q, want %q", main.ID, "main")
	}
	if main.Path != parent {
		t.Fatalf("Path = %q, want %q (MEDIA_ROOT itself, browsed as one folder tree)", main.Path, parent)
	}
	if main.Name == "" {
		t.Fatal("expected a non-empty default display name")
	}
	// anime/misc aren't promoted, so nothing needs hiding from main's
	// own browse view.
	if len(main.HiddenChildren) != 0 {
		t.Fatalf("expected no HiddenChildren when nothing was promoted, got %v", main.HiddenChildren)
	}
}

// TestDiscoverRootsSkipsBrokenSymlink confirms a broken symlink never
// becomes a root (or crashes discovery), while a real subdirectory next
// to it is still merged into "main" as usual (real-root shares
// MEDIA_ROOT's own device — t.TempDir() never fakes a separate mount).
func TestDiscoverRootsSkipsBrokenSymlink(t *testing.T) {
	parent := t.TempDir()
	if err := os.Symlink(filepath.Join(parent, "does-not-exist"), filepath.Join(parent, "broken")); err != nil {
		t.Skipf("symlinks not supported in this environment: %v", err)
	}
	realRoot := filepath.Join(parent, "real-root")
	mustMkdir(t, realRoot)
	if err := os.WriteFile(filepath.Join(realRoot, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	roots, err := config.DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != "main" || roots[0].Path != parent {
		t.Fatalf("expected the broken symlink skipped and real-root merged into a single main root, got %+v", roots)
	}
}

func TestDiscoverRootsEmptyParentIsValid(t *testing.T) {
	parent := t.TempDir()
	roots, err := config.DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots on an empty parent: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("expected zero roots, got %+v", roots)
	}
}

// TestDiscoverRootsHiddenOnlyContentIsValid confirms a MEDIA_ROOT whose
// only content is hidden/NAS-clutter entries reports zero roots (nothing
// visible to show), the same as a genuinely empty parent — not a "main"
// root wrapping invisible content.
func TestDiscoverRootsHiddenOnlyContentIsValid(t *testing.T) {
	parent := t.TempDir()
	mustMkdir(t, filepath.Join(parent, ".hidden"))
	mustMkdir(t, filepath.Join(parent, "@eaDir"))

	roots, err := config.DiscoverRoots(parent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("expected zero roots for hidden-only content, got %+v", roots)
	}
}

func TestLoadRejectsMissingMediaRoot(t *testing.T) {
	t.Setenv("MEDIA_ROOT", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := config.Load(); err == nil {
		t.Fatal("expected Load to fail when MEDIA_ROOT doesn't exist")
	}
}

func TestLoadRejectsMediaRootThatIsAFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_ROOT", file)
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := config.Load(); err == nil {
		t.Fatal("expected Load to fail when MEDIA_ROOT is a file, not a directory")
	}
}

func TestLoadAcceptsValidMediaRootParent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MEDIA_ROOT", dir)
	t.Setenv("DATA_DIR", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.MediaRoot != dir {
		t.Fatalf("MediaRoot = %q, want %q", cfg.MediaRoot, dir)
	}
	if len(cfg.Roots) != 0 {
		t.Fatalf("expected Roots to start empty (populated later via DiscoverRoots), got %+v", cfg.Roots)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
