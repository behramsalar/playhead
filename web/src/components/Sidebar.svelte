<script lang="ts">
  // The app's persistent left navigation: brand, "All Videos"/"Recently
  // Added" (moved out of the top bar per user feedback — this IS "the left
  // expanding menu" they meant), the current library's folder tree, and
  // settings. Rendered once, app-wide (App.svelte), not scoped to the
  // Browse route — the folder tree still needs a root/path to walk, so
  // outside of Browse it falls back to the first configured root with an
  // empty path, same as "All Videos" would land on.
  import { browsePath, recentPath, router, settingsPath, type BrowseQuery } from '../lib/router.svelte'
  import { session } from '../lib/session.svelte'
  import { preferences } from '../stores/preferences.svelte'
  import FolderTree from './FolderTree.svelte'
  import Link from './Link.svelte'

  const DEFAULT_QUERY: BrowseQuery = { view: 'large', sort: 'name', flatten: false, tagIds: [] }

  let currentRoot = $derived(router.route.name === 'browse' ? router.route.root : (session.roots[0]?.id ?? ''))
  let currentPath = $derived(router.route.name === 'browse' ? router.route.path : '')
  let currentQuery = $derived<BrowseQuery>(
    router.route.name === 'browse'
      ? { view: router.route.view, sort: router.route.sort, flatten: router.route.flatten, tagIds: router.route.tagIds }
      : DEFAULT_QUERY,
  )
  let allVideosHref = $derived(browsePath(currentRoot, '', currentQuery))
  let isAllVideosActive = $derived(router.route.name === 'browse')
  let isRecentActive = $derived(router.route.name === 'recent')

  function onRootChange(e: Event) {
    const id = (e.target as HTMLSelectElement).value
    router.navigate(browsePath(id, ''))
  }

  // Mobile: the sidebar is a slide-over drawer (see .sidebar/.backdrop
  // below), so picking a nav link or a folder should close it — otherwise
  // it keeps covering the content pane just navigated to. Delegated on
  // the whole <aside> (any link click, not just the folder tree's own)
  // rather than passed as an onclick prop to each <Link> — Link forwards
  // a custom onclick prop through its `{...rest}` spread, which lands
  // AFTER its own internal SPA-navigation handler and replaces it
  // outright, silently breaking navigation (see Link.svelte).
  function onSidebarClick(e: MouseEvent) {
    if ((e.target as HTMLElement).closest('a')) closeOnMobile()
  }

  function closeOnMobile() {
    if (window.matchMedia('(max-width: 899px)').matches) preferences.setSidebar(false)
  }
</script>

{#if preferences.showSidebar}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="backdrop" onclick={() => preferences.setSidebar(false)}></div>

  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <aside class="sidebar" onclick={onSidebarClick}>
    <Link href="/" class="brand">
      <span class="brand-mark" aria-hidden="true">
        <svg width="15" height="15" viewBox="0 0 24 24"><polygon points="8,5 19,12 8,19" fill="currentColor" /></svg>
      </span>
      <span class="brand-name">{session.serverName || 'Video Server'}</span>
    </Link>

    <nav class="nav" aria-label="Library">
      <Link href={allVideosHref} class={isAllVideosActive ? 'navlink active' : 'navlink'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <rect x="3.2" y="3.2" width="7.3" height="7.3" rx="1.6" /><rect x="13.5" y="3.2" width="7.3" height="7.3" rx="1.6" />
          <rect x="3.2" y="13.5" width="7.3" height="7.3" rx="1.6" /><rect x="13.5" y="13.5" width="7.3" height="7.3" rx="1.6" />
        </svg>
        All Videos
      </Link>
      <Link href={recentPath()} class={isRecentActive ? 'navlink active' : 'navlink'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
          <circle cx="12" cy="12" r="8.5" /><polyline points="12,7 12,12 15.5,14" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        Recently Added
      </Link>
    </nav>

    <div class="divider"></div>

    {#if session.roots.length > 1}
      <select class="root-switcher" value={currentRoot} onchange={onRootChange} aria-label="Media library">
        {#each session.roots as root (root.id)}
          <option value={root.id}>{root.name}</option>
        {/each}
      </select>
    {/if}

    <div class="folders-head">FOLDERS</div>
    <div class="folders-scroll">
      {#if currentRoot}
        <FolderTree root={currentRoot} {currentPath} query={currentQuery} />
      {/if}
    </div>

    <div class="footer">
      <Link href={settingsPath()} class="settings-link" aria-label="Settings">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6">
          <circle cx="12" cy="12" r="3.1" />
          <path
            d="M12 2.5v3M12 18.5v3M21.5 12h-3M5.5 12h-3M18.7 5.3l-2.1 2.1M7.4 16.6l-2.1 2.1M18.7 18.7l-2.1-2.1M7.4 7.4L5.3 5.3"
            stroke-linecap="round"
          />
        </svg>
        Settings
      </Link>
    </div>
  </aside>
{/if}

<style>
  .backdrop {
    display: none;
  }

  .sidebar {
    width: 240px;
    flex-shrink: 0;
    height: 100%;
    box-sizing: border-box;
    padding: 14px 10px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    background: var(--bg-elevated);
    border-right: 1px solid var(--border);
    overflow: hidden;
  }

  :global(.brand) {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 8px 14px;
    flex-shrink: 0;
  }

  .brand-mark {
    flex-shrink: 0;
    width: 26px;
    height: 26px;
    border-radius: 7px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--accent);
    color: var(--bg-elevated);
  }

  .brand-name {
    font-weight: 700;
    font-size: 1rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex-shrink: 0;
  }

  :global(.navlink) {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 8px;
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 0.85rem;
    font-weight: 600;
  }

  :global(.navlink:hover) {
    background: var(--bg-hover);
    color: var(--text);
  }

  :global(.navlink.active) {
    background: color-mix(in srgb, var(--accent) 14%, transparent);
    color: var(--accent);
  }

  .divider {
    height: 1px;
    background: var(--border);
    margin: 10px 4px;
    flex-shrink: 0;
  }

  .root-switcher {
    flex-shrink: 0;
    width: 100%;
    margin-bottom: 10px;
    padding: 6px 8px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.8rem;
  }

  .folders-head {
    flex-shrink: 0;
    padding: 0 8px 8px;
    color: var(--text-tertiary);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.08em;
  }

  .folders-scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .footer {
    flex-shrink: 0;
    padding-top: 10px;
    border-top: 1px solid var(--border);
    margin-top: 8px;
  }

  :global(.settings-link) {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 8px;
    border-radius: 8px;
    color: var(--text-secondary);
    font-size: 0.82rem;
    font-weight: 600;
  }

  :global(.settings-link:hover) {
    background: var(--bg-hover);
    color: var(--text);
  }

  /* A 240px inline column doesn't leave usable space for content on a
     phone-width screen, so below the desktop breakpoint the sidebar
     becomes a slide-over drawer instead: fixed, above the content,
     dismissible via the backdrop or by picking a nav link/folder. */
  @media (max-width: 899px) {
    .backdrop {
      display: block;
      position: fixed;
      inset: 0;
      z-index: 25;
      background: rgba(0, 0, 0, 0.5);
    }

    .sidebar {
      position: fixed;
      top: 0;
      bottom: 0;
      left: 0;
      z-index: 26;
      width: min(80vw, 280px);
      height: 100vh;
      height: 100dvh;
      box-shadow: 2px 0 16px rgba(0, 0, 0, 0.4);
    }
  }
</style>
