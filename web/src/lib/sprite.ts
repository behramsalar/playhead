import type { SpriteFrame, SpriteMeta } from '../api/types'

// Frames are evenly spaced in time by construction (see
// internal/preview.ComputeSpriteLayout on the backend), so picking one by
// fractional position is just an index computation — no need to search by
// timestamp.
export function frameForFraction(meta: SpriteMeta, fraction: number): SpriteFrame {
  const clamped = Math.min(1, Math.max(0, fraction))
  const index = Math.min(meta.frames.length - 1, Math.round(clamped * (meta.frames.length - 1)))
  return meta.frames[index]
}

export function frameForTime(meta: SpriteMeta, timeSeconds: number): SpriteFrame {
  return frameForFraction(meta, meta.duration > 0 ? timeSeconds / meta.duration : 0)
}

// Returns the inline style needed to show just one tile of the sprite
// sheet as a cropped background image, scaled to fill its container
// responsively (not the tile's native pixel size) — the classic
// percentage CSS sprite-sheet technique: background-size as multiples of
// 100% makes each cell exactly one container-size, and background-position
// as percentages (which position "this % of the image" at "this % of the
// container") lands on the right cell regardless of the container's
// actual rendered size.
export function tileStyle(meta: SpriteMeta, frame: SpriteFrame, spriteImageUrl: string): string {
  const col = frame.x / frame.width
  const row = frame.y / frame.height
  const posX = meta.columns > 1 ? (col / (meta.columns - 1)) * 100 : 0
  const posY = meta.rows > 1 ? (row / (meta.rows - 1)) * 100 : 0

  return (
    `background-image: url(${JSON.stringify(spriteImageUrl)});` +
    `background-position: ${posX}% ${posY}%;` +
    `background-size: ${meta.columns * 100}% ${meta.rows * 100}%;` +
    `background-repeat: no-repeat;`
  )
}
