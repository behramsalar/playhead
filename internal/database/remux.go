package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---- Remux --------------------------------------------------------------
//
// Mirrors the thumbnail methods in store.go exactly (see their comments for
// the reasoning): bookkeeping upserts a minimal row when the indexer hasn't
// probed the file yet, a permanent failure isn't retried until the file
// changes, and a full-index status query drives the coverage summary.
// ThumbTarget is reused here (RootID/RelPath is all a caller needs to
// resolve the source file, regardless of which asset it's generating).

// GetRemuxStatus reports a video's remux status. ok is false if the video
// has no index row yet, which callers should treat as "pending".
func (s *Store) GetRemuxStatus(ctx context.Context, id string) (status string, remuxErr *string, ok bool, err error) {
	row := s.db.QueryRowContext(ctx, `SELECT remux_status, remux_error FROM videos WHERE id = ?`, id)
	err = row.Scan(&status, &remuxErr)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	return status, remuxErr, true, nil
}

// SetRemuxOK records a successful remux, upserting a minimal row if none
// exists yet (see SetThumbOK).
func (s *Store) SetRemuxOK(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, indexed_at_unix, remux_status, remux_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, 0, 'ok', NULL, ?)
		ON CONFLICT(id) DO UPDATE SET remux_status = 'ok', remux_error = NULL
	`, id, rootID, relPath, size, mtimeUnix, time.Now().Unix())
	return err
}

// SetRemuxError records a failed remux attempt.
func (s *Store) SetRemuxError(ctx context.Context, id, rootID, relPath string, size, mtimeUnix int64, remuxErr string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO videos (id, root_id, rel_path, size, mtime_unix, indexed_at_unix, remux_status, remux_error, first_indexed_at_unix)
		VALUES (?, ?, ?, ?, ?, 0, 'error', ?, ?)
		ON CONFLICT(id) DO UPDATE SET remux_status = 'error', remux_error = excluded.remux_error
	`, id, rootID, relPath, size, mtimeUnix, remuxErr, time.Now().Unix())
	return err
}

// FilterNeedingRemux returns the subset of ids that haven't successfully
// remuxed and haven't already failed.
func (s *Store) FilterNeedingRemux(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, remux_status FROM videos WHERE id IN (%s)`, placeholders), args...)
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

// GetRemuxCounts aggregates remux status across every indexed video, for
// the remux-status endpoint.
func (s *Store) GetRemuxCounts(ctx context.Context) (ok, errored, pending int, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT remux_status, COUNT(*) FROM videos GROUP BY remux_status`)
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

// ListPendingRemux returns every indexed video across all roots that
// hasn't successfully remuxed and hasn't already failed — the source list
// for a full pre-generation pass.
func (s *Store) ListPendingRemux(ctx context.Context) ([]ThumbTarget, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, root_id, rel_path FROM videos WHERE remux_status = 'pending'`)
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
