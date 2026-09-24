# syntax=docker/dockerfile:1

# --- frontend build -----------------------------------------------------
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- backend build -------------------------------------------------------
FROM golang:1.27-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY web/embed.go ./web/embed.go
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- runtime ---------------------------------------------------------------
# ffmpeg (for ffprobe) arrives here in Phase 2; the ffmpeg *encoding* tools
# it also provides aren't used until Phase 3's thumbnail generation.
FROM alpine:3.20
RUN apk add --no-cache ffmpeg && \
    addgroup -S app && adduser -S -G app app
COPY --from=backend /out/server /usr/local/bin/server
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/server"]
