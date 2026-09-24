package preview_test

import (
	"testing"

	"playhead/internal/preview"
)

func TestComputeSpriteLayoutFrameCountBounds(t *testing.T) {
	cases := []struct {
		name          string
		duration      float64
		minFrameCount int
		maxFrameCount int // generous upper bound: grid rounding can overshoot 150 a bit
	}{
		{"very short clip", 5, 20, 30},
		{"typical clip", 20 * 60, 20, 156},    // 20 min: natural=80, within range
		{"very long clip", 4 * 3600, 20, 156}, // 4 hours: must clamp down, not blow up
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := preview.ComputeSpriteLayout(tc.duration)
			if l.FrameCount < tc.minFrameCount || l.FrameCount > tc.maxFrameCount {
				t.Fatalf("FrameCount = %d, want in [%d, %d]", l.FrameCount, tc.minFrameCount, tc.maxFrameCount)
			}
			if l.Cols*l.Rows != l.FrameCount {
				t.Fatalf("Cols(%d)*Rows(%d) = %d, want FrameCount %d (grid must be exactly filled)", l.Cols, l.Rows, l.Cols*l.Rows, l.FrameCount)
			}
		})
	}
}

// TestComputeSpriteLayoutCoversFullDuration is the critical regression
// test for the "does a long video's preview get truncated" question: a
// naive fixed-interval-then-stop-at-the-cap implementation would only
// cover the video's first ~150*15s = 37.5 minutes. The interval must
// instead stretch so the cap only limits sprite sheet size, never the
// portion of the video it represents.
func TestComputeSpriteLayoutCoversFullDuration(t *testing.T) {
	durations := []float64{
		45 * 60,  // 45 min: just over the naive 37.5 min truncation point
		2 * 3600, // 2 hours
		4 * 3600, // 4 hours: a very long recording
	}
	for _, duration := range durations {
		l := preview.ComputeSpriteLayout(duration)
		lastFrameTime := l.FrameTime(l.FrameCount - 1)

		// The last frame must land within one interval of the true end —
		// i.e. genuinely near the end of the video, not stuck at ~2250s
		// (150*15s) regardless of how long the video actually is. Slack is
		// mathematically exactly one interval by construction
		// (FrameCount*Interval == duration); allow tiny float64 rounding.
		const epsilon = 1e-9
		if slack := duration - lastFrameTime; slack < -epsilon || slack > l.Interval+epsilon {
			t.Fatalf("duration=%.0fs: last frame at %.1fs, interval=%.1fs, slack=%.6fs — coverage looks truncated, not spanning the full duration",
				duration, lastFrameTime, l.Interval, slack)
		}
	}
}

func TestSpriteLayoutFrameTimeAndTilePosition(t *testing.T) {
	l := preview.SpriteLayout{Cols: 5, Rows: 4, FrameCount: 20, Interval: 10}

	if got := l.FrameTime(0); got != 0 {
		t.Fatalf("FrameTime(0) = %v, want 0", got)
	}
	if got := l.FrameTime(3); got != 30 {
		t.Fatalf("FrameTime(3) = %v, want 30", got)
	}

	cases := []struct {
		index   int
		wantCol int
		wantRow int
	}{
		{0, 0, 0},
		{4, 4, 0},
		{5, 0, 1},
		{19, 4, 3},
	}
	for _, tc := range cases {
		col, row := l.TilePosition(tc.index)
		if col != tc.wantCol || row != tc.wantRow {
			t.Fatalf("TilePosition(%d) = (%d, %d), want (%d, %d)", tc.index, col, row, tc.wantCol, tc.wantRow)
		}
	}
}

func TestBuildSpriteMetaTileCoordinates(t *testing.T) {
	l := preview.SpriteLayout{Cols: 3, Rows: 2, FrameCount: 6, Interval: 5}
	meta := preview.BuildSpriteMeta(l, 160, 90, 30)

	if len(meta.Frames) != 6 {
		t.Fatalf("got %d frames, want 6", len(meta.Frames))
	}
	if meta.Columns != 3 || meta.Rows != 2 || meta.TileWidth != 160 || meta.TileHeight != 90 {
		t.Fatalf("unexpected sheet dimensions: %+v", meta)
	}

	// Frame 4 is at grid position (col=1, row=1): x = 160, y = 90.
	f := meta.Frames[4]
	if f.X != 160 || f.Y != 90 {
		t.Fatalf("frame 4 tile position = (%d, %d), want (160, 90)", f.X, f.Y)
	}
	if f.Time != 20 {
		t.Fatalf("frame 4 time = %v, want 20", f.Time)
	}
}
