package database

import (
	"context"
	"time"
)

// RecentVideoRow is one match from ListRecentlyAdded, carrying its root
// alongside FlatVideoRow's fields since "recently added" spans every
// configured root (like SearchVideoRow does for search).
type RecentVideoRow struct {
	RootID string
	FlatVideoRow
}

// ListRecentlyAdded returns the most recently first-indexed videos,
// newest first — the "Recently Added" smart view reads the index, so a
// video not yet indexed simply doesn't appear until a scan reaches it.
// rootID scopes to one root; empty spans every
// configured root. limit bounds the result; callers that also filter by
// tag afterwards should ask for a generously larger limit than they
// intend to display, since that filtering happens after this query.
func (s *Store) ListRecentlyAdded(ctx context.Context, rootID string, limit int) ([]RecentVideoRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, root_id, rel_path, size, mtime_unix, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix
		FROM videos
		WHERE (? = '' OR root_id = ?) AND first_indexed_at_unix > 0
		ORDER BY first_indexed_at_unix DESC
		LIMIT ?
	`, rootID, rootID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RecentVideoRow
	for rows.Next() {
		var row RecentVideoRow
		var mtimeUnix int64
		if err := rows.Scan(&row.ID, &row.RootID, &row.RelPath, &row.Size, &mtimeUnix,
			&row.Metadata.DurationSeconds, &row.Metadata.Width, &row.Metadata.Height,
			&row.Metadata.Container, &row.Metadata.VideoCodec, &row.Metadata.AudioCodec,
			&row.Metadata.Status, &row.Metadata.ProbeError, &row.Metadata.FirstIndexedAtUnix); err != nil {
			return nil, err
		}
		row.ModTime = time.Unix(mtimeUnix, 0)
		result = append(result, row)
	}
	return result, rows.Err()
}
