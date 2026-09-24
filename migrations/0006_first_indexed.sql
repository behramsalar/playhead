-- first_indexed_at_unix is set once, the first time a video's row is ever
-- created (by any of the indexer/thumbnail/sprite/remux upsert paths —
-- see internal/database/store.go), and never touched again on any later
-- upsert: it's "when this video first appeared in the app", not "when it
-- was last (re)indexed", which indexed_at_unix already tracks and rewrites
-- on every re-probe. Powers the "recently added" sort/view.
--
-- Existing rows can't know their true first-seen time retroactively, so
-- they're backfilled from indexed_at_unix (their last known indexing
-- time) where available, or "now" for rows that were never actually
-- probed (indexed_at_unix's own 0 sentinel) — better than clustering
-- everything at epoch 0, which would sort as "oldest" instead of
-- "unknown".
ALTER TABLE videos ADD COLUMN first_indexed_at_unix INTEGER NOT NULL DEFAULT 0;

UPDATE videos SET first_indexed_at_unix = CASE
    WHEN indexed_at_unix > 0 THEN indexed_at_unix
    ELSE CAST(strftime('%s', 'now') AS INTEGER)
END;
