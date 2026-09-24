package database

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// Tag is a user-created label, freeform (created on first use, not from a
// managed list) and global across every root — see ARCHITECTURE.md.
type Tag struct {
	ID    int64
	Name  string
	Color string
}

// TagWithCount is a Tag plus how many videos currently carry it, for the
// filter-by-tag row.
type TagWithCount struct {
	Tag
	Count int
}

// tagPalette is the small, fixed set of colors a freshly-created tag is
// assigned from — deterministically, by hashing the tag's name, so the
// same name always gets the same color even if the tag is deleted and
// recreated later, without needing a color picker UI.
var tagPalette = []string{
	"#f87171", // red
	"#fb923c", // orange
	"#fbbf24", // amber
	"#a3e635", // lime
	"#34d399", // emerald
	"#22d3ee", // cyan
	"#60a5fa", // blue
	"#a78bfa", // violet
	"#f472b6", // pink
	"#94a3b8", // slate
}

func colorForTagName(name string) string {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return tagPalette[h.Sum32()%uint32(len(tagPalette))]
}

// CreateOrGetTag returns the existing tag with this name (case-insensitive,
// enforced by the tags.name UNIQUE COLLATE NOCASE constraint), or creates
// one with a deterministic color if it doesn't exist yet — the whole point
// of freeform tagging: typing a tag name is enough, no separate "create a
// tag" step.
func (s *Store) CreateOrGetTag(ctx context.Context, name string) (Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Tag{}, fmt.Errorf("tag name cannot be empty")
	}

	color := colorForTagName(name)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tags (name, color, created_at_unix) VALUES (?, ?, ?)
		ON CONFLICT(name) DO NOTHING
	`, name, color, time.Now().Unix())
	if err != nil {
		return Tag{}, err
	}

	var tag Tag
	row := s.db.QueryRowContext(ctx, `SELECT id, name, color FROM tags WHERE name = ? COLLATE NOCASE`, name)
	if err := row.Scan(&tag.ID, &tag.Name, &tag.Color); err != nil {
		return Tag{}, err
	}
	return tag, nil
}

// DeleteTag removes a tag entirely (cascading to every video_tags row that
// referenced it, via the foreign key's ON DELETE CASCADE).
func (s *Store) DeleteTag(ctx context.Context, tagID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, tagID)
	return err
}

// ListTagsWithCounts returns every tag, alphabetically, with how many
// videos currently carry it — for the filter-by-tag row, which needs to
// show all tags regardless of whether the current listing has any videos
// wearing them.
func (s *Store) ListTagsWithCounts(ctx context.Context) ([]TagWithCount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.color, COUNT(vt.video_id)
		FROM tags t
		LEFT JOIN video_tags vt ON vt.tag_id = t.id
		GROUP BY t.id
		ORDER BY t.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TagWithCount
	for rows.Next() {
		var t TagWithCount
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Count); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// TagVideo links a tag to a video. Idempotent — tagging an already-tagged
// video with the same tag is a no-op, not an error.
func (s *Store) TagVideo(ctx context.Context, videoID string, tagID int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO video_tags (video_id, tag_id, tagged_at_unix) VALUES (?, ?, ?)
		ON CONFLICT(video_id, tag_id) DO NOTHING
	`, videoID, tagID, time.Now().Unix())
	return err
}

// UntagVideo removes one tag from one video. A no-op if it wasn't tagged.
func (s *Store) UntagVideo(ctx context.Context, videoID string, tagID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM video_tags WHERE video_id = ? AND tag_id = ?`, videoID, tagID)
	return err
}

// GetVideoTags returns one video's tags, alphabetically — for the watch
// page's tag editor.
func (s *Store) GetVideoTags(ctx context.Context, videoID string) ([]Tag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.color
		FROM tags t
		JOIN video_tags vt ON vt.tag_id = t.id
		WHERE vt.video_id = ?
		ORDER BY t.name COLLATE NOCASE
	`, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// GetTagsForVideos batch-fetches tags for many videos in one query — the
// same shape as GetMetadataBatch, for enriching a folder listing without
// an N+1 query per card. A video with no tags is simply absent from the
// result map.
func (s *Store) GetTagsForVideos(ctx context.Context, videoIDs []string) (map[string][]Tag, error) {
	result := make(map[string][]Tag, len(videoIDs))
	if len(videoIDs) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(videoIDs)), ",")
	args := make([]any, len(videoIDs))
	for i, id := range videoIDs {
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT vt.video_id, t.id, t.name, t.color
		FROM video_tags vt
		JOIN tags t ON t.id = vt.tag_id
		WHERE vt.video_id IN (%s)
		ORDER BY t.name COLLATE NOCASE
	`, placeholders)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var videoID string
		var t Tag
		if err := rows.Scan(&videoID, &t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		result[videoID] = append(result[videoID], t)
	}
	return result, rows.Err()
}

// TagVideos bulk-applies one tag to many videos at once (the "select
// several cards, tag them all" flow) — one statement per video inside a
// single transaction, so a partial failure doesn't leave some videos
// tagged and others not.
func (s *Store) TagVideos(ctx context.Context, videoIDs []string, tagID int64) error {
	if len(videoIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO video_tags (video_id, tag_id, tagged_at_unix) VALUES (?, ?, ?)
		ON CONFLICT(video_id, tag_id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, id := range videoIDs {
		if _, err := stmt.ExecContext(ctx, id, tagID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
