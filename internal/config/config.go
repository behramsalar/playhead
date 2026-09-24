// Package config loads server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"playhead/internal/filesystem"
)

// Root describes one configured media root.
type Root struct {
	ID   string // stable public identifier, never a filesystem path
	Name string // display name
	Path string // absolute filesystem path, never exposed to clients
	// HiddenChildren are immediate-child names to exclude when browsing
	// this root's own top level (see filesystem.Browse's hiddenTopLevel
	// parameter). Only ever set on the synthetic "main" root DiscoverRoots
	// creates when some of MEDIA_ROOT's subdirectories were promoted into
	// their own separate roots — without this, a promoted subdirectory's
	// files would be reachable under two different video IDs (once via
	// its own root, once via browsing into it from "main").
	HiddenChildren []string
}

// Config holds the full server configuration.
type Config struct {
	Addr    string
	DataDir string
	// MediaRoot is the parent directory whose immediate subdirectories
	// are auto-discovered as roots (Phase 7) — see DiscoverRoots. Roots
	// itself starts empty; the caller discovers and sets it after Load.
	MediaRoot string
	Roots     []Root

	// ExtraHiddenNames are additional entry names (case-insensitive) to
	// hide from browsing/indexing, on top of the built-in NAS/OS clutter
	// list — kept easy to extend by design.
	ExtraHiddenNames []string
	// ExcludedPaths are relative path prefixes (forward-slash separated,
	// matching filesystem.CleanRelPath's output) to exclude entirely from
	// browsing/indexing/search — e.g. a raw-footage folder never worth
	// previewing. A path is excluded if it equals a prefix or is nested
	// under one.
	ExcludedPaths []string
	// ExcludedExtensions are video file extensions (with leading dot,
	// case-insensitive) to exclude from browsing/indexing/search even
	// though they're otherwise playable containers — e.g. a deployment
	// that never wants .avi files surfaced.
	ExcludedExtensions []string

	// CacheMaxBytes optionally caps DATA_DIR/cache/'s total size; 0 means
	// unlimited. Enforcement evicts the least-recently-generated files
	// first (see internal/preview's cache.go).
	CacheMaxBytes int64
}

// Load reads configuration from environment variables and validates it.
//
// Phase 7: MEDIA_ROOT is now the *parent* directory of one or more roots,
// auto-discovered via DiscoverRoots — Load itself only validates that the
// parent exists and is a readable directory; it doesn't populate Roots
// (that needs settings.Store's per-root overrides too, so it happens in
// main.go after Load returns).
func Load() (*Config, error) {
	addr := getEnv("APP_ADDR", ":8080")
	dataDir := getEnv("DATA_DIR", "/data")

	mediaRoot := os.Getenv("MEDIA_ROOT")
	if mediaRoot == "" {
		return nil, fmt.Errorf("MEDIA_ROOT is required")
	}
	if !isAbs(mediaRoot) {
		return nil, fmt.Errorf("MEDIA_ROOT must be an absolute path")
	}
	if info, err := os.Stat(mediaRoot); err != nil {
		return nil, fmt.Errorf("MEDIA_ROOT %q is not accessible: %w", mediaRoot, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("MEDIA_ROOT %q is not a directory", mediaRoot)
	}

	var cacheMaxBytes int64
	if raw := os.Getenv("CACHE_MAX_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("CACHE_MAX_BYTES must be a non-negative integer, got %q", raw)
		}
		cacheMaxBytes = parsed
	}

	cfg := &Config{
		Addr:               addr,
		DataDir:            dataDir,
		MediaRoot:          mediaRoot,
		ExtraHiddenNames:   splitList(os.Getenv("EXTRA_HIDDEN_NAMES")),
		ExcludedPaths:      splitList(os.Getenv("EXCLUDED_PATHS")),
		ExcludedExtensions: splitList(os.Getenv("EXCLUDED_EXTENSIONS")),
		CacheMaxBytes:      cacheMaxBytes,
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating DATA_DIR %q: %w", dataDir, err)
	}

	return cfg, nil
}

// mainRootID is the fixed, stable ID for MEDIA_ROOT's own content when
// it isn't split out into separate per-mount roots — see DiscoverRoots.
const mainRootID = "main"

