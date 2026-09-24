package api

import "net/http"

// No handleRemux endpoint exists (unlike thumbnails/sprites) — a remuxed
// file isn't a separate asset a client fetches on its own, it's served
// transparently through the existing GET /api/videos/{id}/stream
// (see handleStream) whenever the video is remux-eligible.

func (s *Server) handleRemuxStatus(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Remuxing is not available.")
		return
	}
	status, err := s.previews.RemuxStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read remux status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRemuxPregenerate(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Remuxing is not available.")
		return
	}
	started, err := s.previews.PregenerateRemux(s.appCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not start pre-generation.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": started})
}

func (s *Server) handleRemuxRetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Remuxing is not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}
	if err := s.previews.RetryRemux(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not reset remux status.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
