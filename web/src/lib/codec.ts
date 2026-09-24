import type { VideoMetadata } from '../api/types'

// Mirrors internal/media/video.go's extension map — the browser only
// needs the container's MIME type, not server-side stream-serving logic.
const MIME_BY_EXT: Record<string, string> = {
  '.mkv': 'video/x-matroska',
  '.avi': 'video/x-msvideo',
  '.mov': 'video/quicktime',
  '.mp4': 'video/mp4',
  '.m4v': 'video/mp4',
  '.webm': 'video/webm',
  '.ogv': 'video/ogg',
  '.ts': 'video/mp2t',
}

// Simplified codec-string mapping for canPlayType()'s codecs parameter.
// These aren't exact profile/level strings (ffprobe's plain codec name
// doesn't carry that), so canPlayType may occasionally answer "maybe"
// rather than a precise yes/no — that's fine, we only act on a hard "no".
const VIDEO_CODEC_STRINGS: Record<string, string> = {
  h264: 'avc1.42E01E',
  hevc: 'hvc1.1.6.L93.B0',
  vp8: 'vp8',
  vp9: 'vp09.00.10.08',
  av1: 'av01.0.04M.08',
  mpeg4: 'mp4v.20.8',
  theora: 'theora',
}

const AUDIO_CODEC_STRINGS: Record<string, string> = {
  aac: 'mp4a.40.2',
  mp3: 'mp3',
  opus: 'opus',
  vorbis: 'vorbis',
  ac3: 'ac-3',
  flac: 'flac',
}

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i < 0 ? '' : name.slice(i).toLowerCase()
}

// canPlayType() returns '', 'maybe', or 'probably'. We only ever want to
// flag a confident "no" (empty string) — 'maybe'/'probably' both count as
// playable, since browsers are often deliberately vague.
export function isLikelyIncompatible(fileName: string, meta: VideoMetadata): boolean {
  // Phase 6: a remux-eligible file is actually served as a cached MP4, not
  // in its original container, so canPlayType() against the source
  // container/codecs would give a misleading answer — never flag it.
  if (meta.remuxEligible) return false

  const mime = MIME_BY_EXT[extOf(fileName)]
  if (!mime) return false // unknown extension: no confident answer either way

  const video = document.createElement('video')

  const videoCodec = meta.videoCodec ? VIDEO_CODEC_STRINGS[meta.videoCodec] : undefined
  const audioCodec = meta.audioCodec ? AUDIO_CODEC_STRINGS[meta.audioCodec] : undefined

  let query = mime
  if (videoCodec) {
    query += `; codecs="${videoCodec}${audioCodec ? `, ${audioCodec}` : ''}"`
  }

  return video.canPlayType(query) === ''
}
