package filesystem_test

import (
	"path/filepath"
	"testing"

	"playhead/internal/filesystem"
)

// resetConfigure restores the default (empty) exclusion config after a
// test that calls filesystem.Configure, since it's package-level state
// shared across this package's tests.
func resetConfigure(t *testing.T) {
	t.Cleanup(func() { filesystem.Configure(nil, nil, nil) })
}

func TestConfigureExtraHiddenNames(t *testing.T) {
	resetConfigure(t)
	filesystem.Configure([]string{"Thumbs.db", "desktop.ini"}, nil, nil)

	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Thumbs.db")) // built-in list only covers files/folders by exact name match
	mustWriteFile(t, filepath.Join(root, "movie.mp4"), "data")

	result, err := filesystem.Browse("main", "Videos", root, "")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(result.Folders) != 0 {
		t.Fatalf("expected the configured extra hidden name to be excluded, got folders: %+v", result.Folders)
	}
	if len(result.Videos) != 1 {
		t.Fatalf("expected the unrelated video to still show, got: %+v", result.Videos)
	}

	// Case-insensitive, matching the built-in list's behavior.
	if !filesystem.IsHidden("THUMBS.DB") {
		t.Fatalf("expected case-insensitive match on configured extra hidden name")
	}
}

func TestConfigureExcludedPaths(t *testing.T) {
	resetConfigure(t)
	filesystem.Configure(nil, []string{"Raw Footage"}, nil)

	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Raw Footage"))
	mustWriteFile(t, filepath.Join(root, "Raw Footage", "clip.mp4"), "data")
	mustMkdir(t, filepath.Join(root, "Finished"))
	mustWriteFile(t, filepath.Join(root, "Finished", "final.mp4"), "data")

	result, err := filesystem.Browse("main", "Videos", root, "")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(result.Folders) != 1 || result.Folders[0].Name != "Finished" {
		t.Fatalf("expected only the non-excluded folder, got: %+v", result.Folders)
	}

	// Direct navigation into the excluded folder itself also shows no
	// videos: entries are excluded by their full relative path, which for
	// anything under "Raw Footage" always starts with that prefix
	// regardless of which folder is currently being browsed. Combined with
	// the indexer reusing Browse() for its own walk (and never seeing
	// "Raw Footage" as a folder to descend into from the root listing
	// above), this means an excluded path is never indexed either.
	nested, err := filesystem.Browse("main", "Videos", root, "Raw Footage")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(nested.Videos) != 0 {
		t.Fatalf("expected the excluded folder's own contents to also be hidden, got %+v", nested.Videos)
	}
}

func TestConfigureExcludedExtensions(t *testing.T) {
	resetConfigure(t)
	filesystem.Configure(nil, nil, []string{".avi"})

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "clip.avi"), "data")
	mustWriteFile(t, filepath.Join(root, "clip.mp4"), "data")

	result, err := filesystem.Browse("main", "Videos", root, "")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(result.Videos) != 1 || result.Videos[0].Name != "clip.mp4" {
		t.Fatalf("expected only the non-excluded extension, got: %+v", result.Videos)
	}
}
