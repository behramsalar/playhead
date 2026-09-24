# TASKS.md

Phased checklist. Items are checked only once verified (tests passing and, where relevant, manually exercised).

## Phase 1 — Foundation, browsing, direct playback

- [x] Env-var config (`APP_ADDR`, `MEDIA_ROOT`, `MEDIA_ROOT_NAME`, `DATA_DIR`), modeled as a list of roots.
- [x] Video ID scheme: base64url(`<rootID>/<relativePath>`), encode/decode with full validation.
- [x] Path containment: reject absolute paths, `..` traversal (including nested forms), symlink escapes.
- [x] Hidden-entry filtering (dotfiles + NAS/OS clutter), easy to extend.
- [x] `GET /api/health`
- [x] `GET /api/roots`
- [x] `GET /api/browse` — folders before videos, natural case-insensitive sort, breadcrumbs, correct error statuses (400/403/404/503).
- [x] `GET /api/videos/{id}`
- [x] `GET`/`HEAD /api/videos/{id}/stream` — range support via `http.ServeContent`, correct content types, 404 for non-allow-listed extensions.
- [x] Stable JSON error shape across all endpoints.
- [x] Embedded frontend serving with SPA fallback (`index.html` for unknown non-API routes).
- [x] Svelte SPA: hand-written router, `/browse/:root/*path` and `/watch/:id` routes, `/` redirect.
- [x] Browse page: sticky breadcrumbs, grid view (16:9 placeholder thumbnail, filename, size, relative time), loading/empty/inaccessible/not-found states.
- [x] Player page: `<video controls playsinline preload="metadata">`, back link, clear error message on playback failure.
- [x] Responsive layout verified at ~375px and desktop width; dark mode by default.
- [x] Backend tests: path containment, video ID round-trip/malformed/traversal, range serving (full/HEAD/partial/suffix/unsatisfiable/non-video/missing).
- [x] `go vet ./...`, `go test ./...` clean.
- [x] `svelte-check` clean.
- [x] Multi-stage `Dockerfile`, non-root final image, healthcheck; `docker-compose.yml` example; `.dockerignore`.
- [x] Docker build and run verified: read-only `/media`, read-write `/data`, healthcheck passes.
- [x] `PRODUCT.md`, `ARCHITECTURE.md`, `TASKS.md`, `README.md`.

## Phase 2 — Index and metadata

- [x] SQLite with migrations (pure-Go driver, `modernc.org/sqlite`).
- [x] Recursive scan of all roots; store ID, relative path, size, mtime, duration, dimensions, container, video/audio codec, index status.
- [x] `ffprobe` only for new/changed files (size or mtime changed).
- [x] Remove entries for deleted/renamed files.
- [x] Manual rescan action (`POST /api/scan`) + scan-status endpoint (`GET /api/scan/status`).
- [x] Browsing stays functional during scans; ffprobe fields absent until indexed.
- [x] Backend tests: migration idempotency, indexer (new/unchanged/changed/deleted/unprobeable files, index disposability).
- [x] Docker image includes `ffmpeg`/`ffprobe`; verified end-to-end in a running container (scan status, metadata enrichment, manual rescan, DB deletion + rebuild, concurrent browse during scan).
- [x] `go vet ./...`, `go test ./...`, `go test -race ./...` clean.
- [x] `ARCHITECTURE.md`, `TASKS.md` updated.

## Phase 3 — Thumbnails and polished browsing

- [x] Async thumbnail generation, 1–2 bounded FFmpeg workers (`internal/thumbnail`), low CPU priority via `nice`.
- [x] Frame at ~25% duration, fallback for short/problematic files; `-ss` before `-i`; WebP output under `DATA_DIR/cache/thumbs/`, named from a hash of ID + size + mtime.
- [x] Lazy generation on folder view (`EnqueueFolder`), optional full pre-generation (`POST /api/thumbnails/pregenerate`).
- [x] List, grid, large-thumbnail view modes (`BrowseGrid`/`BrowseList`, `?view=` in the URL).
- [x] Sorting: name (natural), modified time, duration, size (`?sort=` in the URL, `internal/api/sort.go`).
- [x] Flattened (recursive) view using the index (`?flatten=true`, `Store.QueryVideosUnderPath`).
- [x] Lazy-loaded thumbnails (`loading="lazy"`); virtualization not needed yet at this test-library scale — revisit if a real large library proves slow.
- [x] Duration overlay, restrained secondary metadata (resolution), codec compatibility badge (`canPlayType()`).
- [x] Folder state fully in the URL: view/sort/flatten as query params, carried through breadcrumb and folder-card navigation.
- [x] Backend tests: hash/seek-time logic, worker-pool generation/caching/regeneration/permanent-failure behavior, flattened-view SQL (including LIKE-wildcard escaping), sort ordering, thumbnail HTTP endpoints.
- [x] Docker image verified end-to-end: real thumbnail generation (including the 235 MB/23-min test file), lazy vs. pregenerate, streaming stays fast (~1-2ms) during active background thumbnail generation, healthcheck passes.
- [x] `go vet ./...`, `go test ./...`, `go test -race ./...`, `svelte-check` clean.
- [x] `ARCHITECTURE.md`, `TASKS.md`, `README.md` updated.

