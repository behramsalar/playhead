package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type tagsListResponse struct {
	Tags []tagWithCountDTO `json:"tags"`
}

type tagWithCountDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

// handleListTags lists every tag (alphabetically) with how many videos
// currently carry it — the filter-by-tag row, which shows every tag
// regardless of whether the current listing has any videos wearing it.
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, tagsListResponse{Tags: []tagWithCountDTO{}})
		return
	}

	tags, err := s.store.ListTagsWithCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not list tags.")
		return
	}
	out := make([]tagWithCountDTO, len(tags))
	for i, t := range tags {
		out[i] = tagWithCountDTO{ID: t.ID, Name: t.Name, Color: t.Color, Count: t.Count}
	}
	writeJSON(w, http.StatusOK, tagsListResponse{Tags: out})
}

type addTagRequest struct {
	Name string `json:"name"`
}

// handleAddTagToVideo tags one video, creating the tag first if this is
// its first use (freeform tagging — no separate "create a tag" step).
func (s *Server) handleAddTagToVideo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Tagging is not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}

	var req addTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Tag name cannot be empty.")
		return
	}

	tag, err := s.store.CreateOrGetTag(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not create tag.")
		return
	}
	if err := s.store.TagVideo(r.Context(), id, tag.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not tag video.")
		return
	}
	writeJSON(w, http.StatusOK, tagDTO{ID: tag.ID, Name: tag.Name, Color: tag.Color})
}

// handleRemoveTagFromVideo removes one tag from one video (the tag
// itself, and any other video wearing it, is untouched).
func (s *Server) handleRemoveTagFromVideo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Tagging is not available.")
		return
	}
	if _, _, _, ok := s.resolveVideoID(w, id); !ok {
		return
	}

	tagID, err := strconv.ParseInt(r.PathValue("tagId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid tag ID.")
		return
	}

	if err := s.store.UntagVideo(r.Context(), id, tagID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not remove tag.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type bulkTagRequest struct {
	VideoIDs []string `json:"videoIds"`
	Name     string   `json:"name"`
}

// handleBulkTagVideos applies one tag to many videos at once (the
// select-several-cards-and-tag-them-all flow), creating the tag first if
// needed. Video IDs aren't individually validated against a real file the
// way single-video tagging is (resolveVideoID) — tagging an ID that turns
// out to be stale is harmless (an orphaned video_tags row, cleaned up
// naturally since nothing ever reads tags for a video that doesn't
// resolve), and validating potentially hundreds of IDs synchronously
// isn't worth the cost for what's fundamentally a convenience action.
func (s *Server) handleBulkTagVideos(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Tagging is not available.")
		return
	}

	var req bulkTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Tag name cannot be empty.")
		return
	}
	if len(req.VideoIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "videoIds cannot be empty.")
		return
	}

	tag, err := s.store.CreateOrGetTag(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not create tag.")
		return
	}
	if err := s.store.TagVideos(r.Context(), req.VideoIDs, tag.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not tag videos.")
		return
	}
	writeJSON(w, http.StatusOK, tagDTO{ID: tag.ID, Name: tag.Name, Color: tag.Color})
}

// handleDeleteTag removes a tag entirely, untagging every video that
// carried it (ON DELETE CASCADE on video_tags.tag_id) — a "delete this
// tag from my whole library" action, not just from one video.
func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Tagging is not available.")
		return
	}
	tagID, err := strconv.ParseInt(r.PathValue("tagId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid tag ID.")
		return
	}
	if err := s.store.DeleteTag(r.Context(), tagID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not delete tag.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
