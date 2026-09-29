package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"playhead/internal/media"
)

// VideoMetadata is the ffprobe-derived subset of a video's row, as
// consumed by the API layer. The probe-derived fields are nil/empty
// until the file has been successfully probed — ffprobe fields are
// simply absent until indexed. FirstIndexedAtUnix is
// different: it's set the moment any row is created at all (by the
// indexer, or by a thumbnail/sprite/remux job that ran before indexing
// got there), independent of probe success — "when this video first
// appeared", not "was it successfully probed".
type VideoMetadata struct {
	DurationSeconds    *float64
	Width              *int
	Height             *int
	Container          *string
	VideoCodec         *string
	AudioCodec         *string
	Status             string // "ok" or "error"
	ProbeError         *string
	FirstIndexedAtUnix int64
}

// FlatVideoRow is one video returned by QueryVideosUnderPath, carrying
// enough to build both a normal listing entry and the flattened view's
// "subfolder" display field.
type FlatVideoRow struct {
	ID       string
	RelPath  string
	Size     int64
	ModTime  time.Time
	Metadata VideoMetadata
}

// ThumbTarget identifies a video that still needs a thumbnail generated,
// with enough to resolve its on-disk path.
type ThumbTarget struct {
	ID      string
	RootID  string
	RelPath string
}

// Store wraps the video index table.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// NeedsProbe reports whether id is missing from the index, its stored
// size/mtime differ from the current filesystem values, or it has never
// actually been probed — the indexer's signal to (re-)run ffprobe rather
// than trusting the cached row. status == "pending" (the column's default)
// covers a row created by SetThumbOK/SetSpriteOK/SetRemuxOK racing ahead
// of the indexer for a brand-new file: without this check, that row's
// size/mtime already match the real file, so it would look
// already-indexed and be skipped by every future scan even though it was
// never actually probed and has no duration/width/height/codec data. A
// row that *was* probed and failed (status == "error") is deliberately
// excluded from this — see UpsertError — so a permanently broken file
// isn't reprobed on every scan forever.
func (s *Store) NeedsProbe(ctx context.Context, id string, size int64, modTime time.Time) (bool, error) {
	var dbSize int64
	var dbMtime int64
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT size, mtime_unix, status FROM videos WHERE id = ?`, id).Scan(&dbSize, &dbMtime, &status)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return dbSize != size || dbMtime != modTime.Unix() || status == "pending", nil
}

// UpsertOK records a successful probe. The thumbnail/sprite/remux status
// all reset to "pending" because a changed file's old generated assets (if
// any) no longer correspond to its current content.
// first_indexed_at_unix is deliberately absent from the DO UPDATE SET
// clause below (and in every other upsert in this file) — SQLite leaves
// a column untouched on conflict when it isn't listed, so this only ever
// takes effect on the very first INSERT for a given id, no matter which
// upsert path creates the row first. See VideoMetadata's comment.
func (s *Store) UpsertOK(ctx context.Context, id, rootID, relPath string, size int64, modTime time.Time, probe media.ProbeResult) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, indexed_at_unix, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ok', NULL, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			root_id = excluded.root_id,
			rel_path = excluded.rel_path,
			size = excluded.size,
			mtime_unix = excluded.mtime_unix,
			duration_seconds = excluded.duration_seconds,
			width = excluded.width,
			height = excluded.height,
			container = excluded.container,
			video_codec = excluded.video_codec,
			audio_codec = excluded.audio_codec,
			status = 'ok',
			probe_error = NULL,
			indexed_at_unix = excluded.indexed_at_unix,
			thumb_status = 'pending',
			thumb_error = NULL,
			thumb_retry_count = 0,
			sprite_status = 'pending',
			sprite_error = NULL,
			sprite_retry_count = 0,
			remux_status = 'pending',
			remux_error = NULL,
			remux_retry_count = 0
	`, id, rootID, relPath, size, modTime.Unix(),
		probe.DurationSeconds, probe.Width, probe.Height, probe.Container, probe.VideoCodec, probe.AudioCodec,
		now, now)
	return err
}

