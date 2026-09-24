package api

import (
	"log/slog"
	"net/http"
	"path"
	"strconv"
)

// defaultRecentLimit/maxRecentLimit bound the "Recently Added" smart
// view — like filename search, this is a convenience view, not a
// paginated feed.
const (
	defaultRecentLimit = 60
	maxRecentLimit     = 200
	// recentCandidateMultiplier widens the SQL fetch when a tag filter is
	// active, since filtering happens in Go after the query (see
	// filterByAnyTag) — without this, a tag filter could trim an
	// already-limited result down further than the caller asked for.
	recentCandidateMultiplier = 4
)

type recentResponseDTO struct {
	Videos []searchResultDTO `json:"videos"`
}

// handleRecent answers the "Recently Added" smart view: the most
// recently first-indexed videos, newest first, optionally scoped to one
// root and/or filtered by tag. Uses searchResultDTO's shape (not
// videoEntryDTO's) because — like search, and unlike a normal folder
// browse — results can span every configured root, so each entry needs
// to carry its own root alongside it.
func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, recentResponseDTO{Videos: []searchResultDTO{}})
		return
	}

	rootID := r.URL.Query().Get("root")
	if rootID != "" {
		if _, ok := s.rootByID(rootID); !ok {
			writeError(w, http.StatusNotFound, "not_found", "Unknown root.")
			return
		}
	}
	tagIDs := parseTagIDs(r.URL.Query().Get("tags"))

	limit := defaultRecentLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= maxRecentLimit {
			limit = parsed
		}
	}
	fetchLimit := limit
	if len(tagIDs) > 0 {
		fetchLimit = limit * recentCandidateMultiplier
	}

	rows, err := s.store.ListRecentlyAdded(r.Context(), rootID, fetchLimit)
	if err != nil {
		slog.Error("listing recently added failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not list recently added videos.")
		return
	}

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	tagsByID := s.batchTags(r.Context(), ids)

	videos := make([]searchResultDTO, len(rows))
	for i, row := range rows {
		folderPath := path.Dir(row.RelPath)
		if folderPath == "." {
			folderPath = ""
		}
		name := path.Base(row.RelPath)
		videos[i] = searchResultDTO{
			ID:             row.ID,
			Root:           row.RootID,
			Name:           name,
			Path:           row.RelPath,
			FolderPath:     folderPath,
			Size:           row.Size,
			Modified:       row.ModTime,
			Tags:           tagDTOsFrom(tagsByID[row.ID]),
			metadataFields: metadataFieldsFrom(name, row.Metadata, true),
		}
	}

	if len(tagIDs) > 0 {
		videos = filterSearchResultsByAnyTag(videos, tagIDs)
	}
	if len(videos) > limit {
		videos = videos[:limit]
	}

	writeJSON(w, http.StatusOK, recentResponseDTO{Videos: videos})
}

// filterSearchResultsByAnyTag mirrors filterByAnyTag for searchResultDTO
// (a distinct type from videoEntryDTO, so the two can't share one
// generic without more ceremony than two small functions cost).
func filterSearchResultsByAnyTag(videos []searchResultDTO, tagIDs []int64) []searchResultDTO {
	want := make(map[int64]bool, len(tagIDs))
	for _, id := range tagIDs {
		want[id] = true
	}
	kept := make([]searchResultDTO, 0, len(videos))
	for _, v := range videos {
		for _, t := range v.Tags {
			if want[t.ID] {
				kept = append(kept, v)
				break
			}
		}
	}
	return kept
}
