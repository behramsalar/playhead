<script lang="ts">
  import { browse, ApiError } from '../api/client'
  import type { BrowseResult } from '../api/types'
  import Breadcrumbs from '../components/Breadcrumbs.svelte'
  import BrowseGrid from '../components/BrowseGrid.svelte'
  import BrowseList from '../components/BrowseList.svelte'
  import BrowseToolbar from '../components/BrowseToolbar.svelte'
  import StateMessage from '../components/StateMessage.svelte'
  import TagFilterBar from '../components/TagFilterBar.svelte'
  import TagPicker from '../components/TagPicker.svelte'
  import { browsePath, router, type BrowseQuery } from '../lib/router.svelte'

  interface Props extends BrowseQuery {
    root: string
    path: string
  }

  let { root, path, view, sort, flatten, tagIds }: Props = $props()
  let query = $derived<BrowseQuery>({ view, sort, flatten, tagIds })

  let result = $state<BrowseResult | null>(null)
  let error = $state<ApiError | Error | null>(null)
  let loading = $state(true)

  $effect(() => {
    // Re-run whenever root/path/sort/flatten/tagIds changes (including
    // via back/forward); view mode alone doesn't need a refetch.
    const currentRoot = root
    const currentPath = path
    const currentSort = sort
    const currentFlatten = flatten
    const currentTagIds = tagIds
    let cancelled = false

    loading = true
    error = null
    result = null

    browse(currentRoot, currentPath, { sort: currentSort, flatten: currentFlatten, tagIds: currentTagIds })
      .then((r) => {
        if (!cancelled) result = r
      })
      .catch((e) => {
        if (!cancelled) error = e
      })
      .finally(() => {
        if (!cancelled) loading = false
      })

    return () => {
      cancelled = true
    }
  })

  function onTagFilterChange(ids: number[]) {
    router.navigate(browsePath(root, path, { ...query, tagIds: ids }), { replace: true })
  }

  // Selection mode for bulk tagging — reset whenever the folder changes,
  // so a stale selection from a previous folder can't silently apply.
  let selecting = $state(false)
  let selectedIds = $state<Set<string>>(new Set())
  let bulkPickerOpen = $state(false)

  $effect(() => {
    root
    path
    selecting = false
    selectedIds = new Set()
  })

  function toggleSelecting() {
    selecting = !selecting
    selectedIds = new Set()
    bulkPickerOpen = false
  }

  // Distinct from toggleSelecting (which flips): this always exits,
  // regardless of current state — used after a bulk tag is applied, so
  // the picker closing and leaving select mode happen as one step
  // instead of requiring an explicit "Cancel" click to see the result.
  function exitSelecting() {
    selecting = false
    selectedIds = new Set()
    bulkPickerOpen = false
  }

  function toggleSelect(id: string) {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds = next
  }
</script>

<div class="main">
  {#if loading}
    <StateMessage title="Loading…" />
  {:else if error}
    {#if error instanceof ApiError && error.status === 503}
      <StateMessage title="Media root unavailable" detail={error.message} />
    {:else if error instanceof ApiError && error.status === 404}
      <StateMessage title="Folder not found" detail="This folder doesn't exist or was removed." />
    {:else}
      <StateMessage title="Couldn't load this folder" detail={error.message} />
    {/if}
  {:else if result}
    <Breadcrumbs root={result.root} crumbs={result.breadcrumbs} {query} />
    <BrowseToolbar {root} {path} {query} {selecting} onToggleSelecting={toggleSelecting} />
    <TagFilterBar selectedIds={tagIds} onChange={onTagFilterChange} />

    {#if selecting}
      <div class="bulk-bar">
        <span class="count">{selectedIds.size} selected</span>
        <div class="bulk-actions">
          <button
            type="button"
            disabled={selectedIds.size === 0}
            onclick={() => (bulkPickerOpen = !bulkPickerOpen)}
          >
            Tag selected…
          </button>
          {#if bulkPickerOpen && selectedIds.size > 0}
            <TagPicker videoIds={[...selectedIds]} align="start" onClose={exitSelecting} />
          {/if}
        </div>
      </div>
    {/if}

    {#if result.folders.length === 0 && result.videos.length === 0}
      <StateMessage
        title={flatten ? 'No indexed videos here yet' : 'This folder is empty'}
        detail={flatten ? "Videos appear here once they've been indexed." : undefined}
      />
    {:else if view === 'list'}
      <BrowseList
        root={result.root}
        folders={result.folders}
        videos={result.videos}
        {query}
        selectable={selecting}
        {selectedIds}
        onToggleSelect={toggleSelect}
      />
    {:else}
      <BrowseGrid
        root={result.root}
        folders={result.folders}
        videos={result.videos}
        {query}
        selectable={selecting}
        {selectedIds}
        onToggleSelect={toggleSelect}
      />
    {/if}
  {/if}
</div>

<style>
  .main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .bulk-bar {
    position: relative;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 16px;
    background: color-mix(in srgb, var(--accent) 10%, transparent);
    border-bottom: 1px solid var(--border);
  }

  @media (min-width: 640px) {
    .bulk-bar {
      padding: 8px 20px;
    }
  }

  .count {
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .bulk-actions {
    position: relative;
  }

  .bulk-actions button {
    padding: 6px 12px;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .bulk-actions button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
