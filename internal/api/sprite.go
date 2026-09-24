package api

import (
	"net/http"
	"os"
)

// handleSpriteImage serves a video's cached preview sprite sheet,
// generating it first if necessary (bounded wait — see
// internal/preview.Manager.RequestSprite). Same cache/error semantics as
// the thumbnail endpoint.
func (s *Server) handleSpriteImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}

	root, relPath, absPath, ok := s.resolveVideoID(w, id)
	if !ok {
		return
	}

	result, err := s.previews.RequestSprite(r.Context(), id, root.ID, relPath, absPath, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not generate the preview sprite.")
		return
	}
	if result.Failed {
		writeError(w, http.StatusNotFound, "not_found", "No preview sprite could be generated for this video.")
		return
	}
	if !result.Ready {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The preview sprite is still being generated.")
		return
	}

	f, err := os.Open(result.ImagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No preview sprite could be generated for this video.")
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/webp")
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read the preview sprite.")
		return
	}
	http.ServeContent(w, r, "sprite.webp", info.ModTime(), f)
}

// handleSpriteMeta serves the sprite sheet's JSON sidecar (frame
// timestamps and tile coordinates). It shares the same generation and
// wait semantics as the image — a client only needs to fetch one of the
// two first; either request will trigger (and wait for) generation.
func (s *Server) handleSpriteMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}

	root, relPath, absPath, ok := s.resolveVideoID(w, id)
	if !ok {
		return
	}

	result, err := s.previews.RequestSprite(r.Context(), id, root.ID, relPath, absPath, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not generate the preview sprite.")
		return
	}
	if result.Failed {
		writeError(w, http.StatusNotFound, "not_found", "No preview sprite could be generated for this video.")
		return
	}
	if !result.Ready {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The preview sprite is still being generated.")
		return
	}

	f, err := os.Open(result.MetaPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No preview sprite could be generated for this video.")
		return
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "application/json")
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read the preview sprite metadata.")
		return
	}
	http.ServeContent(w, r, "sprite.json", info.ModTime(), f)
}

func (s *Server) handleSpriteStatus(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	status, err := s.previews.SpriteStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read sprite status.")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleSpritePregenerate(w http.ResponseWriter, r *http.Request) {
	if s.previews == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Previews are not available.")
		return
	}
	started, err := s.previews.PregenerateSprites(s.appCtx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not start pre-generation.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int{"queued": started})
}