// UpsertError records a failed probe: the file is still tracked (so it
// shows up in listings and isn't reprobed every scan) but carries no
// metadata. Generation statuses reset for the same reason as UpsertOK.
func (s *Store) UpsertError(ctx context.Context, id, rootID, relPath string, size int64, modTime time.Time, probeErr string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, status, probe_error, indexed_at_unix, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, 'error', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			root_id = excluded.root_id,
			rel_path = excluded.rel_path,
			size = excluded.size,
			mtime_unix = excluded.mtime_unix,
			duration_seconds = NULL,
			width = NULL,
			height = NULL,
			container = NULL,
			video_codec = NULL,
			audio_codec = NULL,
			status = 'error',
			probe_error = excluded.probe_error,
			indexed_at_unix = excluded.indexed_at_unix,
			thumb_status = 'pending',
			thumb_error = NULL,
			thumb_retry_count = 0,
			sprite_status = 'pending',
			sprite_error = NULL,
			sprite_retry_count = 0,
			remux_status = 'pending',
			remux_error = NULL,
			remux_retry_count = 0
	`, id, rootID, relPath, size, modTime.Unix(), probeErr, now, now)
	return err
}

// DeleteStale removes rows for rootID whose id isn't in keepIDs — the
// files that no longer exist under that root (deleted or renamed). It
// returns the number of rows removed.
func (s *Store) DeleteStale(ctx context.Context, rootID string, keepIDs []string) (int64, error) {
	if len(keepIDs) == 0 {
		res, err := s.db.ExecContext(ctx, `DELETE FROM videos WHERE root_id = ?`, rootID)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keepIDs)), ",")
	args := make([]any, 0, len(keepIDs)+1)
	args = append(args, rootID)
	for _, id := range keepIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`DELETE FROM videos WHERE root_id = ? AND id NOT IN (%s)`, placeholders)
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetMetadataBatch fetches metadata for the given IDs in one query, for
// enriching a folder listing. IDs with no row (not yet indexed) are simply
// absent from the result map.
func (s *Store) GetMetadataBatch(ctx context.Context, ids []string) (map[string]VideoMetadata, error) {
	result := make(map[string]VideoMetadata, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT id, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix FROM videos WHERE id IN (%s)`, placeholders)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var meta VideoMetadata
		if err := rows.Scan(&id, &meta.DurationSeconds, &meta.Width, &meta.Height, &meta.Container, &meta.VideoCodec, &meta.AudioCodec, &meta.Status, &meta.ProbeError, &meta.FirstIndexedAtUnix); err != nil {
			return nil, err
		}
		result[id] = meta
	}
	return result, rows.Err()
}

// GetMetadata fetches metadata for a single video ID. ok is false if the
// file hasn't been indexed yet.
func (s *Store) GetMetadata(ctx context.Context, id string) (meta VideoMetadata, ok bool, err error) {
	row := s.db.QueryRowContext(ctx, `SELECT duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix FROM videos WHERE id = ?`, id)
	err = row.Scan(&meta.DurationSeconds, &meta.Width, &meta.Height, &meta.Container, &meta.VideoCodec, &meta.AudioCodec, &meta.Status, &meta.ProbeError, &meta.FirstIndexedAtUnix)
	if err == sql.ErrNoRows {
		return VideoMetadata{}, false, nil
	}
	if err != nil {
		return VideoMetadata{}, false, err
	}
	return meta, true, nil
}

