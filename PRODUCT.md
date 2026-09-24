# PRODUCT.md — Playhead

## What this is

A lightweight, single-user, filesystem-oriented web app for browsing and playing miscellaneous video files on a homelab server. It preserves the user's existing folder structure and plays compatible files directly in the browser.

## Who it's for

One person, running it on their own home server, browsing their own loosely organized video collection (home movies, downloaded clips, recordings — not a curated movie/TV library) from a laptop or phone over Tailscale.

## Scope

- One or more configured media roots, browsed as real folders.
- A "show all videos under this folder" flattened/recursive view.
- List, grid, and large-thumbnail view modes.
- Static thumbnails and lightweight hover/touch preview frames.
- Timeline scrubbing previews in the player.
- Native HTML5 playback with range-based seeking, including in long (~2h+) files.
- Responsive, touch-friendly UI with a working phone experience.
- A single Docker container with low idle CPU and memory.
- A first-run setup flow and an optional single shared-credential login gate (Phase 7).
- Freeform tags (global, ad-hoc creation) for organizing and filtering videos independent of folder structure, and a "Recently Added" view/sort order.

## Non-goals

This is **not** a Jellyfin/Plex alternative. It deliberately does not do:

- User accounts, multi-user sharing, or a full authentication system (OAuth, per-user accounts, password resets). A single shared username/password gate (Phase 7) is a narrow, deliberate exception — still one credential, no per-user accounts.
- Internet exposure (Tailscale/private LAN is the trust boundary — the login gate is a convenience layer on top of it, not a replacement).
- Movie/TV metadata scraping, posters, cast data, recommendations.
- Subtitle or multi-audio-track selection.
- HLS/DASH or any transcoding, live or on-demand.
- Playlists, comments, ratings, social features.
- Redis, Postgres, message brokers, or separate worker services.
- Any modification of media files: never rename, move, upload, or delete.

## Current status

Phases 1–7 are implemented (foundation, indexing, thumbnails/previews, hardening/QoL — search, resume position, configurable exclusions, cache management — container remux for browser-incompatible MKV/AVI/TS files with compatible codecs, and auto-discovered multi-root support with first-run onboarding and a single shared-credential access gate). See `TASKS.md` for the full phased roadmap and `ARCHITECTURE.md` for the technical invariants.

An unplanned UI refresh followed on top of Phase 7 (tags, Recently Added, a visual restyle) — see `TASKS.md`'s "UI refresh" section and `ARCHITECTURE.md`'s "Tags and Recently Added" section. Favoriting/a "show favorites" view was considered alongside it and deliberately deprioritized.
