export interface RootInfo {
  id: string
  name: string
}

export interface Breadcrumb {
  name: string
  path: string
}

export interface FolderEntry {
  name: string
  path: string
}

// Tag is a freeform, global (cross-library) label a video can carry.
export interface Tag {
  id: number
  name: string
  color: string
}

// Present only when the file has been successfully indexed — ffprobe
// fields are simply absent until indexed, so all optional.
export interface VideoMetadata {
  duration?: number
  width?: number
  height?: number
  container?: string
  videoCodec?: string
  audioCodec?: string
  // Phase 6: browser-incompatible container, browser-compatible codecs —
  // the backend serves this file as a cached remuxed MP4 instead of its
  // original bytes, so the frontend's compatibility check should treat it
  // as playable regardless of what canPlayType() says about the original
  // container/codec strings.
  remuxEligible?: boolean
  // addedAt is present as soon as the file has any index row at all
  // (indexed successfully or not, or just thumbnailed) — unlike the
  // other fields above, it isn't gated on a successful probe.
  addedAt?: string
}

export interface VideoEntry extends VideoMetadata {
  id: string
  name: string
  path: string
  size: number
  modified: string
  // Only set in the flattened (recursive) view: the video's folder path
  // relative to the browsed folder, for display as secondary text.
  subfolder?: string
  tags?: Tag[]
}

export interface BrowseResult {
  root: string
  path: string
  breadcrumbs: Breadcrumb[]
  folders: FolderEntry[]
  videos: VideoEntry[]
  sort?: string
  flatten?: boolean
}

export interface VideoInfo extends VideoMetadata {
  id: string
  root: string
  name: string
  path: string
  folderPath: string
  size: number
  modified: string
  resumeSeconds?: number
  tags?: Tag[]
}

export interface SearchResult extends VideoMetadata {
  id: string
  root: string
  name: string
  path: string
  folderPath: string
  size: number
  modified: string
  tags?: Tag[]
}

export interface SearchResponse {
  query: string
  results: SearchResult[]
}

export interface RecentResponse {
  videos: SearchResult[]
}

export interface TagWithCount extends Tag {
  count: number
}

export interface TagsResponse {
  tags: TagWithCount[]
}

export interface SpriteFrame {
  index: number
  time: number
  x: number
  y: number
  width: number
  height: number
}

export interface SpriteMeta {
  tileWidth: number
  tileHeight: number
  columns: number
  rows: number
  duration: number
  frames: SpriteFrame[]
}

export interface ApiErrorBody {
  error: {
    code: string
    message: string
  }
}

// Phase 7: onboarding, auth, and settings.

// ConfigResponse is GET /api/config's body — the one endpoint the SPA
// shell always needs regardless of auth state, so it can decide whether
// to render onboarding, login, or the normal app.
export interface ConfigResponse {
  configured: boolean
  authenticated: boolean
  serverName?: string
  roots: RootInfo[]
}

export interface RootSetting {
  id: string
  folderName: string
  displayName: string
  hidden: boolean
}

export interface SettingsResponse {
  serverName: string
  username: string
  roots: RootSetting[]
}

export interface RootOverrideInput {
  displayName?: string
  hidden?: boolean
}
