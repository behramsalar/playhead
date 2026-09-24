package database

import (
	"context"
	"path"
	"strings"
	"time"
)

// SearchVideoRow is one filename-search match, carrying its root alongside
// FlatVideoRow's fields since a search (unlike QueryVideosUnderPath) can
// span multiple configured roots.
type SearchVideoRow struct {
	RootID string
	FlatVideoRow
}

// searchResultCap bounds how many rows SearchVideosByName returns, a
// defensive limit rather than a documented feature — filename search is
// explicitly low-priority/simple, not a paginated search engine.
const searchResultCap = 500

// SearchVideosByName returns indexed videos whose filename (not folder
// names elsewhere in the path) contains query as a case-insensitive
// substring. rootID scopes the search to one root; empty searches every
// configured root. An unindexed file simply doesn't appear yet — this
// reads the index, same "absent until indexed" caveat as the flattened
// view.
func (s *Store) SearchVideosByName(ctx context.Context, rootID, query string) ([]SearchVideoRow, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	pattern := `%` + escapeLike(query) + `%`
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, root_id, rel_path, size, mtime_unix, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix
		FROM videos
		WHERE (? = '' OR root_id = ?) AND rel_path LIKE ? ESCAPE '\'
		LIMIT ?`, rootID, rootID, pattern, searchResultCap*4) // wider SQL prefilter; the real match is the filename-only check below
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(query)
	var result []SearchVideoRow
	for rows.Next() {
		var row SearchVideoRow
		var mtimeUnix int64
		if err := rows.Scan(&row.ID, &row.RootID, &row.RelPath, &row.Size, &mtimeUnix,
			&row.Metadata.DurationSeconds, &row.Metadata.Width, &row.Metadata.Height,
			&row.Metadata.Container, &row.Metadata.VideoCodec, &row.Metadata.AudioCodec,
			&row.Metadata.Status, &row.Metadata.ProbeError, &row.Metadata.FirstIndexedAtUnix); err != nil {
			return nil, err
		}
		if !strings.Contains(strings.ToLower(path.Base(row.RelPath)), lowerQuery) {
			continue
		}
		row.ModTime = time.Unix(mtimeUnix, 0)
		result = append(result, row)
		if len(result) >= searchResultCap {
			break
		}
	}
	return result, rows.Err()
}
