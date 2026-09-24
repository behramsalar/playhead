package preview

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// contentHash derives a stable identifier from a video's ID plus its
// current size and mtime. A changed file naturally gets a different hash,
// so the cache never serves a stale asset and old files become orphans
// (cleanup is a later phase, not this one). Thumbnails and sprites for
// the same video share this hash so both invalidate together.
func contentHash(videoID string, size, mtimeUnix int64) string {
	h := sha256.New()
	h.Write([]byte(videoID))
	h.Write([]byte{'|'})
	h.Write([]byte(strconv.FormatInt(size, 10)))
	h.Write([]byte{'|'})
	h.Write([]byte(strconv.FormatInt(mtimeUnix, 10)))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// FileName returns the cache filename for a video's thumbnail.
func FileName(videoID string, size, mtimeUnix int64) string {
	return contentHash(videoID, size, mtimeUnix) + ".webp"
}

// spriteImageName and spriteMetaName return the cache filenames for a
// sprite sheet and its JSON sidecar, from the same hash used elsewhere.
func spriteImageName(hash string) string { return hash + ".sprite.webp" }
func spriteMetaName(hash string) string  { return hash + ".json" }

// remuxFileName returns the cache filename for a video's container remux.
func remuxFileName(videoID string, size, mtimeUnix int64) string {
	return contentHash(videoID, size, mtimeUnix) + ".remux.mp4"
}

// SeekTime picks where to grab the representative frame: roughly 20-30%
// into the video, with a fallback for very short or unknown-duration
// files so the seek point is never negative or past the end.
func SeekTime(durationSeconds *float64) float64 {
	if durationSeconds == nil || *durationSeconds <= 0 {
		return 1
	}
	d := *durationSeconds
	t := d * 0.25
	if d < 4 {
		t = d / 2
	}
	if t > d-0.5 {
		t = d - 0.5
	}
	if t < 0 {
		t = 0
	}
	return t
}
