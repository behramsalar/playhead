package filesystem

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"playhead/internal/media"
)

// Breadcrumb is one segment of a folder path, from the root down.
type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FolderEntry is a subfolder within a browsed folder.
type FolderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// VideoEntry is a video file within a browsed folder.
type VideoEntry struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// BrowseResult is the full listing for one folder.
type BrowseResult struct {
	Root        string        `json:"root"`
	Path        string        `json:"path"`
	Breadcrumbs []Breadcrumb  `json:"breadcrumbs"`
	Folders     []FolderEntry `json:"folders"`
	Videos      []VideoEntry  `json:"videos"`
}

// RootAccessible reports whether rootPath currently exists and is a
// readable directory, for the health endpoint.
func RootAccessible(rootPath string) bool {
	info, err := os.Stat(rootPath)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.Open(rootPath)
	if err != nil {
		return false
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	return err == nil || err == io.EOF
}

// Browse lists the folders and videos directly inside relPath under
// rootPath, applying the hidden-entry filter and natural sort order.
// rootID and rootName are used only to build IDs and breadcrumbs.
//
// hiddenTopLevel additionally hides specific entry names, but only at
// the root's own top level (relPath == ""), never deeper — this is how
// config.DiscoverRoots' synthetic "main" root excludes subdirectories
// that were promoted into their own separate roots (a genuinely distinct
// mount point), so the same file is never reachable under two different
// video IDs. Almost every caller passes none.
func Browse(rootID, rootName, rootPath, relPath string, hiddenTopLevel ...string) (*BrowseResult, error) {
	canonicalRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, ErrRootInaccessible
	}

	canonicalPath, err := Resolve(rootPath, relPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(canonicalPath)
	if err != nil {
		return nil, ErrNotFound
	}
	if !info.IsDir() {
		return nil, ErrNotFound
	}

	entries, err := os.ReadDir(canonicalPath)
	if err != nil {
		if os.IsPermission(err) {
			return nil, ErrRootInaccessible
		}
		return nil, err
	}

	cleanedRel, err := CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}

	// Initialized (not nil) so an empty folder serializes as [] rather
	// than null in the JSON response.
	folders := []FolderEntry{}
	videos := []VideoEntry{}

	atTopLevel := cleanedRel == ""

	for _, entry := range entries {
		name := entry.Name()
		if IsHidden(name) {
			continue
		}
		if atTopLevel && containsName(hiddenTopLevel, name) {
			continue
		}

		entryRel := name
		if cleanedRel != "" {
			entryRel = path.Join(cleanedRel, name)
		}
		if IsExcludedPath(entryRel) {
			continue
		}

		isDir, ok := resolveEntryIsDir(canonicalRoot, canonicalPath, name, entry)
		if !ok {
			// Broken symlink, or resolves outside the root: hide it.
			continue
		}

		if isDir {
			folders = append(folders, FolderEntry{Name: name, Path: entryRel})
			continue
		}

		if !media.IsVideoExt(name) || IsExcludedExt(name) {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			continue
		}
		videos = append(videos, VideoEntry{
			ID:       EncodeVideoID(rootID, entryRel),
			Name:     name,
			Path:     entryRel,
			Size:     fi.Size(),
			Modified: fi.ModTime(),
		})
	}

	sort.Slice(folders, func(i, j int) bool { return NaturalLess(folders[i].Name, folders[j].Name) })
	sort.Slice(videos, func(i, j int) bool { return NaturalLess(videos[i].Name, videos[j].Name) })

	return &BrowseResult{
		Root:        rootID,
		Path:        cleanedRel,
		Breadcrumbs: buildBreadcrumbs(rootName, cleanedRel),
		Folders:     folders,
		Videos:      videos,
	}, nil
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// resolveEntryIsDir determines whether a directory entry is itself a
// directory, following symlinks, and confirms the resolved target stays
// within canonicalRoot (the whole media root, not just the current
// folder — a symlink may legitimately point elsewhere inside the root).
// ok is false when the entry is a broken symlink or escapes the root, in
// which case callers should silently omit it from the listing.
func resolveEntryIsDir(canonicalRoot, canonicalParent, name string, entry os.DirEntry) (isDir bool, ok bool) {
	if entry.Type()&os.ModeSymlink == 0 {
		return entry.IsDir(), true
	}

	canonical, existed, err := resolveCanonical(filepath.Join(canonicalParent, name))
	if err != nil || !existed {
		return false, false
	}
	if !isWithin(canonical, canonicalRoot) {
		return false, false
	}
	info, statErr := os.Stat(canonical)
	if statErr != nil {
		return false, false
	}
	return info.IsDir(), true
}

func buildBreadcrumbs(rootName, relPath string) []Breadcrumb {
	crumbs := []Breadcrumb{{Name: rootName, Path: ""}}
	if relPath == "" {
		return crumbs
	}
	acc := ""
	for _, part := range strings.Split(relPath, "/") {
		if acc == "" {
			acc = part
		} else {
			acc = acc + "/" + part
		}
		crumbs = append(crumbs, Breadcrumb{Name: part, Path: acc})
	}
	return crumbs
}
