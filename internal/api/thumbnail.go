package api

import (
	"net/http"
	"os"
)

// handleThumbnail serves a video's cached thumbnail, generating it first
// if necessary (bounded wait — see internal/thumbnail.Manager.Request).
// Cache-Control is aggressive because the URL's underlying file is named
// from a content hash: if the video changes, a fresh request naturally
// gets a different on-disk file once regenerated, so a stale cached image
// under the old name never gets served as if it were current.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Thumbnails are not available.")
		return
	}

	root, relPath, absPath, ok := s.resolveVideoID(w, id)
	if !ok {
		return
	}

	result, err := s.previews.Request(r.Context(), id, root.ID, relPath, absPath, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not generate the thumbnail.")
		return
	}
	if result.Failed {
		writeError(w, http.StatusNotFound, "not_found", "No thumbnail could be generated for this video.")
		return
	}
	if !result.Ready {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The thumbnail is still being generated.")
		return
	}

	f, err := os.Open(result.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No thumbnail could be generated for this video.")
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/webp")
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read the thumbnail.")
		return
	}
	http.ServeContent(w, r, "thumbnail.webp", info.ModTime(), f)
}

func (s *Server) handleThumbnailStatus(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Thumbnails are not available.")
		return
	}
	status, err := s.previews.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read thumbnail status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleThumbnailPregenerate(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Thumbnails are not available.")
		return
	}
	started, err := s.previews.Pregenerate(s.appCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not start pre-generation.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": started})
}