// QueryVideosUnderPath returns every indexed video whose relative path is
// pathPrefix itself or nested under it — the flattened (recursive) view,
// answered entirely from the index rather than a filesystem walk. An empty
// pathPrefix matches every video in rootID.
func (s *Store) QueryVideosUnderPath(ctx context.Context, rootID, pathPrefix string) ([]FlatVideoRow, error) {
	var rows *sql.Rows
	var err error
	if pathPrefix == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, rel_path, size, mtime_unix, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix
			FROM videos WHERE root_id = ?`, rootID)
	} else {
		pattern := escapeLike(pathPrefix) + `/%`
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, rel_path, size, mtime_unix, duration_seconds, width, height, container, video_codec, audio_codec, status, probe_error, first_indexed_at_unix
			FROM videos WHERE root_id = ? AND rel_path LIKE ? ESCAPE '\'`, rootID, pattern)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FlatVideoRow
	for rows.Next() {
		var row FlatVideoRow
		var mtimeUnix int64
		if err := rows.Scan(&row.ID, &row.RelPath, &row.Size, &mtimeUnix,
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

// escapeLike escapes SQLite LIKE wildcards (%, _) and the escape character
// itself in a literal string that will be used with "LIKE ? ESCAPE '\'".
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// GetThumbStatus reports a video's thumbnail generation status. ok is
// false if the video has no index row yet, which callers should treat the
// same as "pending" — thumbnails don't require the file to be probed
// first, only to exist on disk.
func (s *Store) GetThumbStatus(ctx context.Context, id string) (status string, thumbErr *string, ok bool, err error) {
	row := s.db.QueryRowContext(ctx, `SELECT thumb_status, thumb_error FROM videos WHERE id = ?`, id)
	err = row.Scan(&status, &thumbErr)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return status, thumbErr, true, nil
}

// SetThumbOK records a successful thumbnail generation. Thumbnails don't
// require the metadata indexer to have run first, so this upserts a
// minimal row (indexed_at_unix left at 0, a sentinel meaning "not yet
// actually probed", and status left at its default "pending" — see
// NeedsProbe) when one doesn't already exist; the indexer's own upsert
// still runs for this file later (status == "pending" keeps NeedsProbe
// returning true even though size/mtime already match) and fills in the
// rest. durationSeconds is the caller's best-effort probe result (nil if
// unavailable) — recorded now via COALESCE so the duration badge doesn't
// have to wait for that later indexer pass, without ever clobbering an
// already-known value with a missing one. rootID/relPath/size/mtimeUnix
// must reflect the file's current on-disk state (the caller just stat'd
// it to compute the thumbnail's cache filename, so this is free).
func (s *Store) SetThumbOK(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, durationSeconds *float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, duration_seconds, indexed_at_unix, thumb_status, thumb_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'ok', NULL, ?)
		ON CONFLICT(id) DO UPDATE SET thumb_status = 'ok', thumb_error = NULL, duration_seconds = COALESCE(excluded.duration_seconds, duration_seconds)
	`, id, rootID, relPath, size, mtimeUnix, durationSeconds, time.Now().Unix())
	return err
}

// SetThumbError records a failed thumbnail generation attempt. See
// SetThumbOK for why this upserts rather than only updating.
func (s *Store) SetThumbError(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, thumbErr string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, indexed_at_unix, thumb_status, thumb_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, 0, 'error', ?, ?)
		ON CONFLICT(id) DO UPDATE SET thumb_status = 'error', thumb_error = excluded.thumb_error
	`, id, rootID, relPath, size, mtimeUnix, thumbErr, time.Now().Unix())
	return err
}

// FilterNeedingThumbnail returns the subset of ids that haven't
// successfully generated a thumbnail and haven't already failed — IDs with
// no row at all count as needing one, same as GetThumbStatus.
func (s *Store) FilterNeedingThumbnail(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, thumb_status FROM videos WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := make(map[string]string, len(ids))
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		known[id] = status
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var need []string
	for _, id := range ids {
		if status, exists := known[id]; !exists || status == "pending" {
			need = append(need, id)
		}
	}
	return need, nil
}

