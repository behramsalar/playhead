// Deterministic per-name color, for folders — same approach (FNV-1a hash
// of the lowercased name, modulo a fixed palette) and the same palette
// values as the backend's tag colors (internal/database/tags.go), so a
// folder full of subfolders reads as visually distinct without a picker
// UI, and the app's overall color language stays consistent between tags
// and folders rather than introducing a second, unrelated palette.
const PALETTE = [
  '#f87171', // red
  '#fb923c', // orange
  '#fbbf24', // amber
  '#a3e635', // lime
  '#34d399', // emerald
  '#22d3ee', // cyan
  '#60a5fa', // blue
  '#a78bfa', // violet
  '#f472b6', // pink
  '#94a3b8', // slate
]

export function colorForFolderName(name: string): string {
  let hash = 0x811c9dc5 // FNV-1a 32-bit offset basis
  const lower = name.toLowerCase()
  for (let i = 0; i < lower.length; i++) {
    hash ^= lower.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193) // FNV-1a 32-bit prime
  }
  // Math.imul returns a signed 32-bit int; >>> 0 makes it unsigned before
  // the modulo, matching Go's uint32 arithmetic.
  return PALETTE[(hash >>> 0) % PALETTE.length]
}
