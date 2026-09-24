package api

import (
	"time"

	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/preview"
)

// metadataFields is embedded in the API's video DTOs. Pointer fields are
// omitted from the JSON entirely (via omitempty) rather than sent as null
// when a file hasn't been indexed yet or its probe failed — ffprobe
// fields are simply absent until indexed.
type metadataFields struct {
	Duration   *float64 `json:"duration,omitempty"`
	Width      *int     `json:"width,omitempty"`
	Height     *int     `json:"height,omitempty"`
	Container  *string  `json:"container,omitempty"`
	VideoCodec *string  `json:"videoCodec,omitempty"`
	AudioCodec *string  `json:"audioCodec,omitempty"`
	// RemuxEligible marks a file whose codecs are browser-compatible but
	// whose container isn't (Phase 6) — the frontend's compatibility
	// check treats this the same as natively playable, since it'll
	// actually be served as a remuxed MP4, not in its original container.
	RemuxEligible bool `json:"remuxEligible,omitempty"`
	// AddedAt is when this video's index row was first created — by the
	// indexer, or by a thumbnail/sprite/remux job that ran before it, so
	// unlike the probe-derived fields above it's present as soon as a row
	// exists at all, not only once successfully probed (see
	// database.VideoMetadata's FirstIndexedAtUnix).
	AddedAt *time.Time `json:"addedAt,omitempty"`
}

// name is the video's filename (extension only matters for the remux
// eligibility check — see preview.NeedsRemux).
func metadataFieldsFrom(name string, meta database.VideoMetadata, ok bool) metadataFields {
	var fields metadataFields
	if ok && meta.FirstIndexedAtUnix > 0 {
		t := time.Unix(meta.FirstIndexedAtUnix, 0)
		fields.AddedAt = &t
	}
	if !ok || meta.Status != "ok" {
		return fields
	}
	fields.Duration = meta.DurationSeconds
	fields.Width = meta.Width
	fields.Height = meta.Height
	fields.Container = meta.Container
	fields.VideoCodec = meta.VideoCodec
	fields.AudioCodec = meta.AudioCodec
	if meta.VideoCodec != nil && meta.AudioCodec != nil {
		fields.RemuxEligible = preview.NeedsRemux(name, *meta.VideoCodec, *meta.AudioCodec)
	}
	return fields
}

// tagDTO is a tag as attached to a video or listed in the filter row.
type tagDTO struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func tagDTOsFrom(tags []database.Tag) []tagDTO {
	out := make([]tagDTO, len(tags))
	for i, t := range tags {
		out[i] = tagDTO{ID: t.ID, Name: t.Name, Color: t.Color}
	}
	return out
}

type videoEntryDTO struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	// Subfolder is only set in the flattened (recursive) view: the
	// video's folder path relative to the browsed path, so the UI can
	// show it as secondary text. Empty when the video is directly in the
	// browsed folder.
	Subfolder string   `json:"subfolder,omitempty"`
	Tags      []tagDTO `json:"tags,omitempty"`
	metadataFields
}

type browseResponseDTO struct {
	Root        string                   `json:"root"`
	Path        string                   `json:"path"`
	Breadcrumbs []filesystem.Breadcrumb  `json:"breadcrumbs"`
	Folders     []filesystem.FolderEntry `json:"folders"`
	Videos      []videoEntryDTO          `json:"videos"`
	Sort        string                   `json:"sort,omitempty"`
	Flatten     bool                     `json:"flatten,omitempty"`
}
