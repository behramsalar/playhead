ALTER TABLE videos ADD COLUMN thumb_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE videos ADD COLUMN thumb_error TEXT;
