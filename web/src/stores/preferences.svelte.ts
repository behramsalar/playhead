// UI-only preferences that persist across visits via localStorage. Scoped
// to just what's needed today (the sidebar open/collapsed state) rather
// than a general preferences system — extend deliberately, not speculatively.

const SHOW_SIDEBAR_KEY = 'videoplayer:showSidebar'

function readBool(key: string, fallback: boolean): boolean {
  try {
    const v = localStorage.getItem(key)
    return v === null ? fallback : v === 'true'
  } catch {
    // Private browsing / blocked storage: fall back silently, no crash.
    return fallback
  }
}

function writeBool(key: string, value: boolean) {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // Best-effort; just won't persist across visits.
  }
}

// Open by default on desktop: the sidebar carries primary navigation (All
// Videos / Recently Added / the folder tree), not just a power-user
// folder-tree toggle, so it should be visible out of the box. On a first
// visit from a phone-width viewport, default to closed instead — the
// sidebar renders as a full-width slide-over drawer below the desktop
// breakpoint (see Sidebar.svelte), and starting every mobile session with
// that drawer covering the whole screen would be actively unusable. Once
// a preference is ever saved (either way), it always wins over this.
function defaultShowSidebar(): boolean {
  try {
    return !window.matchMedia('(max-width: 899px)').matches
  } catch {
    return true
  }
}

class Preferences {
  showSidebar = $state(readBool(SHOW_SIDEBAR_KEY, defaultShowSidebar()))

  toggleSidebar() {
    this.showSidebar = !this.showSidebar
    writeBool(SHOW_SIDEBAR_KEY, this.showSidebar)
  }

  setSidebar(open: boolean) {
    this.showSidebar = open
    writeBool(SHOW_SIDEBAR_KEY, open)
  }
}

export const preferences = new Preferences()
