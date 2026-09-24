ALTER TABLE videos ADD COLUMN remux_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE videos ADD COLUMN remux_error TEXT;
