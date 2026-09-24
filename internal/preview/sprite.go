package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// SpriteFrame is one tile in a sprite sheet.
type SpriteFrame struct {
	Index  int     `json:"index"`
	Time   float64 `json:"time"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
}

// SpriteMeta is the JSON sidecar describing a sprite sheet, enough for a
// client to pick a frame by time or by pointer position and crop it out
// of the sheet without any server-side help.
type SpriteMeta struct {
	TileWidth  int           `json:"tileWidth"`
	TileHeight int           `json:"tileHeight"`
	Columns    int           `json:"columns"`
	Rows       int           `json:"rows"`
	Duration   float64       `json:"duration"`
	Frames     []SpriteFrame `json:"frames"`
}

// BuildSpriteMeta computes frame metadata from the layout — deterministic,
// no need to parse anything out of ffmpeg's own output.
func BuildSpriteMeta(layout SpriteLayout, tileWidth, tileHeight int, duration float64) SpriteMeta {
	frames := make([]SpriteFrame, layout.FrameCount)
	for i := range frames {
		col, row := layout.TilePosition(i)
		frames[i] = SpriteFrame{
			Index:  i,
			Time:   layout.FrameTime(i),
			X:      col * tileWidth,
			Y:      row * tileHeight,
			Width:  tileWidth,
			Height: tileHeight,
		}
	}
	return SpriteMeta{
		TileWidth:  tileWidth,
		TileHeight: tileHeight,
		Columns:    layout.Cols,
		Rows:       layout.Rows,
		Duration:   duration,
		Frames:     frames,
	}
}

// GenerateSprite extracts layout.FrameCount frames from inputPath, evenly
// spaced across its full duration, and assembles them into a single grid
// image at imagePath plus a JSON sidecar at metaPath.
//
// Frames are extracted one at a time — each its own fast, independent
// `-ss`-before`-i` seek, same trick Phase 3's thumbnail generation uses —
// rather than either (a) one fps-filter pass that linearly decodes the
// entire file just to subsample it (measured in minutes for a 23-minute
// 1080p file, since decoding isn't skipped just because most frames are
// dropped), or (b) all frames seeked and decoded *simultaneously* in one
// ffmpeg invocation via many `-ss`/`-i` pairs feeding a filter graph, which
// is fast (~12s for 100 frames) but memory-hungry: up to 150 concurrent
// H.264 decode contexts, each buffering reference frames for a 1080p
// stream, reliably got the process OOM-killed in a memory-constrained
// container in testing — exactly the kind of environment this app
// targets: low idle CPU and memory on modest homelab hardware.
// Sequential extraction keeps at most one decode context live at a time;
// the final montage pass only has to read back small already-scaled
// images, which is cheap regardless of frame count.
func GenerateSprite(ctx context.Context, inputPath, imagePath, metaPath string, layout SpriteLayout, tileWidth, tileHeight int, duration float64) error {
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		return fmt.Errorf("creating sprite cache dir: %w", err)
	}

	framesDir, err := os.MkdirTemp(filepath.Dir(imagePath), "sprite-frames-*")
	if err != nil {
		return fmt.Errorf("creating frame extraction dir: %w", err)
	}
	defer os.RemoveAll(framesDir)

	for i := 0; i < layout.FrameCount; i++ {
		framePath := filepath.Join(framesDir, fmt.Sprintf("frame%04d.png", i))
		if err := extractFrame(ctx, inputPath, framePath, layout.FrameTime(i), tileWidth, tileHeight); err != nil {
			return fmt.Errorf("extracting frame %d/%d: %w", i+1, layout.FrameCount, err)
		}
	}

	tmp := imagePath + ".tmp-" + strconv.Itoa(os.Getpid())
	defer os.Remove(tmp) // no-op once renamed away
	if err := assembleGrid(ctx, framesDir, tmp, layout); err != nil {
		return err
	}

	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced no sprite output")
	}

	meta := BuildSpriteMeta(layout, tileWidth, tileHeight, duration)
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding sprite metadata: %w", err)
	}
	tmpMeta := metaPath + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.WriteFile(tmpMeta, metaBytes, 0o644); err != nil {
		return fmt.Errorf("writing sprite metadata: %w", err)
	}
	defer os.Remove(tmpMeta)

	// Both files are renamed into place only after both succeeded, so a
	// concurrent reader never sees an image without its metadata or vice
	// versa (Request checks for both before reporting Ready).
	if err := os.Rename(tmp, imagePath); err != nil {
		return fmt.Errorf("finalizing sprite image: %w", err)
	}
	if err := os.Rename(tmpMeta, metaPath); err != nil {
		os.Remove(imagePath)
		return fmt.Errorf("finalizing sprite metadata: %w", err)
	}
	return nil
}

// extractFrame grabs one scaled frame at seekSeconds and writes it as a
// PNG (lossless, cheap to decode again for the montage pass — the final
// output is WebP-encoded once, in assembleGrid, not per frame).
func extractFrame(ctx context.Context, inputPath, outputPath string, seekSeconds float64, width, height int) error {
	args := niceArgs()
	args = append(args, "ffmpeg",
		"-ss", strconv.FormatFloat(seekSeconds, 'f', 2, 64),
		"-i", inputPath,
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d", width, height),
		"-y", outputPath,
	)
	return runFFmpeg(ctx, args)
}

// assembleGrid combines the sequentially-extracted frames in framesDir
// (named frame0000.png, frame0001.png, ...) into a single Cols×Rows WebP
// grid at outputPath. This is the only step that touches all frames at
// once, but by now they're small pre-scaled PNGs, not H.264 streams —
// reading them back is cheap regardless of frame count.
func assembleGrid(ctx context.Context, framesDir, outputPath string, layout SpriteLayout) error {
	args := niceArgs()
	args = append(args, "ffmpeg",
		"-framerate", "1",
		"-i", filepath.Join(framesDir, "frame%04d.png"),
		"-vf", fmt.Sprintf("tile=%dx%d", layout.Cols, layout.Rows),
		"-frames:v", "1",
		"-c:v", "libwebp",
		"-q:v", "70",
		"-f", "webp",
		"-y", outputPath,
	)
	return runFFmpeg(ctx, args)
}

func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// The real Go-level error (exit status, signal, timeout) matters
		// for diagnosis as much as ffmpeg's own stderr.
		tail := stderr.String()
		const maxTail = 2000
		if len(tail) > maxTail {
			tail = "…" + tail[len(tail)-maxTail:]
		}
		return fmt.Errorf("ffmpeg failed (%v): %s", err, tail)
	}
	return nil
}