## Phase 4 — Preview sprites

- [x] One sprite sheet + JSON metadata per video (`internal/preview/sprite.go`; `Store.SetSpriteOK`/`SetSpriteError`).
- [x] Frame count scaled with duration (~1 per 15s, clamped [20, 150], interval recomputed from the clamped count so frames always span the *entire* duration — verified against a real 23-minute file that the last frame lands near the true end, not truncated at ~37 min); ~160px tiles.
- [x] Hover preview (desktop) and touch-drag preview (mobile) on `VideoCard`/`VideoRow`, via `ScrubPreview.svelte`; timeline scrubbing preview above a custom scrub strip in the player, via `TimelineScrubber.svelte` (native `<video controls>` seek bar has no scriptable position, so this is a purpose-built companion control, not an overlay guessing at native chrome).
- [x] Enhancement only — verified: a folder/video with no sprite generated (or a permanently-failed one) browses and plays normally, just without a preview.
- [x] Thumbnail (Phase 3) and sprite generation share one worker pool (`internal/thumbnail` renamed to `internal/preview`) — a second independent pool would have let total FFmpeg concurrency exceed the configured bound.
- [x] Backend tests: frame-count/interval math incl. full-duration-coverage regression test, tile coordinate math, real-ffmpeg sprite generation (cache/regenerate/permanent-failure), shared-pool concurrency, sprite HTTP endpoints.
- [x] Docker end-to-end verification against the real 235 MB/23-minute test file, including catching and fixing a real OOM failure under container memory constraints (see `ARCHITECTURE.md`) and confirming streaming stays fast (~1-2ms) during active sprite generation.
- [x] Two real frontend bugs found via actual browser interaction (not just `svelte-check`): an async-metadata-fetch race that could leave hover preview never appearing, and Svelte 5's passive touch-event listeners silently no-op'ing `preventDefault()` (would have broken drag-vs-tap disambiguation and page-scroll blocking on real devices). Both fixed; see `ARCHITECTURE.md`.
- [x] `go test ./...`, `go vet ./...`, `svelte-check` pass; Docker build succeeds.

## Phase 5 — Hardening and quality of life

