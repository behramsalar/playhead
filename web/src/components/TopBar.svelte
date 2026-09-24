<script lang="ts">
  import { browsePath, recentPath, router, searchPath, SORT_KEYS, VIEW_MODES, type SortKey, type ViewMode } from '../lib/router.svelte'
  import { preferences } from '../stores/preferences.svelte'

  // Filename search is explicitly low-priority (folder browsing is
  // primary) — a plain text box that navigates to /search on submit,
  // not a live-as-you-type autocomplete.
  let value = $state(router.route.name === 'search' ? router.route.query : '')

  $effect(() => {
    if (router.route.name === 'search') value = router.route.query
  })

  function onSubmit(e: SubmitEvent) {
    e.preventDefault()
    router.navigate(searchPath(value))
  }

  // View mode lives in both Browse's and Recent's route state (each has
  // its own query string), but the control for it lives here in the
  // always-rendered top bar — so, like TopBar's other route-derived bits,
  // it just reads router.route directly rather than being prop-drilled
  // down from either route component. Sort only exists on Browse
  // (Recently Added is inherently sorted by recency already).
  let hasViewModes = $derived(router.route.name === 'browse' || router.route.name === 'recent')
  let hasSort = $derived(router.route.name === 'browse')

  const VIEW_ICON_LABEL: Record<ViewMode, string> = { grid: 'Grid view', list: 'List view', large: 'Large thumbnail view' }
  const SORT_LABELS: Record<SortKey, string> = {
    name: 'Name',
    modified: 'Modified',
    duration: 'Duration',
    size: 'Size',
    added: 'Recently added',
  }

  function setView(mode: ViewMode) {
    if (router.route.name === 'browse') {
      const { root, path, sort, flatten, tagIds } = router.route
      router.navigate(browsePath(root, path, { view: mode, sort, flatten, tagIds }), { replace: true })
    } else if (router.route.name === 'recent') {
      router.navigate(recentPath({ view: mode, tagIds: router.route.tagIds }), { replace: true })
    }
  }

  function setSort(sort: SortKey) {
    if (router.route.name !== 'browse') return
    const { root, path, view, flatten, tagIds } = router.route
    router.navigate(browsePath(root, path, { view, sort, flatten, tagIds }), { replace: true })
  }

  // A single read for the view-buttons' active state, so the template
  // doesn't need to re-narrow router.route.name in two different places
  // (Browse and Recent each carry their own `view`, in their own route
  // shape).
  let currentView = $derived<ViewMode>(
    router.route.name === 'browse' || router.route.name === 'recent' ? router.route.view : 'large',
  )
</script>

<header class="top-bar">
  <button
    type="button"
    class="icon-btn sidebar-toggle"
    class:active={preferences.showSidebar}
    aria-pressed={preferences.showSidebar}
    aria-label={preferences.showSidebar ? 'Collapse sidebar' : 'Expand sidebar'}
    onclick={() => preferences.toggleSidebar()}
  >
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7">
      <rect x="3.4" y="4.6" width="17.2" height="14.8" rx="2.2" />
      <line x1="9.6" y1="4.6" x2="9.6" y2="19.4" />
    </svg>
  </button>

  <div class="spacer"></div>

  <form class="search" onsubmit={onSubmit}>
    <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <circle cx="11" cy="11" r="7" /><line x1="21" y1="21" x2="16.4" y2="16.4" />
    </svg>
    <input type="search" placeholder="Search filenames…" bind:value aria-label="Search filenames" />
  </form>

  {#if hasViewModes}
    <div class="views" role="group" aria-label="View mode">
      {#each VIEW_MODES as mode (mode)}
        <button
          type="button"
          class="vmode"
          class:active={currentView === mode}
          aria-label={VIEW_ICON_LABEL[mode]}
          aria-pressed={currentView === mode}
          onclick={() => setView(mode)}
        >
          {#if mode === 'grid'}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <rect x="3.2" y="3.2" width="7.3" height="7.3" rx="1.6" /><rect x="13.5" y="3.2" width="7.3" height="7.3" rx="1.6" />
              <rect x="3.2" y="13.5" width="7.3" height="7.3" rx="1.6" /><rect x="13.5" y="13.5" width="7.3" height="7.3" rx="1.6" />
            </svg>
          {:else if mode === 'list'}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round">
              <line x1="4" y1="6.5" x2="20" y2="6.5" /><line x1="4" y1="12" x2="20" y2="12" /><line x1="4" y1="17.5" x2="20" y2="17.5" />
            </svg>
          {:else}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <rect x="3.4" y="3.4" width="17.2" height="17.2" rx="2.2" />
            </svg>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  {#if hasSort && router.route.name === 'browse'}
    <label class="sortpill">
      <span class="sr-only">Sort by</span>
      <span class="sortpill-label" aria-hidden="true">{SORT_LABELS[router.route.sort]}</span>
      <select value={router.route.sort} onchange={(e) => setSort(e.currentTarget.value as SortKey)}>
        {#each SORT_KEYS as key (key)}
          <option value={key}>{SORT_LABELS[key]}</option>
        {/each}
      </select>
      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polyline points="5,9 12,16 19,9" />
      </svg>
    </label>
  {/if}
</header>

<style>
  .top-bar {
    position: sticky;
    top: 0;
    z-index: 20;
    display: flex;
    align-items: center;
    gap: 10px;
    height: var(--header-height);
    padding: 0 16px;
    background: var(--bg-elevated);
    border-bottom: 1px solid var(--border);
  }

  .icon-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: 8px;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .icon-btn:hover {
    background: var(--bg-hover);
  }

  .icon-btn.active {
    color: var(--accent);
  }

  /* Pushes search/view/sort to the right, leaving the toggle button
     alone at the left — replaces a brand/breadcrumb block that used to
     occupy this space, now that both live elsewhere (Sidebar, and
     Breadcrumbs stays in its own row below). */
  .spacer {
    flex: 1;
    min-width: 0;
  }

  .search {
    flex-shrink: 1;
    min-width: 90px;
    max-width: 280px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--bg);
  }

  .search-icon {
    flex-shrink: 0;
    color: var(--text-tertiary);
  }

  .search input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: none;
    color: var(--text);
    font-size: 0.85rem;
  }

  .search input::placeholder {
    color: var(--text-tertiary);
  }

  .views {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 3px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--bg);
  }

  .vmode {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 7px;
    border: none;
    background: none;
    color: var(--text-tertiary);
    cursor: pointer;
  }

  .vmode:hover {
    color: var(--text);
  }

  .vmode.active {
    background: var(--bg-hover);
    color: var(--accent);
  }

  .sortpill {
    position: relative;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 7px 26px 7px 12px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text-secondary);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .sortpill-label {
    white-space: nowrap;
  }

  .sortpill svg {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    pointer-events: none;
    color: var(--text-tertiary);
  }

  .sortpill select {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    cursor: pointer;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
  }

  /* Below the desktop breakpoint, drop the sort pill's label chrome down
     to icon-width and let the search box shrink further rather than
     wrapping the whole bar to a second line. */
  @media (max-width: 639px) {
    .search {
      min-width: 60px;
    }
  }
</style>
