#!/usr/bin/env bash
# Guided deploy: builds the image and (re)starts the container from
# .env's settings, but never mounts a host folder without first printing
# exactly which folder maps to which library and asking for confirmation.
# See DEPLOY.md for the full walkthrough.
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f .env ]; then
  echo "No .env found. Create one first:" >&2
  echo "  cp .env.example .env" >&2
  echo "then edit MEDIA_PATHS (and DATA_DIR_HOST/PORT if you want non-defaults)." >&2
  exit 1
fi

set -a
# shellcheck disable=SC1091
source .env
set +a

: "${MEDIA_PATHS:?Set MEDIA_PATHS in .env — see .env.example}"
: "${DATA_DIR_HOST:=./data}"
: "${PORT:=8080}"
: "${CONTAINER_NAME:=playhead}"
: "${IMAGE_NAME:=playhead:latest}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is not installed (or not on PATH). Install Docker first." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon isn't reachable — is Docker running?" >&2
  exit 1
fi

mkdir -p "$DATA_DIR_HOST"
DATA_DIR_ABS="$(cd "$DATA_DIR_HOST" && pwd)"

# Resolve each comma-separated MEDIA_PATHS entry to an absolute host path
# and the /media/<name> subpath it'll be mounted at inside the container
# — a plain basename by default, since that's what the app would show as
# the library's initial display name anyway (renameable later in
# Settings). Every path is validated to actually exist before anything
# is shown, so a typo fails loudly here instead of silently mounting
# nothing (or the wrong thing) into the container.
VOLUME_ARGS=(-v "$DATA_DIR_ABS:/data")
MOUNT_LINES=()
IFS=',' read -ra RAW_PATHS <<< "$MEDIA_PATHS"
for raw in "${RAW_PATHS[@]}"; do
  # Trim leading/trailing whitespace (a "path1, path2" list is easy to type).
  p="$(echo "$raw" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  [ -z "$p" ] && continue
  if [ ! -d "$p" ]; then
    echo "ERROR: '$p' (from MEDIA_PATHS) does not exist or is not a directory." >&2
    exit 1
  fi
  abs="$(cd "$p" && pwd)"
  name="$(basename "$abs")"
  MOUNT_LINES+=("  $abs  ->  /media/$name  (read-only)")
  VOLUME_ARGS+=(-v "$abs:/media/$name:ro")
done

if [ "${#MOUNT_LINES[@]}" -eq 0 ]; then
  echo "ERROR: MEDIA_PATHS in .env didn't resolve to any folder." >&2
  exit 1
fi

echo "This will mount:"
printf '%s\n' "${MOUNT_LINES[@]}"
echo "  $DATA_DIR_ABS  ->  /data  (app's own data: index, thumbnails, settings)"
echo ""
echo "The app will run on http://localhost:$PORT (container name: $CONTAINER_NAME)."
echo ""
read -r -p "Proceed? [y/N] " confirm
case "$confirm" in
  y|Y|yes|YES) ;;
  *) echo "Aborted — nothing was built, mounted, or started."; exit 1 ;;
esac

echo ""
echo "Building image ($IMAGE_NAME)..."
docker build -t "$IMAGE_NAME" .

echo ""
echo "Stopping any existing '$CONTAINER_NAME' container..."
docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true

echo "Starting container..."
docker run -d \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  -p "$PORT:8080" \
  "${VOLUME_ARGS[@]}" \
  -e MEDIA_ROOT=/media \
  -e DATA_DIR=/data \
  "$IMAGE_NAME"

echo ""
echo "Waiting for it to become healthy..."
for _ in $(seq 1 15); do
  status="$(docker inspect --format '{{.State.Health.Status}}' "$CONTAINER_NAME" 2>/dev/null || echo unknown)"
  [ "$status" = "healthy" ] && break
  sleep 1
done

echo ""
docker ps --filter "name=$CONTAINER_NAME"
echo ""
echo "Libraries discovered:"
docker logs "$CONTAINER_NAME" 2>&1 | grep "configured media root" || echo "  (none logged yet — check 'docker logs $CONTAINER_NAME')"
echo ""
if [ "${status:-}" = "healthy" ]; then
  echo "Up and healthy. Open http://localhost:$PORT (or this machine's Tailscale name/IP) to finish first-run setup — set the server name and an admin username/password there yourself."
else
  echo "Container started but isn't reporting healthy yet — check 'docker logs $CONTAINER_NAME'."
fi
echo ""
echo "Reminder: keep this on your private network/Tailscale — the login gate is a convenience layer, not a substitute for not exposing the port to the public internet."
