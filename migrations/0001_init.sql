CREATE TABLE videos (
    id TEXT PRIMARY KEY,
    root_id TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    size INTEGER NOT NULL,
    mtime_unix INTEGER NOT NULL,
    duration_seconds REAL,
    width INTEGER,
    height INTEGER,
    container TEXT,
    video_codec TEXT,
    audio_codec TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    probe_error TEXT,
    indexed_at_unix INTEGER NOT NULL
);

CREATE INDEX idx_videos_root_id ON videos (root_id);
