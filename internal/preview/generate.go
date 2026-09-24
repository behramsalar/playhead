package preview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// niceArgs prefixes an ffmpeg invocation with `nice` when available, so
// thumbnail generation runs at low CPU priority and doesn't compete with
// an actively streaming video. Its absence (some minimal containers won't
// have it) is not an error — generation just runs at normal priority.
func niceArgs() []string {
	if _, err := exec.LookPath("nice"); err != nil {
		return nil
	}
	return []string{"nice", "-n", "15"}
}

// FFmpegAvailable reports whether the ffmpeg binary can be found on PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// generate extracts one frame from inputPath at seekSeconds and writes it
// as WebP to outputPath. It writes to a temp file first and renames into
// place, so a concurrent reader never sees a partial image.
func generate(ctx context.Context, inputPath, outputPath string, seekSeconds float64) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating thumbnail cache dir: %w", err)
	}

	tmp := outputPath + ".tmp-" + strconv.Itoa(os.Getpid())
	defer os.Remove(tmp) // no-op once renamed away

	args := niceArgs()
	args = append(args, "ffmpeg",
		"-ss", strconv.FormatFloat(seekSeconds, 'f', 2, 64),
		"-i", inputPath,
		"-frames:v", "1",
		"-vf", "scale=320:-1",
		"-c:v", "libwebp",
		"-q:v", "80",
		"-f", "webp", // the temp filename doesn't end in .webp, so ffmpeg can't infer the muxer from its extension
		"-y", tmp,
	)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ffmpeg failed: %s", msg)
	}

	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced no output")
	}

	if err := os.Rename(tmp, outputPath); err != nil {
		return fmt.Errorf("finalizing thumbnail: %w", err)
	}
	return nil
}

// GenerateWithFallback tries seekSeconds first; if that fails (a common
// problem right at the end of very short or malformed files), it retries
// once at a safer point before giving up.
func GenerateWithFallback(ctx context.Context, inputPath, outputPath string, seekSeconds float64) error {
	if err := generate(ctx, inputPath, outputPath, seekSeconds); err == nil {
		return nil
	}

	fallback := seekSeconds / 2
	if fallback == seekSeconds {
		fallback = 0
	}
	return generate(ctx, inputPath, outputPath, fallback)
}
