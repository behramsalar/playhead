package database

import (
	"context"
	"database/sql"
	"time"
)

// GetResumePosition returns a video's saved playback position, if any. ok
// is false when nothing has been saved (including for an unknown video
// ID) — the caller should just start playback from the beginning.
func (s *Store) GetResumePosition(ctx context.Context, videoID string) (positionSeconds float64, ok bool, err error) {
	row := s.db.QueryRowContext(ctx, `SELECT position_seconds FROM resume_positions WHERE video_id = ?`, videoID)
	err = row.Scan(&positionSeconds)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return positionSeconds, true, nil
}

// SetResumePosition saves (or updates) a video's playback position.
func (s *Store) SetResumePosition(ctx context.Context, videoID string, positionSeconds float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO resume_positions (video_id, position_seconds, updated_at_unix)
		VALUES (?, ?, ?)
		ON CONFLICT(video_id) DO UPDATE SET position_seconds = excluded.position_seconds, updated_at_unix = excluded.updated_at_unix
	`, videoID, positionSeconds, time.Now().Unix())
	return err
}

// ClearResumePosition removes a video's saved position — called once
// playback reaches the end, so a rewatch starts from the top.
func (s *Store) ClearResumePosition(ctx context.Context, videoID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM resume_positions WHERE video_id = ?`, videoID)
	return err
}
