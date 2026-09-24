// Reactive session state, driving App.svelte's top-level gate: not
// configured -> onboarding, configured but not authenticated -> login,
// both -> the normal app. Mirrors router.svelte.ts's pattern of a small
// hand-written reactive class rather than pulling in a state library.
import { getConfig, onUnauthorized } from '../api/client'
import type { RootInfo } from '../api/types'

class Session {
  loading = $state(true)
  configured = $state(false)
  authenticated = $state(false)
  serverName = $state('')
  roots = $state<RootInfo[]>([])

  // refresh re-fetches GET /api/config, the one endpoint always reachable
  // regardless of auth state. Called on initial load and after
  // setup/login/logout/settings changes that could change any of the
  // above.
  async refresh(): Promise<void> {
    try {
      const cfg = await getConfig()
      this.configured = cfg.configured
      this.authenticated = cfg.authenticated
      this.serverName = cfg.serverName ?? ''
      this.roots = cfg.roots
    } finally {
      this.loading = false
    }
  }
}

export const session = new Session()

// A 401 from any API call (not just login/setup's own) means the current
// session is no longer valid — drop back to the login screen rather than
// showing a confusing error wherever that call happened to originate.
onUnauthorized(() => {
  session.authenticated = false
})