// GetThumbCounts aggregates thumbnail status across every indexed video,
// for the thumbnail-status endpoint.
func (s *Store) GetThumbCounts(ctx context.Context) (ok, errored, pending int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT thumb_status, COUNT(*) FROM videos GROUP BY thumb_status`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, 0, err
		}
		switch status {
		case "ok":
			ok = count
		case "error":
			errored = count
		default:
			pending += count
		}
	}
	return ok, errored, pending, rows.Err()
}

// ListPendingThumbnails returns every indexed video across all roots that
// hasn't successfully generated a thumbnail and hasn't already failed —
// the source list for a full pre-generation pass.
func (s *Store) ListPendingThumbnails(ctx context.Context) ([]ThumbTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, root_id, rel_path FROM videos WHERE thumb_status = 'pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ThumbTarget
	for rows.Next() {
		var t ThumbTarget
		if err := rows.Scan(&t.ID, &t.RootID, &t.RelPath); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// ---- Sprites ----------------------------------------------------------
//
// Mirrors the thumbnail methods above exactly (see their comments for the
// reasoning): sprite bookkeeping upserts a minimal row when the indexer
// hasn't probed the file yet, a permanent failure isn't retried until the
// file changes, and a full-index status query drives the coverage summary.

// GetSpriteStatus reports a video's sprite generation status. ok is false
// if the video has no index row yet, which callers should treat as
// "pending".
func (s *Store) GetSpriteStatus(ctx context.Context, id string) (status string, spriteErr *string, ok bool, err error) {
	row := s.db.QueryRowContext(ctx, `SELECT sprite_status, sprite_error FROM videos WHERE id = ?`, id)
	err = row.Scan(&status, &spriteErr)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return status, spriteErr, true, nil
}

// SetSpriteOK records a successful sprite generation, upserting a minimal
// row if none exists yet (see SetThumbOK). durationSeconds is the
// caller's best-effort probe result (nil if unavailable) — sprite
// generation already needs an accurate duration to lay out its frames, so
// this records that same value rather than discarding it once the sheet
// is done.
func (s *Store) SetSpriteOK(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, durationSeconds *float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, duration_seconds, indexed_at_unix, sprite_status, sprite_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, ?, 0, 'ok', NULL, ?)
		ON CONFLICT(id) DO UPDATE SET sprite_status = 'ok', sprite_error = NULL, duration_seconds = COALESCE(excluded.duration_seconds, duration_seconds)
	`, id, rootID, relPath, size, mtimeUnix, durationSeconds, time.Now().Unix())
	return err
}

// SetSpriteError records a failed sprite generation attempt.
func (s *Store) SetSpriteError(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, spriteErr string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, indexed_at_unix, sprite_status, sprite_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, 0, 'error', ?, ?)
		ON CONFLICT(id) DO UPDATE SET sprite_status = 'error', sprite_error = excluded.sprite_error
	`, id, rootID, relPath, size, mtimeUnix, spriteErr, time.Now().Unix())
	return err
}

// FilterNeedingSprite returns the subset of ids that haven't successfully
// generated a sprite and haven't already failed.
func (s *Store) FilterNeedingSprite(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, sprite_status FROM videos WHERE id IN (%s)`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	known := make(map[string]string, len(ids))
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		known[id] = status
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var need []string
	for _, id := range ids {
		if status, exists := known[id]; !exists || status == "pending" {
			need = append(need, id)
		}
	}
	return need, nil
}

