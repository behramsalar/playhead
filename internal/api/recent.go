package api

import (
	"log/slog"
	"net/http"
	"path"
	"strconv"

	"playhead/internal/database"
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
	// Spanning every root (rootID == "") can surface rows belonging to a
	// currently-hidden root: hiding one drops it from s.roots (see
	// refreshRoots/settings.MergeRoots) so it stops being scanned, but
	// doesn't touch its already-indexed rows — those stay in the
	// database, ready to reappear instantly if the root is unhidden
	// later, rather than needing a full rescan. A single-root request
	// doesn't need this check: an unknown/hidden root already 404'd via
	// rootByID above, so every row for it is inherently fine. Widening
	// the fetch (like the existing tag-filter case) keeps the returned
	// count close to what was asked for even after filtering.
	fetchLimit := limit
	if rootID == "" || len(tagIDs) > 0 {
		fetchLimit = limit * recentCandidateMultiplier
	}

	rows, err := s.store.ListRecentlyAdded(r.Context(), rootID, fetchLimit)
	if err != nil {
		slog.Error("listing recently added failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not list recently added videos.")
		return
	}
	if rootID == "" {
		rows = filterRecentRowsToVisibleRoots(rows, s)
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

// filterRecentRowsToVisibleRoots drops rows belonging to a root that
// isn't currently in s.roots — most notably a hidden root (see the call
// site's comment in handleRecent). rootByID is the same live-root check
// every other endpoint already resolves a root through; this just
// applies it per-row instead of once to a single requested root.
func filterRecentRowsToVisibleRoots(rows []database.RecentVideoRow, s *Server) []database.RecentVideoRow {
	kept := make([]database.RecentVideoRow, 0, len(rows))
	for _, row := range rows {
		if _, ok := s.rootByID(row.RootID); ok {
			kept = append(kept, row)
		}
	}
	return kept
}