// mainRootDefaultName is "main"'s default display name — deliberately
// not the raw parent-directory basename (often something generic and
// unhelpful like "media" or "data"), matching Phase 1-6's own default
// for a single unnamed root. Renamable from onboarding/settings either
// way.
const mainRootDefaultName = "Videos"

// DiscoverRoots inspects parentDir (MEDIA_ROOT) and decides, per
// immediate subdirectory, whether it represents a genuinely separate
// filesystem mount (a distinct device ID — e.g. a Docker volume bind-
// mounted at MEDIA_ROOT/Movies) or just an organizational subfolder
// sharing parentDir's own mount. A distinct mount is "promoted": it
// becomes its own root, ID and default display name equal to the
// folder's own name. Everything else — plain subfolders, loose files
// directly in parentDir, and (if device detection isn't available at
// all) every subdirectory — stays together as a single "main" root:
// parentDir itself, browsed as one folder tree, exactly like Phases 1-6.
// "main" only exists when there's something left for it to show; if
// every subdirectory was promoted and parentDir has no loose files, it's
// omitted entirely.
//
// This distinction matters: auto-splitting every subdirectory into its
// own top-level library (the original design) is wrong for the common
// case of one mounted directory containing organizational subfolders
// (e.g. a single `-v host:/media` with `Anime/`, `Misc/`, `TV/` inside
// it) — those aren't separate libraries, just folders, and splitting
// them fragments browsing/search across disconnected roots for no
// reason. It's only correct when MEDIA_ROOT's subdirectories really are
// separate Docker volumes (`-v host1:/media/Movies -v host2:/media/TV`),
// which the mount-boundary check distinguishes automatically, with no
// configuration either way.
//
// Hidden entries (dotfiles, NAS/OS clutter) are excluded either way. A
// symlink is followed (os.Stat, not the non-dir-aware DirEntry.IsDir) so
// a symlinked mount still counts; anything that fails to stat (broken
// symlink, permissions) is silently skipped rather than failing the
// whole discovery — a root that isn't currently accessible should just
// not appear, not break every other root.
func DiscoverRoots(parentDir string) ([]Root, error) {
	entries, err := os.ReadDir(parentDir) // already sorted by name
	if err != nil {
		return nil, err
	}

	parentDev, parentDevOK := deviceIDFunc(parentDir)

	var promoted []Root
	hasMainContent := false

	for _, e := range entries {
		name := e.Name()
		if filesystem.IsHidden(name) {
			continue
		}
		fullPath := filepath.Join(parentDir, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			hasMainContent = true // a loose file directly in MEDIA_ROOT
			continue
		}

		if parentDevOK {
			if dev, ok := deviceIDFunc(fullPath); ok && dev != parentDev {
				promoted = append(promoted, Root{ID: name, Name: name, Path: fullPath})
				continue
			}
		}
		// An empty same-device subdirectory contributes nothing
		// browsable, so it shouldn't single-handedly force a "main" root
		// into existence — this matters in practice, not just in theory:
		// Alpine's base image (this app's own Docker image) ships
		// pre-existing empty /media/cdrom, /media/floppy, /media/usb
		// directories (a standard FHS removable-media convention), and
		// /media is exactly the path this project's own docs recommend
		// mounting media at.
		if dirHasEntries(fullPath) {
			hasMainContent = true
		}
	}

	if !hasMainContent {
		return promoted, nil
	}

	hiddenChildren := make([]string, len(promoted))
	for i, r := range promoted {
		hiddenChildren[i] = r.ID
	}
	main := Root{ID: mainRootID, Name: mainRootDefaultName, Path: parentDir, HiddenChildren: hiddenChildren}
	return append([]Root{main}, promoted...), nil
}

// dirHasEntries reports whether path contains at least one entry (a
// cheap, shallow check — a single Readdirnames(1) call, not a recursive
// walk). An unreadable directory conservatively counts as "has entries"
// so a permissions problem never silently hides real content.
func dirHasEntries(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if err != nil {
		return len(names) > 0
	}
	return true
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func isAbs(p string) bool {
	return len(p) > 0 && p[0] == '/'
}

// splitList parses a comma-separated env var into trimmed, non-empty
// entries. An unset or empty var yields nil.
func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
