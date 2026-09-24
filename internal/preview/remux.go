package preview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// remuxContainerExts lists file extensions browsers won't reliably
// direct-play regardless of what's inside them. Extensions already
// treated as browser-native (.mp4, .m4v, .mov, .webm, .ogv) are
// deliberately excluded — remuxing an already-compatible container adds
// cost for no benefit.
var remuxContainerExts = map[string]bool{
	".mkv": true,
	".avi": true,
	".ts":  true,
}

// remuxVideoCodecs and remuxAudioCodecs are the codecs a `-c copy` remux
// into MP4 can carry and a browser can decode. A file whose codec isn't
// in these lists (e.g. mpeg2video, common on older TV-tuner recordings)
// isn't a container problem — remuxing wouldn't make it playable, since
// this app never transcodes anything, by design.
var remuxVideoCodecs = map[string]bool{
	"h264": true,
	"hevc": true,
}

var remuxAudioCodecs = map[string]bool{
	"aac": true,
	"mp3": true,
}

// NeedsRemux reports whether a video should be served via a cached
// container-only remux rather than its original bytes. name is the
// video's filename (extension is what matters); videoCodec/audioCodec
// come from the index (ffprobe). Matches the same extension-based
// reasoning the rest of the app already uses for content-type decisions,
// rather than ffprobe's own format_name — which for example reports both
// .mkv and .webm as "matroska,webm", not a useful signal here.
func NeedsRemux(name, videoCodec, audioCodec string) bool {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return false
	}
	if !remuxContainerExts[strings.ToLower(name[i:])] {
		return false
	}
	return remuxVideoCodecs[strings.ToLower(videoCodec)] && remuxAudioCodecs[strings.ToLower(audioCodec)]
}

// remux repackages inputPath's video+audio streams into an MP4 at
// outputPath with `-c copy` — no decoding or re-encoding, just moving
// bytes into a different container, which is why this is fast regardless
// of file length (see ARCHITECTURE.md for measured timings). Only the
// first video and first audio stream are mapped: some real-world .ts
// files carry additional streams (e.g. a timed_id3 metadata stream) that
// MP4 can't hold and that would otherwise fail the mux entirely. Writes
// to a temp file and renames into place, same as thumbnail/sprite
// generation, so a concurrent reader never sees a partial file.
func remux(ctx context.Context, inputPath, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating remux cache dir: %w", err)
	}

	tmp := outputPath + ".tmp-" + strconv.Itoa(os.Getpid())
	defer os.Remove(tmp) // no-op once renamed away

	args := niceArgs()
	args = append(args, "ffmpeg",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a:0",
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4", // the temp filename doesn't end in .mp4, so ffmpeg can't infer the muxer from its extension
		"-y", tmp,
	)

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		tail := stderr.String()
		const maxTail = 2000
		if len(tail) > maxTail {
			tail = "…" + tail[len(tail)-maxTail:]
		}
		return fmt.Errorf("ffmpeg failed (%v): %s", err, tail)
	}

	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced no output")
	}

	if err := os.Rename(tmp, outputPath); err != nil {
		return fmt.Errorf("finalizing remux: %w", err)
	}
	return nil
}
