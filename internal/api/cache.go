package api

import "net/http"

// handleCacheStatus reports current preview-cache disk usage against the
// configured limit (CACHE_MAX_BYTES), if any.
func (s *Server) handleCacheStatus(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	status, err := s.previews.CacheStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read cache status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleEnforceCacheLimit manually re-runs cache-size-limit eviction —
// otherwise it only runs once at startup, so this is how a lowered
// CACHE_MAX_BYTES (or a cache that's grown since) gets enforced without
// restarting the server.
func (s *Server) handleEnforceCacheLimit(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	removedFiles, removedBytes, err := s.previews.EnforceCacheLimit(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Cache limit enforcement failed.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"filesRemoved": int64(removedFiles), "bytesRemoved": removedBytes})
}

// handleCleanOrphans removes cache files that no longer correspond to any
// current video row (left behind when a file changes, is removed, or is
// renamed) — the manual counterpart to the automatic post-scan cleanup.
func (s *Server) handleCleanOrphans(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	removed, err := s.previews.CleanOrphans(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Orphan cleanup failed.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"filesRemoved": removed})
}

// handlePreviewsClear wipes every cached thumbnail/sprite and resets every
// video's generation status to pending — the "clear/rebuild previews"
// control. Regeneration itself then happens the normal way: lazily as
// folders are viewed, or via the existing pregenerate endpoints.
func (s *Server) handlePreviewsClear(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	removed, err := s.previews.ClearAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not clear previews.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"filesRemoved": removed})
}

// handleThumbnailRetry forces one video's thumbnail to be regenerated on
// next view, even if it previously failed permanently.
func (s *Server) handleThumbnailRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Thumbnails are not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}
	if err := s.previews.RetryThumbnail(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not reset thumbnail status.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSpriteRetry is handleThumbnailRetry's sprite equivalent.
func (s *Server) handleSpriteRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Sprites are not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}
	if err := s.previews.RetrySprite(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not reset sprite status.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
