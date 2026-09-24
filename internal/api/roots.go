package api

import (
	"log/slog"

	"playhead/internal/config"
	"playhead/internal/settings"
)

func (s *Server) rootByID(id string) (config.Root, bool) {
	s.rootsMu.RLock()
	defer s.rootsMu.RUnlock()
	r, ok := s.roots[id]
	return r, ok
}

// allRoots returns roots in the same order they were last set (which
// follows config.DiscoverRoots' sorted os.ReadDir order) — not map
// iteration order, which Go randomizes per-process and would otherwise
// make the home-redirect target and the root-switcher's order change on
// every single page load for no reason.
func (s *Server) allRoots() []config.Root {
	s.rootsMu.RLock()
	defer s.rootsMu.RUnlock()
	out := make([]config.Root, len(s.rootsOrdered))
	copy(out, s.rootsOrdered)
	return out
}

func (s *Server) setRoots(roots []config.Root) {
	byID := make(map[string]config.Root, len(roots))
	ordered := make([]config.Root, len(roots))
	copy(ordered, roots)
	for _, r := range roots {
		byID[r.ID] = r
	}
	s.rootsMu.Lock()
	s.roots = byID
	s.rootsOrdered = ordered
	s.rootsMu.Unlock()
}

// refreshRoots re-discovers MEDIA_ROOT's immediate subdirectories,
// applies the current settings overrides (display names, hidden), and
// pushes the merged list to every component that resolves a root by ID —
// this server's own map, the indexer, and the preview manager (Phase 7:
// "must not require a container restart to pick up a newly mounted
// drive/folder"). Safe to call with s.settings unconfigured (nil
// override map, so discovered roots pass through using their folder
// names) — used at startup before setup has ever run, so the empty-state
// "no libraries found yet" UI still has an accurate (possibly empty)
// root list to show.
func (s *Server) refreshRoots() ([]config.Root, error) {
	discovered, err := config.DiscoverRoots(s.mediaRootParent)
	if err != nil {
		return nil, err
	}

	var cfg settings.Config
	if s.settings != nil {
		cfg, _ = s.settings.Get() // ok=false (not configured yet) just means no overrides
	}
	merged := settings.MergeRoots(discovered, cfg)

	s.setRoots(merged)
	if s.indexer != nil {
		s.indexer.SetRoots(merged)
	}
	if s.previews != nil {
		s.previews.SetRoots(merged)
	}
	slog.Info("roots refreshed", "count", len(merged))
	return merged, nil
}