- [x] Remember view/sort preferences (`localStorage`, `internal/lib/router.svelte.ts`'s `readPersistedView`/`readPersistedSort`) — an explicit `?view=`/`?sort=` in a URL still always wins.
- [x] Resume position, keyed by video ID (`resume_positions` table, no foreign key to `videos` — survives a full reindex; verified by `TestResumePositionSurvivesReindex`). Dismissable "Resume from X" banner in `Watch.svelte`, never a silent jump; throttled periodic save, best-effort flush on unload/navigate-away, cleared on `ended`.
- [x] Filename search (`GET /api/search?q=&root=`, `Store.SearchVideosByName`) — case-insensitive substring match against the filename only, reading the index (same "absent until indexed" caveat as the flattened view). `Search.svelte` + a search box in `TopBar.svelte`.
- [x] Configurable ignored paths/extensions (`EXTRA_HIDDEN_NAMES`/`EXCLUDED_PATHS`/`EXCLUDED_EXTENSIONS`, `filesystem.Configure`) — enforced inside `filesystem.Browse` itself, so the indexer/flatten/search all inherit it for free; an excluded path is never indexed, not just hidden from listings.
- [x] Clear/rebuild preview controls — `POST /api/previews/clear` (wipe + reset all), per-video retry endpoints, a "Rebuild previews" toolbar button.
- [x] Cache size limits (`CACHE_MAX_BYTES`, oldest-generation-time-first eviction), orphaned-asset cleanup (`POST /api/cache/clean-orphans`, plus automatic after every scan).
- [x] Graceful FFmpeg job cancellation on shutdown — verified empirically (SIGTERM to a running server mid-generation, both directly and via `docker stop`; no orphaned ffmpeg process either way, confirmed via `pgrep`).
- [x] Optional PWA manifest (`web/public/manifest.json` + generated icons, linked from `index.html`).
- [x] Backend tests: resume position (save/load/clear, reindex survival), ignored-paths/extensions (`internal/filesystem`), orphan cleanup/cache-limit eviction/clear-all (`internal/preview`), search and all new endpoints (`internal/api`).
- [x] `go vet ./...`, `go test ./...`, `go test -race ./...`, `svelte-check` clean; Docker build succeeds.
- [x] Docker end-to-end verification: real thumbnail/sprite generation, resume position, search, automatic post-scan orphan cleanup (a touched file's stale cache assets actually removed from disk, confirmed via `ls`), and the shutdown-cancellation check above, all against the real Alpine/libwebp image.

## Phase 6 — Remux for browser-incompatible containers

- [x] `.ts` added to the recognized/streamable video extensions (`internal/media`).
- [x] Remux eligibility check: compatible video (h264, hevc) + audio (aac, mp3) codecs, incompatible container (mkv, avi, ts) — pure function of already-indexed ffprobe data, no new index columns for the decision itself.
- [x] `remux_status`/`remux_error` columns (migration `0005`), mirroring `thumb_status`/`sprite_status` — including the reset-to-`pending`-on-file-change behavior in `UpsertOK`/`UpsertError` (initially missed for `remux_status` specifically; caught by a test and fixed — see `ARCHITECTURE.md`).
- [x] Remux generation shares `preview.Manager`'s existing bounded worker pool (a third job type alongside thumbnails/sprites) — `ffmpeg -map 0:v:0 -map 0:a:0 -c copy -movflags +faststart` to `DATA_DIR/cache/remux/<hash>.remux.mp4`, explicitly mapping only video+audio (some `.ts` files carry a `timed_id3` data stream MP4 can't hold).
- [x] Lazily triggered on folder view (`EnqueueFolder`), same pattern as thumbnails/sprites.
- [x] `GET /api/videos/{id}/stream` serves the cached remux (bounded wait, same pattern as thumbnail `Request`) instead of the original file when eligible; unchanged for everything else — verified with a regression test asserting byte-identical behavior for non-eligible files.
- [x] `GET /api/remux/status`, `POST /api/remux/pregenerate` — mirror the thumbnail/sprite pair.
- [x] Cache size limits / orphan cleanup / clear-all (Phase 5's `internal/preview/cache.go`) extended to cover `cache/remux/` too.
- [x] `remuxEligible` surfaced in the video API response; frontend compatibility badge (`lib/codec.ts`) treats a remux-eligible file as playable rather than flagging it; `.ts` also added to the frontend's own extension→MIME map so a non-eligible `.ts` (incompatible codec) still gets a confident badge instead of falling into "unknown extension".
- [x] Backend tests: eligibility function (compatible/incompatible combinations, case-insensitivity), real-ffmpeg remux generation (cache hit/regenerate-on-change/permanent-failure-not-retried, output verified via `ffprobe` not just file existence), `EnqueueFolder` eligibility filtering (ineligible/unindexed videos never queued), orphan cleanup/clear-all extended to `cache/remux/`, store-level remux lifecycle tests mirroring the thumbnail ones, the stream endpoint (eligible → remux served with corrected Content-Type + working Range requests; non-eligible → byte-identical regression; remux failure → 422).
- [x] `go vet ./...`, `go test ./...`, `go test -race ./...`, `svelte-check` pass; Docker build succeeds.
- [x] Docker end-to-end verification against the user's real `.ts` file (669 MB, 21:18, H.264/AAC with a `timed_id3` stream): scan correctly reported `remuxEligible: true`; `/stream` served a valid MP4 (verified via `ffprobe` on the exact response bytes) with correct `Content-Type: video/mp4`; Range requests returned `206` correctly against the cached remux; real playback in a browser tab advanced in real time and an arbitrary seek (to 15:00 of 21:18) worked correctly with playback resuming from the seeked position — confirming the "pre-generate and cache" design fully resolves the original "seeking during a live remux" open question.

## Phase 7 — Onboarding, settings, and an access gate

- [x] `MEDIA_ROOT` auto-discovers roots by genuine mount boundary, not every subdirectory (`config.DiscoverRoots`, filesystem device ID via `deviceIDFunc`) — a subdirectory that's a real separate mount is promoted to its own root; plain organizational subfolders (the common single-volume case) merge into one synthetic `"main"` root browsed as a folder tree, exactly like Phases 1-6. Corrected after real usage showed the original "every subdirectory is a root" design fragmenting a single mounted library into a disconnected multi-root switcher. Symlinked mounts followed, hidden entries excluded, `config.Root.HiddenChildren` + `filesystem.Browse`'s `hiddenTopLevel` prevent a promoted root's files from also being reachable via `"main"`.
- [x] `DATA_DIR/config.json`: server name, auth credentials, per-root display-name/hidden overrides — separate from `index.db` (`internal/settings`, atomic writes).
- [x] First-run onboarding flow (blocked from re-running once configured — `POST /api/setup` 409s if already configured): server name, admin username/password, name the discovered roots.
- [x] Login page + signed-cookie session (bcrypt password hash, no server-side session table); auth middleware gating the API (`requireAuth`, applied explicitly per-route).
- [x] Top bar shows the configured server name.
- [x] Settings page: rename server/roots, hide a root, rescan for newly mounted roots without a restart, change password (rotates the session secret).
- [x] Root switcher once more than one root exists; roots served in stable (not random map-iteration) order.
- [x] `GET /api/health` deliberately kept public (Docker `HEALTHCHECK` can't authenticate) — caught as a regression during development, fixed, and regression-tested.
- [x] Backend tests: root discovery/merge with overrides, mount-boundary promotion vs. same-device merging (including the all-promoted/no-main and loose-file/forces-main edge cases, via a device-ID test seam since real separate mounts can't be created in a unit test), `Browse`'s `hiddenTopLevel` only applying at a root's own top level, password hashing, session token sign/verify (including expiry and tamper rejection), setup blocked once configured, auth middleware denies unauthenticated requests and allows valid sessions, health stays public, root order is stable, settings round-trip (rename/hide/rescan/password change).
- [x] `go vet ./...`, `go test ./...`, `go test -race ./...`, `svelte-check` pass; Docker build succeeds.
- [x] Docker end-to-end verification against the user's real media, both mount shapes:
  - Single volume (`anime`/`misc`/`tv` all under one `-v host:/media`): correctly discovered as **one** merged library (not three), browsable as a folder tree.
  - Two genuinely separate volumes (`-v host1:/media/videos -v host2:/media/videos2`): correctly discovered as **two** separate libraries — and caught a second real bug in the process: Alpine's base image ships pre-existing empty `/media/{cdrom,floppy,usb}` directories, which briefly produced a spurious third "main" library on every such deployment until `dirHasEntries` excluded empty same-device directories from counting as real content. Fixed and reverified against the live container (full reinit — fresh `config.json`/cache/index, fresh onboarding) with both `videos` and `videos2` showing as exactly two clean, correctly-named roots.
  - Also verified (separately, before the mount-boundary fix, using a since-superseded multi-root test scenario): fresh setup flow, duplicate setup rejected (409), login, session persistence across a container restart (including after deleting `index.db`, proving invariant 5's independence), a newly-mounted root appearing after rescan with no restart, password change rotating the session and rejecting the old password everywhere.

## UI refresh — tags, Recently Added, visual restyle (unplanned, not a numbered phase; `feature/ui-refresh` branch)

Requested directly by the user once Phase 7 was working, alongside a mockup for visual direction. See `ARCHITECTURE.md`'s "Tags and Recently Added" section for design detail.

- [x] Tag system: global, freeform/ad-hoc creation, deterministic color assignment, `tags`/`video_tags` tables (migration `0007`, no FK from `video_tags.video_id` to `videos` — survives reindex like `resume_positions`).
- [x] Tagging UX on all three surfaces: inline on the video card (`VideoCard.svelte`), on the watch/player page (`Watch.svelte`), and bulk (select-mode + `Browse.svelte`'s bulk toolbar).
- [x] Tag filtering, OR semantics (match any selected tag), computed in Go from the already-fetched per-video tag map — no extra SQL query (`filterByAnyTag`/`filterSearchResultsByAnyTag`).
- [x] "Recently Added": new `first_indexed_at_unix` column (migration `0006`), set once across all 8 row-creating upsert paths, never updated after; `GET /api/recent`, `Recent.svelte`, `?sort=added` in Browse.
- [x] Visual restyle: warm amber/stone palette (`app.css`), folders as a separate wide-card row above the video grid, folder-tree toggle moved to a top-bar icon button, reorganized top bar/toolbar. Not a literal copy of the reference mockup's colors/spacing, per the user's own "doesn't have to mirror everything" instruction.
- [x] Favoriting/"show favorites" — explicitly requested by the user to be deprioritized; not built.
- [x] Backend tests: tag lifecycle/case-insensitivity/color-determinism/batch-fetch/bulk/cascade-delete (`internal/database/tags_test.go`), `first_indexed_at_unix` set-once-across-all-upsert-paths behavior (`internal/database/first_indexed_test.go`), recently-added ordering/root-scoping/limit (`internal/database/recent_test.go`), tag and recent API handlers (`internal/api/tags_test.go`, `internal/api/recent_test.go`).
- [x] `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...`, `svelte-check`, `npm run build` all clean — note `svelte-check` alone missed a real build error this session (an invalid `class:` directive on a custom component); `npm run build` is required too, not `svelte-check` alone.
- [x] Browser-verified end-to-end (local preview server, not yet Docker): tag creation/display/filtering, bulk-select + bulk-tag applying to multiple videos correctly, Recently Added page rendering, and a full mobile-viewport (375px) pass — which found and fixed two real bugs: the top bar's search box collapsing to an unusably narrow sliver (`TopBar.svelte`'s brand/root-switcher now shrink-with-ellipsis instead), and the tag-picker popover rendering off-screen to the left when its trigger sits near a container's left edge (`TagPicker.svelte`'s new `align` prop; see `ARCHITECTURE.md`).
- [x] Docker image rebuilt and redeployed to the live container multiple times over this branch's lifetime, most recently after the reliability fix below; `README.md` updated for tags/Recently Added. Still not merged to `main` — this and everything below remain on `feature/ui-refresh`.

## Reliability fix — stuck generation failures, unbounded job duration (unplanned, not a numbered phase)

Prompted by a real deployment problem: thumbnail/sprite generation went from fine to ~30 minutes per file with nothing else running, and separately, a UID/permissions mismatch during deployment failed the first couple hundred thumbnails, which then stayed stuck in `error` status until manually retried one by one. See `ARCHITECTURE.md`'s "Reliability fix" section for the full root-cause writeup, including why a CPU-priority explanation (briefly attempted, uncommitted, by another session) didn't fit the symptom and was reverted.

- [x] Root cause identified: no per-job timeout anywhere in the generation pipeline — every job ran under the server's whole-lifetime context, so one stalled FFmpeg invocation could hold a worker slot forever, backing up everything queued behind it.
- [x] Per-job timeouts added (`internal/preview/manager.go`): 5 min (thumbnail), 20 min (sprite, ~150 sequential frame extractions), 30 min (remux) — generous enough not to wrongly kill a slow-but-fine large file, verified via a real end-to-end test with a 1ms override rather than waiting out a real multi-minute timeout.
- [x] Bounded automatic retry: migration `0008` adds `thumb_retry_count`/`sprite_retry_count`/`remux_retry_count`; a post-scan sweep (`Manager.RequeueFailedForRetry`, wired into the existing `AfterScan` hook alongside the kept auto-`Pregenerate`) requeues failures up to `maxAutoRetries` (3) attempts, then leaves a genuinely broken file alone rather than retrying forever. A manual per-video retry, a full rebuild, or the file itself changing all reset the counter for a fresh budget.
- [x] `nice -n 15` CPU-priority throttling (briefly removed, uncommitted, by another session) restored — doesn't address a stuck-job problem and contradicts this project's explicit Phase 3 "low CPU priority" requirement.
- [x] Backend tests: timeout enforcement kills a hung job and records it as a normal failure (`internal/preview/preview_test.go`'s `TestJobTimeoutKillsHungGeneration`), the full requeue-retry-exhaust-retry-budget cycle through the Manager (`TestRequeueFailedForRetry`) and at the Store layer directly (`internal/database/retry_test.go`: cap enforcement, non-error rows never touched, manual reset / file-change / full-rebuild all restore the retry budget, sprite/remux mirror the thumbnail behavior).
- [x] `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...` all clean.
- [x] Verified against a real failure, not just synthetic tests: made `DATA_DIR/cache/thumbs` read-only, confirmed two real clips correctly failed and recorded `error`, fixed the permission, triggered a rescan, and confirmed both fully recovered (`{"ready":2,"errored":0}` on both `/api/thumbnails/status` and `/api/sprites/status`) with zero manual intervention. Separately, inspected the live deployment's real database directly: found exactly one genuine stuck failure, an audio-only `.mp4` with no video stream at all — confirming the bounded retry cap (not infinite retry) is the correct design, since this file can never succeed no matter how many times it's retried.
- [ ] Not yet done: rebuild and redeploy the Docker image with this fix (the live container is still running the pre-fix image as of this writing).
