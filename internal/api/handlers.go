package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/media"
	"playhead/internal/preview"
	"playhead/internal/streaming"
)

// resolveVideoID decodes a video ID, looks up its root, confirms the
// extension is allow-listed, and resolves the canonical on-disk path.
// On any failure it writes the error response itself and returns ok=false.
func (s *Server) resolveVideoID(w http.ResponseWriter, id string) (root config.Root, relPath, absPath string, ok bool) {
	rootID, decodedRelPath, err := filesystem.DecodeVideoID(id)
	if err != nil {
		writeFSError(w, err)
		return
	}

	root, found := s.rootByID(rootID)
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "The requested video was not found.")
		return
	}

	if !media.IsVideoExt(decodedRelPath) {
		writeError(w, http.StatusNotFound, "not_found", "The requested video was not found.")
		return
	}

	absPath, err = filesystem.Resolve(root.Path, decodedRelPath)
	if err != nil {
		writeFSError(w, err)
		return
	}

	return root, decodedRelPath, absPath, true
}

type rootStatus struct {
	ID         string `json:"id"`
	Accessible bool   `json:"accessible"`
}

type healthResponse struct {
	Status string       `json:"status"`
	Roots  []rootStatus `json:"roots"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	roots := s.allRoots()
	statuses := make([]rootStatus, 0, len(roots))
	for _, root := range roots {
		statuses = append(statuses, rootStatus{
			ID:         root.ID,
			Accessible: filesystem.RootAccessible(root.Path),
		})
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", Roots: statuses})
}

type rootInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type rootsResponse struct {
	Roots []rootInfo `json:"roots"`
}

func (s *Server) handleRoots(w http.ResponseWriter, r *http.Request) {
	allRoots := s.allRoots()
	roots := make([]rootInfo, 0, len(allRoots))
	for _, root := range allRoots {
		roots = append(roots, rootInfo{ID: root.ID, Name: root.Name})
	}
	writeJSON(w, http.StatusOK, rootsResponse{Roots: roots})
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	rootID := r.URL.Query().Get("root")
	relPath := r.URL.Query().Get("path")
	sortBy := r.URL.Query().Get("sort")
	flatten := r.URL.Query().Get("flatten") == "true"
	tagIDs := parseTagIDs(r.URL.Query().Get("tags"))

	root, ok := s.rootByID(rootID)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "Unknown root.")
		return
	}

	// Browse validates the path and builds breadcrumbs the same way for
	// both modes; flatten mode just replaces Folders/Videos below.
	result, err := filesystem.Browse(root.ID, root.Name, root.Path, relPath, root.HiddenChildren...)
	if err != nil {
		writeFSError(w, err)
		return
	}

	folders := result.Folders
	var videos []videoEntryDTO

	if flatten {
		folders = []filesystem.FolderEntry{}
		videos, err = s.buildFlattenedVideos(r.Context(), root.ID, result.Path)
		if err != nil {
			slog.Error("flattened listing failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not build the flattened listing.")
			return
		}
	} else {
		videos = s.buildVideoDTOs(r.Context(), result.Videos)
		if s.previews != nil {
			s.previews.EnqueueFolder(s.appCtx, root, result.Videos)
		}
	}

	if len(tagIDs) > 0 {
		videos = filterByAnyTag(videos, tagIDs)
	}

	sortVideos(videos, sortBy)

	writeJSON(w, http.StatusOK, browseResponseDTO{
		Root:        result.Root,
		Path:        result.Path,
		Breadcrumbs: result.Breadcrumbs,
		Folders:     folders,
		Videos:      videos,
		Sort:        sortBy,
		Flatten:     flatten,
	})
}

// parseTagIDs parses a comma-separated "tags" query param into tag IDs,
// silently skipping any entry that isn't a valid integer rather than
// failing the whole request over one malformed filter value.
func parseTagIDs(raw string) []int64 {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// filterByAnyTag keeps only the videos carrying at least one of tagIDs
// (OR semantics) — applied after tags are already attached to each DTO
// (buildVideoDTOs/buildFlattenedVideos), so this is pure in-memory
// filtering, not a second database round trip.
func filterByAnyTag(videos []videoEntryDTO, tagIDs []int64) []videoEntryDTO {
	want := make(map[int64]bool, len(tagIDs))
	for _, id := range tagIDs {
		want[id] = true
	}
	kept := make([]videoEntryDTO, 0, len(videos))
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

// buildVideoDTOs enriches a folder's plain filesystem listing with index
// metadata and tags, each batched in one query.
func (s *Server) buildVideoDTOs(ctx context.Context, videos []filesystem.VideoEntry) []videoEntryDTO {
	metaByID := s.batchMetadata(ctx, videos)
	ids := make([]string, len(videos))
	for i, v := range videos {
		ids[i] = v.ID
	}
	tagsByID := s.batchTags(ctx, ids)

	dtos := make([]videoEntryDTO, len(videos))
	for i, v := range videos {
		meta, ok := metaByID[v.ID]
		dtos[i] = videoEntryDTO{
			ID:             v.ID,
			Name:           v.Name,
			Path:           v.Path,
			Size:           v.Size,
			Modified:       v.Modified,
			Tags:           tagDTOsFrom(tagsByID[v.ID]),
			metadataFields: metadataFieldsFrom(v.Name, meta, ok),
		}
	}
	return dtos
}

// buildFlattenedVideos answers the recursive "all videos under this
// folder" view entirely from the index rather than a filesystem walk. A
// video not yet indexed simply doesn't appear here until a scan reaches
// it — the browse endpoint itself keeps working regardless.
func (s *Server) buildFlattenedVideos(ctx context.Context, rootID, basePath string) ([]videoEntryDTO, error) {
	if s.store == nil {
		return []videoEntryDTO{}, nil
	}

	rows, err := s.store.QueryVideosUnderPath(ctx, rootID, basePath)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	tagsByID := s.batchTags(ctx, ids)

	dtos := make([]videoEntryDTO, len(rows))
	for i, row := range rows {
		name := path.Base(row.RelPath)
		dtos[i] = videoEntryDTO{
			ID:             row.ID,
			Name:           name,
			Path:           row.RelPath,
			Subfolder:      subfolderOf(row.RelPath, basePath),
			Size:           row.Size,
			Modified:       row.ModTime,
			Tags:           tagDTOsFrom(tagsByID[row.ID]),
			metadataFields: metadataFieldsFrom(name, row.Metadata, true),
		}
	}
	return dtos, nil
}

// subfolderOf returns relPath's containing folder, relative to basePath,
// for display as the flattened view's secondary text. Empty when the
// video is directly inside basePath.
func subfolderOf(relPath, basePath string) string {
	dir := path.Dir(relPath)
	if dir == "." {
		dir = ""
	}
	if basePath == "" {
		return dir
	}
	return strings.TrimPrefix(strings.TrimPrefix(dir, basePath), "/")
}

// batchMetadata fetches index metadata for a folder's videos in one query.
// It returns an empty map (every video absent) when no index is wired up.
func (s *Server) batchMetadata(ctx context.Context, videos []filesystem.VideoEntry) map[string]database.VideoMetadata {
	if s.store == nil || len(videos) == 0 {
		return map[string]database.VideoMetadata{}
	}
	ids := make([]string, len(videos))
	for i, v := range videos {
		ids[i] = v.ID
	}
	metaByID, err := s.store.GetMetadataBatch(ctx, ids)
	if err != nil {
		slog.Error("fetching index metadata failed", "error", err)
		return map[string]database.VideoMetadata{}
	}
	return metaByID
}

// batchTags fetches tags for a set of video IDs in one query. It returns
// an empty map (every video untagged) when no index is wired up.
func (s *Server) batchTags(ctx context.Context, ids []string) map[string][]database.Tag {
	if s.store == nil || len(ids) == 0 {
		return map[string][]database.Tag{}
	}
	tagsByID, err := s.store.GetTagsForVideos(ctx, ids)
	if err != nil {
		slog.Error("fetching tags failed", "error", err)
		return map[string][]database.Tag{}
	}
	return tagsByID
}

type videoInfo struct {
	ID            string    `json:"id"`
	Root          string    `json:"root"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	FolderPath    string    `json:"folderPath"`
	Size          int64     `json:"size"`
	Modified      time.Time `json:"modified"`
	ResumeSeconds *float64  `json:"resumeSeconds,omitempty"`
	Tags          []tagDTO  `json:"tags,omitempty"`
	metadataFields
}

