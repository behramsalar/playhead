// A small hand-written router: state lives in the URL (pathname + query),
// so reload and back/forward work without any extra bookkeeping.

export type ViewMode = 'grid' | 'list' | 'large'
export type SortKey = 'name' | 'modified' | 'duration' | 'size' | 'added'

export const VIEW_MODES: ViewMode[] = ['grid', 'list', 'large']
export const SORT_KEYS: SortKey[] = ['name', 'modified', 'duration', 'size', 'added']

export interface BrowseQuery {
  view: ViewMode
  sort: SortKey
  flatten: boolean
  tagIds: number[]
}

const DEFAULT_QUERY: BrowseQuery = { view: 'large', sort: 'name', flatten: false, tagIds: [] }

// Remembered view/sort preferences (not flatten — that's a per-folder
// choice, not a general one): an explicit ?view=/&sort= in a URL always
// wins, but a bare /browse/:root link falls back to whatever you last
// used instead of always resetting to the hardcoded default.
const VIEW_STORAGE_KEY = 'videoplayer:defaultView'
const SORT_STORAGE_KEY = 'videoplayer:defaultSort'

function readPersistedView(): ViewMode {
  try {
    const v = localStorage.getItem(VIEW_STORAGE_KEY)
    return VIEW_MODES.includes(v as ViewMode) ? (v as ViewMode) : DEFAULT_QUERY.view
  } catch {
    return DEFAULT_QUERY.view
  }
}

function readPersistedSort(): SortKey {
  try {
    const v = localStorage.getItem(SORT_STORAGE_KEY)
    return SORT_KEYS.includes(v as SortKey) ? (v as SortKey) : DEFAULT_QUERY.sort
  } catch {
    return DEFAULT_QUERY.sort
  }
}

function persistViewSort(view: ViewMode, sort: SortKey) {
  try {
    localStorage.setItem(VIEW_STORAGE_KEY, view)
    localStorage.setItem(SORT_STORAGE_KEY, sort)
  } catch {
    // Private browsing / blocked storage: just won't persist.
  }
}

// Recently Added has a view mode but no sort (it's always newest-first),
// so it persists just the view half of the same remembered preference —
// switching to grid there and back to a folder view keeps grid, and vice
// versa, since both read/write the one shared VIEW_STORAGE_KEY.
function persistView(view: ViewMode) {
  try {
    localStorage.setItem(VIEW_STORAGE_KEY, view)
  } catch {
    // Private browsing / blocked storage: just won't persist.
  }
}

export type Route =
  | { name: 'home' }
  | ({ name: 'browse'; root: string; path: string } & BrowseQuery)
  | { name: 'watch'; id: string }
  | { name: 'search'; query: string }
  | { name: 'settings' }
  | ({ name: 'recent' } & Pick<BrowseQuery, 'view' | 'tagIds'>)
  | { name: 'notfound' }

function parseTagIds(raw: string | null): number[] {
  if (!raw) return []
  return raw
    .split(',')
    .map((s) => Number(s.trim()))
    .filter((n) => Number.isInteger(n) && n > 0)
}

function parseQuery(search: string): BrowseQuery {
  const params = new URLSearchParams(search)
  const view = params.get('view')
  const sort = params.get('sort')
  return {
    view: VIEW_MODES.includes(view as ViewMode) ? (view as ViewMode) : readPersistedView(),
    sort: SORT_KEYS.includes(sort as SortKey) ? (sort as SortKey) : readPersistedSort(),
    flatten: params.get('flatten') === 'true',
    tagIds: parseTagIds(params.get('tags')),
  }
}

function parse(pathname: string, search: string): Route {
  const segments = pathname.split('/').filter(Boolean).map(decodeURIComponent)

  if (segments.length === 0) {
    return { name: 'home' }
  }
  if (segments[0] === 'browse' && segments.length >= 2) {
    const [, root, ...rest] = segments
    return { name: 'browse', root, path: rest.join('/'), ...parseQuery(search) }
  }
  if (segments[0] === 'watch' && segments.length === 2) {
    return { name: 'watch', id: segments[1] }
  }
  if (segments[0] === 'search' && segments.length === 1) {
    return { name: 'search', query: new URLSearchParams(search).get('q') ?? '' }
  }
  if (segments[0] === 'settings' && segments.length === 1) {
    return { name: 'settings' }
  }
  if (segments[0] === 'recent' && segments.length === 1) {
    const q = parseQuery(search)
    return { name: 'recent', view: q.view, tagIds: q.tagIds }
  }
  return { name: 'notfound' }
}

class Router {
  route: Route = $state(parse(window.location.pathname, window.location.search))

  constructor() {
    window.addEventListener('popstate', () => {
      this.route = parse(window.location.pathname, window.location.search)
    })
  }

  navigate(to: string, opts: { replace?: boolean } = {}) {
    if (opts.replace) {
      history.replaceState({}, '', to)
    } else {
      history.pushState({}, '', to)
    }
    this.route = parse(window.location.pathname, window.location.search)
  }
}

export const router = new Router()

// Build a /browse/:root/*path URL, percent-encoding each segment so
// unicode, spaces, and slashes-within-names all round-trip correctly.
// query carries the view/sort/flatten state forward (defaults omitted
// from the URL to keep plain folder links clean).
export function browsePath(root: string, path: string, query: Partial<BrowseQuery> = {}): string {
  const segments = path ? path.split('/') : []
  const encoded = [root, ...segments].map(encodeURIComponent)
  const url = `/browse/${encoded.join('/')}`

  const merged = { ...DEFAULT_QUERY, ...query }
  persistViewSort(merged.view, merged.sort)

  const params = new URLSearchParams()
  if (merged.view !== DEFAULT_QUERY.view) params.set('view', merged.view)
  if (merged.sort !== DEFAULT_QUERY.sort) params.set('sort', merged.sort)
  if (merged.flatten) params.set('flatten', 'true')
  if (merged.tagIds.length) params.set('tags', merged.tagIds.join(','))

  const qs = params.toString()
  return qs ? `${url}?${qs}` : url
}

export function watchPath(id: string): string {
  return `/watch/${id}`
}

export function searchPath(query?: string): string {
  if (!query) return '/search'
  return `/search?q=${encodeURIComponent(query)}`
}

export function settingsPath(): string {
  return '/settings'
}

export function recentPath(query: { view?: ViewMode; tagIds?: number[] } = {}): string {
  const view = query.view ?? readPersistedView()
  const tagIds = query.tagIds ?? []
  persistView(view)

  const params = new URLSearchParams()
  if (view !== DEFAULT_QUERY.view) params.set('view', view)
  if (tagIds.length) params.set('tags', tagIds.join(','))

  const qs = params.toString()
  return qs ? `/recent?${qs}` : '/recent'
}
