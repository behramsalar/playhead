package filesystem_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"playhead/internal/filesystem"
)

func TestCleanRelPath(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"empty is root", "", "", nil},
		{"simple", "sub/file.mp4", "sub/file.mp4", nil},
		{"dot", ".", "", nil},
		{"absolute rejected", "/etc/passwd", "", filesystem.ErrInvalidPath},
		{"parent traversal rejected", "..", "", filesystem.ErrInvalidPath},
		{"nested traversal rejected", "a/../../x", "", filesystem.ErrInvalidPath},
		{"leading traversal rejected", "../x", "", filesystem.ErrInvalidPath},
		{"backslash rejected", "a\\..\\b", "", filesystem.ErrInvalidPath},
		{"null byte rejected", "a\x00b", "", filesystem.ErrInvalidPath},
		{"redundant slashes cleaned", "a//b/./c", "a/b/c", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filesystem.CleanRelPath(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CleanRelPath(%q) err = %v, want %v", tc.input, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("CleanRelPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"), "data")
	mustMkdir(t, filepath.Join(root, "sub"))
	mustWriteFile(t, filepath.Join(root, "sub", "clip.mp4"), "data")

	t.Run("root itself", func(t *testing.T) {
		got, err := filesystem.Resolve(root, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantCanonical, _ := filepath.EvalSymlinks(root)
		if got != wantCanonical {
			t.Fatalf("got %q, want %q", got, wantCanonical)
		}
	})

	t.Run("nested file", func(t *testing.T) {
		got, err := filesystem.Resolve(root, "sub/clip.mp4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if filepath.Base(got) != "clip.mp4" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := filesystem.Resolve(root, "nope.mp4")
		if !errors.Is(err, filesystem.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("missing nested folder", func(t *testing.T) {
		_, err := filesystem.Resolve(root, "sub/does-not-exist/x.mp4")
		if !errors.Is(err, filesystem.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		_, err := filesystem.Resolve(root, "/etc/passwd")
		if !errors.Is(err, filesystem.ErrInvalidPath) {
			t.Fatalf("got %v, want ErrInvalidPath", err)
		}
	})

	t.Run("traversal rejected", func(t *testing.T) {
		_, err := filesystem.Resolve(root, "a/../../x")
		if !errors.Is(err, filesystem.ErrInvalidPath) {
			t.Fatalf("got %v, want ErrInvalidPath", err)
		}
	})

	t.Run("symlink escaping root rejected", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need elevated privileges on windows")
		}
		outside := t.TempDir()
		mustWriteFile(t, filepath.Join(outside, "secret.mp4"), "data")
		linkPath := filepath.Join(root, "escape")
		if err := os.Symlink(outside, linkPath); err != nil {
			t.Skipf("symlinks not supported: %v", err)
		}

		_, err := filesystem.Resolve(root, "escape/secret.mp4")
		if !errors.Is(err, filesystem.ErrOutsideRoot) {
			t.Fatalf("got %v, want ErrOutsideRoot", err)
		}
	})

	t.Run("root inaccessible", func(t *testing.T) {
		_, err := filesystem.Resolve(filepath.Join(root, "does-not-exist-root"), "")
		if !errors.Is(err, filesystem.ErrRootInaccessible) {
			t.Fatalf("got %v, want ErrRootInaccessible", err)
		}
	})
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