func (s *Server) handleVideoInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	root, relPath, absPath, ok := s.resolveVideoID(w, id)
	if !ok {
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "The requested video was not found.")
		return
	}

	folderPath := path.Dir(relPath)
	if folderPath == "." {
		folderPath = ""
	}

	var meta database.VideoMetadata
	var metaOK bool
	var resumeSeconds *float64
	var tags []database.Tag
	if s.store != nil {
		meta, metaOK, err = s.store.GetMetadata(r.Context(), id)
		if err != nil {
			slog.Error("fetching index metadata failed", "id", id, "error", err)
		}
		if pos, ok, err := s.store.GetResumePosition(r.Context(), id); err != nil {
			slog.Error("fetching resume position failed", "id", id, "error", err)
		} else if ok {
			resumeSeconds = &pos
		}
		if tags, err = s.store.GetVideoTags(r.Context(), id); err != nil {
			slog.Error("fetching video tags failed", "id", id, "error", err)
			tags = nil
		}
	}

	writeJSON(w, http.StatusOK, videoInfo{
		ID:             id,
		Root:           root.ID,
		Name:           path.Base(relPath),
		Path:           relPath,
		FolderPath:     folderPath,
		Size:           info.Size(),
		Modified:       info.ModTime(),
		ResumeSeconds:  resumeSeconds,
		Tags:           tagDTOsFrom(tags),
		metadataFields: metadataFieldsFrom(path.Base(relPath), meta, metaOK),
	})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	root, relPath, absPath, ok := s.resolveVideoID(w, id)
	if !ok {
		return
	}
	name := path.Base(relPath)

	// A remux-eligible file is served from its cached, browser-compatible
	// MP4 copy instead of its original bytes — everything else about this
	// handler (range support, error handling) is unchanged, and a
	// non-eligible file never touches this branch at all.
	if s.previews != nil && s.store != nil {
		if meta, ok, err := s.store.GetMetadata(r.Context(), id); err != nil {
			slog.Error("fetching index metadata for stream failed", "id", id, "error", err)
		} else if ok && meta.Status == "ok" && meta.VideoCodec != nil && meta.AudioCodec != nil &&
			preview.NeedsRemux(name, *meta.VideoCodec, *meta.AudioCodec) {
			result, err := s.previews.RequestRemux(r.Context(), id, root.ID, relPath, absPath, true)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Could not prepare this video for playback.")
				return
			}
			if result.Failed {
				writeError(w, http.StatusUnprocessableEntity, "remux_failed", "This file's video couldn't be converted for playback in this browser.")
				return
			}
			if !result.Ready {
				writeError(w, http.StatusServiceUnavailable, "unavailable", "This video is being prepared for playback; try again in a moment.")
				return
			}
			// The served bytes are now MP4, not the original container —
			// ServeFile derives Content-Type from this name's extension, so
			// it must reflect what's actually being sent, not the source
			// file's real name.
			remuxedName := strings.TrimSuffix(name, path.Ext(name)) + ".mp4"
			if err := streaming.ServeFile(w, r, result.Path, remuxedName); err != nil {
				writeError(w, http.StatusNotFound, "not_found", "The requested video was not found.")
			}
			return
		}
	}

	if err := streaming.ServeFile(w, r, absPath, name); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "The requested video was not found.")
		return
	}
}
