export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex++
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[unitIndex]}`
}

const RELATIVE_UNITS: [Intl.RelativeTimeFormatUnit, number][] = [
  ['year', 365 * 24 * 60 * 60],
  ['month', 30 * 24 * 60 * 60],
  ['week', 7 * 24 * 60 * 60],
  ['day', 24 * 60 * 60],
  ['hour', 60 * 60],
  ['minute', 60],
]

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function formatRelativeTime(iso: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const diffSeconds = (then - Date.now()) / 1000

  for (const [unit, secondsInUnit] of RELATIVE_UNITS) {
    if (Math.abs(diffSeconds) >= secondsInUnit) {
      return rtf.format(Math.round(diffSeconds / secondsInUnit), unit)
    }
  }
  return rtf.format(Math.round(diffSeconds), 'second')
}

// Truncates a filename in the middle so the extension and tail stay
// visible, which matters more than the start for distinguishing files.
export function truncateMiddle(name: string, maxLen = 40): string {
  if (name.length <= maxLen) return name
  const keep = maxLen - 1 // reserve one char for the ellipsis
  const head = Math.ceil(keep * 0.6)
  const tail = keep - head
  return `${name.slice(0, head)}…${name.slice(name.length - tail)}`
}

// Formats seconds as m:ss, growing to h:mm:ss for anything an hour or
// longer — the standard video-duration convention.
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.round(totalSeconds))
  const hours = Math.floor(s / 3600)
  const minutes = Math.floor((s % 3600) / 60)
  const seconds = s % 60

  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
  }
  return `${minutes}:${String(seconds).padStart(2, '0')}`
}
