-- Tags are user data (like resume_positions), not part of the disposable
-- media index/cache: video_tags.video_id deliberately has no foreign key
-- to videos, so tagging is never touched by DeleteStale or a rescan.
-- tags.id is stable for the tag's lifetime, so video_tags.tag_id safely
-- cascades on tag deletion.
CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE COLLATE NOCASE,
    color TEXT NOT NULL,
    created_at_unix INTEGER NOT NULL
);

CREATE TABLE video_tags (
    video_id TEXT NOT NULL,
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    tagged_at_unix INTEGER NOT NULL,
    PRIMARY KEY (video_id, tag_id)
);

CREATE INDEX idx_video_tags_video_id ON video_tags(video_id);
CREATE INDEX idx_video_tags_tag_id ON video_tags(tag_id);
