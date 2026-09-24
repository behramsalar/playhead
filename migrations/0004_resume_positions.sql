-- Resume position is user data (how far you got into a video), not part
-- of the disposable media index/cache: it lives in its own table, with no
-- foreign key to videos, so it is never touched by DeleteStale or a
-- rescan/reindex and survives deleting index.db's other tables' rows.
CREATE TABLE resume_positions (
    video_id TEXT PRIMARY KEY,
    position_seconds REAL NOT NULL,
    updated_at_unix INTEGER NOT NULL
);
