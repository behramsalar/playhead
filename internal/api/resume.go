package api

import (
	"encoding/json"
	"net/http"
)

type saveResumeRequest struct {
	PositionSeconds float64 `json:"positionSeconds"`
}

// handleResumeSave saves (or updates) a video's playback position,
// throttled client-side (see Watch.svelte) — this endpoint itself does no
// throttling of its own, it just persists whatever it's given.
func (s *Server) handleResumeSave(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Resume position is not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}

	var req saveResumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}
	if req.PositionSeconds < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "positionSeconds must not be negative.")
		return
	}

	if err := s.store.SetResumePosition(r.Context(), id, req.PositionSeconds); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save resume position.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleResumeClear removes a video's saved position — called once
// playback reaches the end, so a rewatch starts from the top.
func (s *Server) handleResumeClear(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Resume position is not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}

	if err := s.store.ClearResumePosition(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not clear resume position.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
