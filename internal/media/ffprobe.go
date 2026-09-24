package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// ProbeResult is the subset of ffprobe's output the indexer cares about.
type ProbeResult struct {
	DurationSeconds float64
	Width           int
	Height          int
	Container       string
	VideoCodec      string
	AudioCodec      string
}

// FFprobeAvailable reports whether the ffprobe binary can be found on
// PATH. The indexer checks this once at startup so a missing binary
// produces one clear log line instead of a probe failure per file.
func FFprobeAvailable() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

type probeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

type probeOutput struct {
	Format  probeFormat   `json:"format"`
	Streams []probeStream `json:"streams"`
}

// Probe runs ffprobe against absPath and parses its JSON output. It never
// touches the media directory beyond reading this one file read-only.
func Probe(ctx context.Context, absPath string) (ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		absPath,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return ProbeResult{}, fmt.Errorf("ffprobe failed: %s", msg)
	}

	var out probeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return ProbeResult{}, fmt.Errorf("parsing ffprobe output: %w", err)
	}

	result := ProbeResult{
		Container: out.Format.FormatName,
	}
	if out.Format.Duration != "" {
		if d, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil {
			result.DurationSeconds = d
		}
	}

	for _, s := range out.Streams {
		switch s.CodecType {
		case "video":
			if result.VideoCodec == "" {
				result.VideoCodec = s.CodecName
				result.Width = s.Width
				result.Height = s.Height
			}
		case "audio":
			if result.AudioCodec == "" {
				result.AudioCodec = s.CodecName
			}
		}
	}

	if result.VideoCodec == "" {
		return ProbeResult{}, fmt.Errorf("no video stream found")
	}

	return result, nil
}
