// Maps a pointer/touch clientX to a [0, 1] fraction across an element's
// bounding rect — the shared geometry behind both hover and touch-drag
// scrubbing.
export function fractionFromClientX(clientX: number, rect: DOMRect): number {
  if (rect.width === 0) return 0
  return Math.min(1, Math.max(0, (clientX - rect.left) / rect.width))
}
