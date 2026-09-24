package filesystem_test

import (
	"errors"
	"path/filepath"
	"testing"

	"playhead/internal/filesystem"
)

func TestBrowseRoot(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Zeta"))
	mustMkdir(t, filepath.Join(root, "alpha"))
	mustWriteFile(t, filepath.Join(root, "b.mp4"), "data")
	mustWriteFile(t, filepath.Join(root, "a.mp4"), "data")
	mustWriteFile(t, filepath.Join(root, "notes.txt"), "not a video")
	mustWriteFile(t, filepath.Join(root, ".hidden.mp4"), "data")
	mustMkdir(t, filepath.Join(root, "@eaDir"))
	mustMkdir(t, filepath.Join(root, "#recycle"))

	result, err := filesystem.Browse("main", "Videos", root, "")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}

	if len(result.Folders) != 2 || result.Folders[0].Name != "alpha" || result.Folders[1].Name != "Zeta" {
		t.Fatalf("unexpected folders: %+v", result.Folders)
	}
	if len(result.Videos) != 2 || result.Videos[0].Name != "a.mp4" || result.Videos[1].Name != "b.mp4" {
		t.Fatalf("unexpected videos: %+v", result.Videos)
	}
	if len(result.Breadcrumbs) != 1 || result.Breadcrumbs[0].Name != "Videos" || result.Breadcrumbs[0].Path != "" {
		t.Fatalf("unexpected breadcrumbs: %+v", result.Breadcrumbs)
	}
}

// TestBrowseHidesTopLevelNamesOnly confirms the hiddenTopLevel parameter
// (used by config.DiscoverRoots' synthetic "main" root to exclude
// subdirectories promoted into their own separate roots) only hides an
// exact-name match at the browsed root's own top level, never a
// same-named entry deeper in the tree — a promoted mount is always an
// immediate child of MEDIA_ROOT, so hiding it anywhere else would risk
// hiding an unrelated folder that just happens to share its name.
func TestBrowseHidesTopLevelNamesOnly(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Movies")) // promoted elsewhere; must not appear here
	mustMkdir(t, filepath.Join(root, "Anime"))
	mustMkdir(t, filepath.Join(root, "Anime", "Movies")) // same name, but nested — must still show

	top, err := filesystem.Browse("main", "Videos", root, "", "Movies")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(top.Folders) != 1 || top.Folders[0].Name != "Anime" {
		t.Fatalf("expected only Anime at the top level (Movies hidden), got %+v", top.Folders)
	}

	nested, err := filesystem.Browse("main", "Videos", root, "Anime", "Movies")
	if err != nil {
		t.Fatalf("Browse (nested) error: %v", err)
	}
	if len(nested.Folders) != 1 || nested.Folders[0].Name != "Movies" {
		t.Fatalf("expected the nested Movies folder to still show (hiddenTopLevel only applies at the root), got %+v", nested.Folders)
	}
}

func TestBrowseNestedFolder(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "Shows", "Season 1"))
	mustWriteFile(t, filepath.Join(root, "Shows", "Season 1", "ep2.mp4"), "data")
	mustWriteFile(t, filepath.Join(root, "Shows", "Season 1", "ep10.mp4"), "data")

	result, err := filesystem.Browse("main", "Videos", root, "Shows/Season 1")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if result.Path != "Shows/Season 1" {
		t.Fatalf("got path %q", result.Path)
	}
	if len(result.Videos) != 2 || result.Videos[0].Name != "ep2.mp4" || result.Videos[1].Name != "ep10.mp4" {
		t.Fatalf("expected natural sort ep2 before ep10, got %+v", result.Videos)
	}
	wantCrumbs := []string{"Videos", "Shows", "Season 1"}
	if len(result.Breadcrumbs) != len(wantCrumbs) {
		t.Fatalf("unexpected breadcrumbs: %+v", result.Breadcrumbs)
	}
	for i, name := range wantCrumbs {
		if result.Breadcrumbs[i].Name != name {
			t.Fatalf("breadcrumb %d = %q, want %q", i, result.Breadcrumbs[i].Name, name)
		}
	}
	if result.Breadcrumbs[2].Path != "Shows/Season 1" {
		t.Fatalf("last breadcrumb path = %q", result.Breadcrumbs[2].Path)
	}
}

func TestBrowseVideoIDsAreStable(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "movie.mp4"), "data")

	result, err := filesystem.Browse("main", "Videos", root, "")
	if err != nil {
		t.Fatalf("Browse error: %v", err)
	}
	if len(result.Videos) != 1 {
		t.Fatalf("expected 1 video, got %d", len(result.Videos))
	}
	gotRoot, gotPath, err := filesystem.DecodeVideoID(result.Videos[0].ID)
	if err != nil {
		t.Fatalf("DecodeVideoID error: %v", err)
	}
	if gotRoot != "main" || gotPath != "movie.mp4" {
		t.Fatalf("got (%q, %q)", gotRoot, gotPath)
	}
}

func TestBrowseMissingFolder(t *testing.T) {
	root := t.TempDir()
	_, err := filesystem.Browse("main", "Videos", root, "does-not-exist")
	if !errors.Is(err, filesystem.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestBrowseUnknownRootPath(t *testing.T) {
	_, err := filesystem.Browse("main", "Videos", "/definitely/does/not/exist", "")
	if !errors.Is(err, filesystem.ErrRootInaccessible) {
		t.Fatalf("got %v, want ErrRootInaccessible", err)
	}
}
