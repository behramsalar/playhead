-- Tracks how many times each preview asset type has been automatically
-- requeued after a failure (internal/preview's post-scan retry sweep),
-- so a permanently broken file (genuinely corrupt, unsupported codec)
-- eventually stops being retried instead of consuming worker slots on
-- every single scan forever. A manual per-video retry, a full rebuild,
-- or the file itself changing all reset the relevant counter back to 0
-- — see internal/database/store.go's Reset*Status/Upsert* methods.
ALTER TABLE videos ADD COLUMN thumb_retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN sprite_retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN remux_retry_count INTEGER NOT NULL DEFAULT 0;
