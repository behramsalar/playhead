package api

import (
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

type searchResultDTO struct {
	ID         string    `json:"id"`
	Root       string    `json:"root"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	FolderPath string    `json:"folderPath"`
	Size       int64     `json:"size"`
	Modified   time.Time `json:"modified"`
	Tags       []tagDTO  `json:"tags,omitempty"`
	metadataFields
}

type searchResponseDTO struct {
	Query   string            `json:"query"`
	Results []searchResultDTO `json:"results"`
}

// handleSearch answers a simple filename substring search against the
// index — deliberately low priority (folder browsing is primary), so no
// fuzzy matching or full-text engine. A blank query or an index that
// isn't wired up both just return no results rather than an error.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rootID := r.URL.Query().Get("root")

	if rootID != "" {
		if _, ok := s.rootByID(rootID); !ok {
			writeError(w, http.StatusNotFound, "not_found", "Unknown root.")
			return
		}
	}

	if s.store == nil || query == "" {
		writeJSON(w, http.StatusOK, searchResponseDTO{Query: query, Results: []searchResultDTO{}})
		return
	}

	rows, err := s.store.SearchVideosByName(r.Context(), rootID, query)
	if err != nil {
		slog.Error("search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Search failed.")
		return
	}

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	tagsByID := s.batchTags(r.Context(), ids)

	results := make([]searchResultDTO, len(rows))
	for i, row := range rows {
		folderPath := path.Dir(row.RelPath)
		if folderPath == "." {
			folderPath = ""
		}
		name := path.Base(row.RelPath)
		results[i] = searchResultDTO{
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
	writeJSON(w, http.StatusOK, searchResponseDTO{Query: query, Results: results})
}