// ListPendingSprites returns every indexed video across all roots that
// hasn't successfully generated a sprite and hasn't already failed.
func (s *Store) ListPendingSprites(ctx context.Context) ([]ThumbTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, root_id, rel_path FROM videos WHERE sprite_status = 'pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ThumbTarget
	for rows.Next() {
		var t ThumbTarget
		if err := rows.Scan(&t.ID, &t.RootID, &t.RelPath); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

// VideoCacheKey is enough of a video row to recompute its current
// thumbnail/sprite cache hash (see internal/preview's contentHash), for
// orphaned-cache-file detection.
type VideoCacheKey struct {
	ID        string
	Size      int64
	MtimeUnix int64
}

// ListVideoCacheKeys returns every indexed video's cache-hash inputs, for
// computing the set of currently-valid thumbnail/sprite cache filenames.
func (s *Store) ListVideoCacheKeys(ctx context.Context) ([]VideoCacheKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, size, mtime_unix FROM videos`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []VideoCacheKey
	for rows.Next() {
		var k VideoCacheKey
		if err := rows.Scan(&k.ID, &k.Size, &k.MtimeUnix); err != nil {
			return nil, err
		}
		result = append(result, k)
	}
	return result, rows.Err()
}

// ResetAllPreviewStatus resets thumb_status/sprite_status/remux_status to
// 'pending' for every video, for the "clear/rebuild everything" control —
// actual cache file deletion is the caller's (preview.Manager's) job.
func (s *Store) ResetAllPreviewStatus(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE videos SET
			thumb_status = 'pending', thumb_error = NULL, thumb_retry_count = 0,
			sprite_status = 'pending', sprite_error = NULL, sprite_retry_count = 0,
			remux_status = 'pending', remux_error = NULL, remux_retry_count = 0
	`)
	return err
}

// ResetThumbStatus resets one video's thumbnail status to 'pending' and
// its automatic-retry counter back to 0, forcing it to be retried even if
// it previously failed permanently (including exhausting its automatic
// retries) — the per-video "retry" control. It does nothing if the video
// has no row yet.
func (s *Store) ResetThumbStatus(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE videos SET thumb_status = 'pending', thumb_error = NULL, thumb_retry_count = 0 WHERE id = ?`, id)
	return err
}

// ResetSpriteStatus is ResetThumbStatus's sprite equivalent.
func (s *Store) ResetSpriteStatus(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE videos SET sprite_status = 'pending', sprite_error = NULL, sprite_retry_count = 0 WHERE id = ?`, id)
	return err
}

// ResetRemuxStatus is ResetThumbStatus's remux equivalent.
func (s *Store) ResetRemuxStatus(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE videos SET remux_status = 'pending', remux_error = NULL, remux_retry_count = 0 WHERE id = ?`, id)
	return err
}

// RequeueRetryableThumbnails resets every thumbnail still in 'error'
// status, with fewer than maxRetries prior automatic retries, back to
// 'pending' and increments its retry counter — picked up by the next
// Pregenerate/lazy request exactly like any other pending thumbnail. This
// is what turns a transient/environmental failure (e.g. a permissions
// problem during deployment that affects a batch of files) into
// something that self-heals on the next scan, instead of requiring a
// manual per-video retry click for every affected file. A file that has
// already exhausted maxRetries is left alone — still visible via
// GetThumbCounts/the error status, still fixable with an explicit
// ResetThumbStatus, but no longer silently retried forever.
func (s *Store) RequeueRetryableThumbnails(ctx context.Context, maxRetries int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE videos SET thumb_status = 'pending', thumb_error = NULL, thumb_retry_count = thumb_retry_count + 1
		WHERE thumb_status = 'error' AND thumb_retry_count < ?
	`, maxRetries)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RequeueRetryableSprites is RequeueRetryableThumbnails's sprite equivalent.
func (s *Store) RequeueRetryableSprites(ctx context.Context, maxRetries int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE videos SET sprite_status = 'pending', sprite_error = NULL, sprite_retry_count = sprite_retry_count + 1
		WHERE sprite_status = 'error' AND sprite_retry_count < ?
	`, maxRetries)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RequeueRetryableRemux is RequeueRetryableThumbnails's remux equivalent.
func (s *Store) RequeueRetryableRemux(ctx context.Context, maxRetries int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE videos SET remux_status = 'pending', remux_error = NULL, remux_retry_count = remux_retry_count + 1
		WHERE remux_status = 'error' AND remux_retry_count < ?
	`, maxRetries)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetSpriteCounts aggregates sprite status across every indexed video.
func (s *Store) GetSpriteCounts(ctx context.Context) (ok, errored, pending int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sprite_status, COUNT(*) FROM videos GROUP BY sprite_status`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, 0, 0, err
		}
		switch status {
		case "ok":
			ok = count
		case "error":
			errored = count
		default:
			pending += count
		}
	}
	return ok, errored, pending, rows.Err()
}
