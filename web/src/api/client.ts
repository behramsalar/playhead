import type {
  ApiErrorBody,
  BrowseResult,
  ConfigResponse,
  RecentResponse,
  RootInfo,
  RootOverrideInput,
  SearchResponse,
  SettingsResponse,
  SpriteMeta,
  Tag,
  TagsResponse,
  VideoInfo,
} from './types'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

// onUnauthorized lets the session store learn about a 401 from *any* API
// call (not just the ones it triggers itself) — e.g. a session that
// expires mid-use while browsing hits this on the next request, and the
// app should drop back to the login screen instead of showing a confusing
// error in whatever component happened to be making that call.
let unauthorizedHandler: (() => void) | null = null
export function onUnauthorized(handler: () => void): void {
  unauthorizedHandler = handler
}

async function throwForError(res: Response): Promise<never> {
  let code = 'unknown_error'
  let message = `Request failed with status ${res.status}`
  try {
    const body = (await res.json()) as ApiErrorBody
    if (body?.error) {
      code = body.error.code
      message = body.error.message
    }
  } catch {
    // Non-JSON error body; fall back to the generic message above.
  }
  if (res.status === 401) unauthorizedHandler?.()
  throw new ApiError(res.status, code, message)
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) await throwForError(res)
  return (await res.json()) as T
}

// requestVoid is for endpoints that reply 204 No Content (resume-position
// save/clear) — no body to parse on success.
async function requestVoid(url: string, init: RequestInit): Promise<void> {
  const res = await fetch(url, init)
  if (!res.ok) await throwForError(res)
}

export function getRoots(): Promise<{ roots: RootInfo[] }> {
  return request('/api/roots')
}

export interface BrowseOptions {
  sort?: string
  flatten?: boolean
  tagIds?: number[]
}

export function browse(root: string, path: string, opts: BrowseOptions = {}): Promise<BrowseResult> {
  const params = new URLSearchParams({ root, path })
  if (opts.sort) params.set('sort', opts.sort)
  if (opts.flatten) params.set('flatten', 'true')
  if (opts.tagIds?.length) params.set('tags', opts.tagIds.join(','))
  return request(`/api/browse?${params.toString()}`)
}

export function getVideo(id: string): Promise<VideoInfo> {
  return request(`/api/videos/${id}`)
}

export function streamUrl(id: string): string {
  return `/api/videos/${id}/stream`
}

export function thumbnailUrl(id: string): string {
  return `/api/videos/${id}/thumbnail`
}

export function spriteUrl(id: string): string {
  return `/api/videos/${id}/sprite`
}

export function getSpriteMeta(id: string): Promise<SpriteMeta> {
  return request(`/api/videos/${id}/sprite.json`)
}

export function saveResumePosition(id: string, positionSeconds: number): Promise<void> {
  return requestVoid(`/api/videos/${id}/resume`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ positionSeconds }),
  })
}

export function clearResumePosition(id: string): Promise<void> {
  return requestVoid(`/api/videos/${id}/resume`, { method: 'DELETE' })
}

export function search(query: string, root?: string): Promise<SearchResponse> {
  const params = new URLSearchParams({ q: query })
  if (root) params.set('root', root)
  return request(`/api/search?${params.toString()}`)
}

export function clearPreviews(): Promise<{ filesRemoved: number }> {
  return request('/api/previews/clear', { method: 'POST' })
}

// ---- Phase 7: onboarding, auth, settings --------------------------------

export function getConfig(): Promise<ConfigResponse> {
  return request('/api/config')
}

export interface SetupInput {
  serverName: string
  username: string
  password: string
  roots?: Record<string, RootOverrideInput>
}

export function setup(input: SetupInput): Promise<ConfigResponse> {
  return request('/api/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function login(username: string, password: string): Promise<ConfigResponse> {
  return request('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
}

export function logout(): Promise<void> {
  return requestVoid('/api/auth/logout', { method: 'POST' })
}

export function getSettings(): Promise<SettingsResponse> {
  return request('/api/settings')
}

export interface UpdateSettingsInput {
  serverName?: string
  roots?: Record<string, RootOverrideInput>
}

export function updateSettings(input: UpdateSettingsInput): Promise<SettingsResponse> {
  return request('/api/settings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export function rescanRoots(): Promise<SettingsResponse> {
  return request('/api/settings/rescan', { method: 'POST' })
}

export function changePassword(
  currentPassword: string,
  updates: { newUsername?: string; newPassword?: string },
): Promise<void> {
  return requestVoid('/api/settings/password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ currentPassword, ...updates }),
  })
}

// ---- Tags -----------------------------------------------------------------

export function getTags(): Promise<TagsResponse> {
  return request('/api/tags')
}

export function addTagToVideo(videoId: string, name: string): Promise<Tag> {
  return request(`/api/videos/${videoId}/tags`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
}

export function removeTagFromVideo(videoId: string, tagId: number): Promise<void> {
  return requestVoid(`/api/videos/${videoId}/tags/${tagId}`, { method: 'DELETE' })
}

export function bulkTagVideos(videoIds: string[], name: string): Promise<Tag> {
  return request('/api/tags/bulk', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ videoIds, name }),
  })
}

export function deleteTag(tagId: number): Promise<void> {
  return requestVoid(`/api/tags/${tagId}`, { method: 'DELETE' })
}

// ---- Recently added ---------------------------------------------------

export interface RecentOptions {
  root?: string
  limit?: number
  tagIds?: number[]
}

export function getRecent(opts: RecentOptions = {}): Promise<RecentResponse> {
  const params = new URLSearchParams()
  if (opts.root) params.set('root', opts.root)
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.tagIds?.length) params.set('tags', opts.tagIds.join(','))
  const qs = params.toString()
  return request(`/api/recent${qs ? `?${qs}` : ''}`)
}
