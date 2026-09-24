// Package settings persists onboarding/auth/per-root-display state to
// DATA_DIR/config.json — deliberately separate from the disposable
// index.db: deleting the media index to force a rescan must never also
// erase login credentials or server setup.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"playhead/internal/config"
)

// ErrNotConfigured is returned by Get/require-configured callers when no
// config.json exists yet — the real, expected state before first-run
// setup completes.
var ErrNotConfigured = errors.New("not configured")

// RootOverride is a per-root display customization. A root with no entry
// here just uses its discovered folder name and isn't hidden.
type RootOverride struct {
	DisplayName string `json:"displayName,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
}

// Config is the full contents of config.json.
type Config struct {
	ServerName       string                  `json:"serverName"`
	AuthUsername     string                  `json:"authUsername"`
	AuthPasswordHash string                  `json:"authPasswordHash"`
	SessionSecret    string                  `json:"sessionSecret"`
	Roots            map[string]RootOverride `json:"roots,omitempty"`
}

// Store is a mutex-guarded, in-memory-cached view of config.json, safe
// for concurrent use: HTTP handlers may read the current config while a
// settings update is being saved.
type Store struct {
	path string

	mu  sync.RWMutex
	cfg *Config // nil until Load finds a file, or Save is called
}

// NewStore returns a Store for DATA_DIR/config.json. Call Load once at
// startup before serving traffic.
func NewStore(dataDir string) *Store {
	return &Store{path: filepath.Join(dataDir, "config.json")}
}

// Load reads config.json from disk into memory, if it exists. A missing
// file is not an error — it means "not yet configured", which Get
// reports via ErrNotConfigured.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	s.mu.Lock()
	s.cfg = &cfg
	s.mu.Unlock()
	return nil
}

// Get returns the current in-memory config. ok is false if setup hasn't
// been completed yet.
func (s *Store) Get() (cfg Config, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg == nil {
		return Config{}, false
	}
	return *s.cfg, true
}

// Configured reports whether setup has been completed.
func (s *Store) Configured() bool {
	_, ok := s.Get()
	return ok
}

// Save writes cfg to config.json atomically (temp file + rename, same
// pattern internal/preview uses for cache files, so a concurrent reader —
// including a process crash mid-write — never sees a partial file), then
// updates the in-memory copy.
func (s *Store) Save(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}

	s.mu.Lock()
	cfgCopy := cfg
	s.cfg = &cfgCopy
	s.mu.Unlock()
	return nil
}

// MergeRoots applies cfg's per-root overrides to a freshly discovered
// root list: a hidden root is dropped entirely, a display-name override
// replaces the folder-name default, and a root with no override at all
// passes through unchanged. Order follows discovered (already
// alphabetical, from config.DiscoverRoots' os.ReadDir).
func MergeRoots(discovered []config.Root, cfg Config) []config.Root {
	merged := make([]config.Root, 0, len(discovered))
	for _, r := range discovered {
		override, ok := cfg.Roots[r.ID]
		if ok {
			if override.Hidden {
				continue
			}
			if override.DisplayName != "" {
				r.Name = override.DisplayName
			}
		}
		merged = append(merged, r)
	}
	return merged
}
