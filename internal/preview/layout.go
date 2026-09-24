package preview

import "math"

// nominalIntervalSeconds is the target spacing between sprite frames
// before the [20, 150] frame-count clamp adjusts it.
const nominalIntervalSeconds = 15.0

// SpriteLayout describes one video's sprite sheet: a Cols×Rows grid of
// FrameCount frames, each Interval seconds apart, covering the video's
// entire duration.
type SpriteLayout struct {
	Cols       int
	Rows       int
	FrameCount int
	Interval   float64 // seconds between frames
}

// ComputeSpriteLayout picks a frame count and interval for a video of the
// given duration.
//
// The [20, 150] frame count is a cap on sprite size/generation cost, not
// on how much of the video gets covered: the interval is recomputed from
// the (grid-rounded) frame count so frames always span the video's full
// duration. A video far longer than 150×15s still gets its last frame
// near the end, just spaced further apart — never truncated to a preview
// of only its first ~37 minutes.
func ComputeSpriteLayout(durationSeconds float64) SpriteLayout {
	if durationSeconds <= 0 {
		durationSeconds = 1
	}

	natural := int(math.Round(durationSeconds / nominalIntervalSeconds))
	if natural < 20 {
		natural = 20
	}
	if natural > 150 {
		natural = 150
	}

	cols := int(math.Ceil(math.Sqrt(float64(natural))))
	rows := int(math.Ceil(float64(natural) / float64(cols)))
	frameCount := cols * rows // exact grid fill, so every tile is a real frame

	return SpriteLayout{
		Cols:       cols,
		Rows:       rows,
		FrameCount: frameCount,
		Interval:   durationSeconds / float64(frameCount),
	}
}

// FrameTime returns the timestamp (seconds) of frame index (0-based).
func (l SpriteLayout) FrameTime(index int) float64 {
	return float64(index) * l.Interval
}

// TilePosition returns frame index's column and row within the grid.
func (l SpriteLayout) TilePosition(index int) (col, row int) {
	return index % l.Cols, index / l.Cols
}
