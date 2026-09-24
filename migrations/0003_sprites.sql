ALTER TABLE videos ADD COLUMN sprite_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE videos ADD COLUMN sprite_error TEXT;
