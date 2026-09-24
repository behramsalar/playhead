# DEPLOY.md — deploying this on a home server

This is Playhead, the self-hosted video browser described in `PRODUCT.md`/`README.md`. This guide is written for whoever (human or a Claude Code session) is setting it up on a **new machine** — it assumes no prior context beyond "get this app running here, pointed at my video folders."

## Prerequisites

- Docker installed and running on the target machine (`docker info` should succeed, not error).
- This repository present on the target machine (however it got there — `git clone`, `rsync`, a tarball; not this guide's concern).
- You know which folder(s) on the target machine contain the videos. If you don't yet, stop and find out before continuing — don't guess a path.

Nothing else needs to be installed. `ffmpeg`/`ffprobe` are bundled inside the Docker image; you do not need them on the host.

## The fast path: `deploy.sh`

This is the recommended way to deploy, and the one that satisfies "show me exactly what will be mounted before you touch anything":

```bash
cp .env.example .env
```

Edit `.env`: at minimum, set `MEDIA_PATHS` to the folder(s) that hold the videos, comma-separated for more than one. Each path becomes its **own separate library** in the app (its own entry in the sidebar) — that's about distinct host paths, not subfolders. If you have one folder with everything organized into subfolders underneath it, that's `MEDIA_PATHS=/that/one/folder` (singular) — the subfolders stay as folders you browse into, not separate libraries. Only use multiple comma-separated paths if you actually have multiple, separately-located video collections (e.g. one drive for movies, another for home videos).

If a path contains a space, quote the whole `MEDIA_PATHS` value — `.env.example` has an example.

`DATA_DIR_HOST` (where the app's own database/thumbnails/settings live) and `PORT` have sensible defaults (`./data`, `8080`) — only change them if you need to.

Then run:

```bash
./deploy.sh
```

It will:

1. Resolve every path in `MEDIA_PATHS` and confirm each one actually exists (fails loudly on a typo, before touching Docker at all).
2. Print exactly which host folder will be mounted to which library name inside the container, plus the data directory — and **stop and ask for confirmation** before building or running anything.
3. Build the image, stop/remove any previous container of the same name (an upgrade re-run, not a first deploy, would hit this), and start the new one.
4. Wait for the container's healthcheck to pass and print which libraries it actually discovered (parsed straight from its own startup logs), so you can confirm what got mounted matches what you expected.

If something looks wrong in the confirmation prompt (wrong folder, wrong count of libraries), answer `n` and fix `.env` — nothing has been touched yet at that point.

## What happens after `deploy.sh` finishes

The container is running, but the app itself isn't configured yet. Open `http://<this-machine>:<PORT>` (or the machine's Tailscale name, if reached that way) in a browser — you'll land on a first-run setup page: a server name, an admin username/password, and confirming/renaming the discovered libraries.

**This step should be done by the person who will actually use the app, not automated** — it sets the real login credential that gates access afterward. If you're an agent doing this deployment on someone else's behalf, stop here and hand the URL back to them to finish setup themselves, rather than filling in a username/password for them.

## Verifying it worked

```bash
docker ps --filter name=playhead          # or your CONTAINER_NAME, if changed
docker logs playhead --tail 30
curl -s http://localhost:8080/api/health      # adjust the port if you changed it
```

`docker logs` should show one `configured media root id=... name=...` line per folder in `MEDIA_PATHS`. `curl .../api/health` should return quickly with no error (it's deliberately left reachable without logging in, for exactly this kind of check).

## The alternative: `docker-compose.yml`

`deploy.sh` is preferred when you have more than one media folder, or just want the confirmation step. For a single library and no interactive step, plain Compose works too:

1. Edit `docker-compose.yml`'s `volumes:` line — replace `/path/to/your/videos` with your real path.
2. `docker compose up -d --build`

This path does **not** print a confirmation before mounting — read the `volumes:` line yourself before running it. It only supports one media path cleanly; for more than one, either add more `volumes:` entries by hand (each `/host/path:/media/<name>:ro`, matching a genuinely different host path per entry) or just use `deploy.sh` instead.

## Updating later

Get the newer code onto the machine (however you originally did), then re-run whichever of the two paths above you used the first time. `DATA_DIR_HOST` (the database, thumbnails, and — importantly — your login/settings) is untouched by a rebuild; only the application image changes. You will not need to redo first-run setup after an update.

## Security

The login gate this app has is a convenience layer for a shared home network, not a substitute for keeping it off the public internet — see `README.md`'s notes on the Tailscale/private-LAN boundary. Don't port-forward `PORT` from your router. If you want HTTPS, `README.md`'s "HTTPS on your Tailscale network" section covers `tailscale serve`.

## Troubleshooting

- **`docker: permission denied` / daemon not reachable** — Docker isn't running, or the user running this script isn't in the `docker` group (Linux). Fix that first; `deploy.sh` checks for this and fails with a clear message rather than a confusing one deeper in.
- **A library didn't show up / has the wrong name** — check `docker logs <container>` for the `configured media root` lines; the name defaults to the mounted folder's own basename and can be renamed later from the app's Settings page once you're logged in.
- **Healthcheck never turns healthy** — `docker logs <container>` for the actual startup error; a common cause is `DATA_DIR_HOST` pointing somewhere Docker can't write (permissions).
- **Deleting `data/index.db` (or the whole data dir) to force a full rescan** — safe for the media index specifically; by design (see `ARCHITECTURE.md` invariant 2) it does not affect the source video files, and — separately — it does not erase your server name/login/settings, which live in `data/config.json`. Deleting the *entire* `DATA_DIR_HOST` does remove settings/login along with the index, requiring first-run setup again.
