<script lang="ts">
  import { ApiError, getRecent } from '../api/client'
  import type { RecentResponse, SearchResult, VideoEntry } from '../api/types'
  import BrowseGrid from '../components/BrowseGrid.svelte'
  import BrowseList from '../components/BrowseList.svelte'
  import StateMessage from '../components/StateMessage.svelte'
  import TagFilterBar from '../components/TagFilterBar.svelte'
  import { recentPath, router, type ViewMode } from '../lib/router.svelte'

  interface Props {
    view: ViewMode
    tagIds: number[]
  }

  let { view, tagIds }: Props = $props()

  let result = $state<RecentResponse | null>(null)
  let error = $state<ApiError | Error | null>(null)
  let loading = $state(true)

  $effect(() => {
    const currentTagIds = tagIds
    let cancelled = false

    loading = true
    error = null

    getRecent({ tagIds: currentTagIds })
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
    router.navigate(recentPath({ view, tagIds: ids }), { replace: true })
  }

  // VideoCard/VideoRow (via BrowseGrid/BrowseList) expect a folder-listing
  // VideoEntry; a recent-added result carries the same fields under
  // different names (folderPath instead of subfolder) plus which root
  // it's in — see Search.svelte's identical conversion, since /api/recent
  // reuses the same DTO shape as search (both span every root, unlike a
  // normal folder browse, which is also why there's no single `root` to
  // pass BrowseGrid/BrowseList below — it's only used there to build
  // folder links, and this view never has any folders).
  function asVideoEntry(r: SearchResult): VideoEntry {
    const { root: _root, folderPath, ...rest } = r
    return { ...rest, subfolder: folderPath || undefined }
  }

  // BrowseGrid/BrowseList also take a full BrowseQuery for folder-card
  // links (irrelevant here, since `folders` is always empty) and grid
  // sizing (`query.view`, which does matter). sort/flatten don't apply to
  // this view; filled in with harmless defaults rather than widening
  // BrowseQuery's required fields for one caller that doesn't use them.
  let query = $derived({ view, sort: 'added' as const, flatten: false, tagIds })
</script>

<div class="page">
  <div class="header">
    <h1>Recently Added</h1>
  </div>
  <TagFilterBar selectedIds={tagIds} onChange={onTagFilterChange} />

  {#if loading && !result}
    <StateMessage title="Loading…" />
  {:else if error}
    <StateMessage title="Couldn't load recently added videos" detail={error.message} />
  {:else if result && result.videos.length === 0}
    <StateMessage title="Nothing here yet" detail="Videos appear here once they've been indexed." />
  {:else if result}
    {@const videos = result.videos.map(asVideoEntry)}
    {#if view === 'list'}
      <BrowseList root="" folders={[]} {videos} {query} />
    {:else}
      <BrowseGrid root="" folders={[]} {videos} {query} />
    {/if}
  {/if}
</div>

<style>
  .page {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .header {
    padding: 16px 16px 0;
  }

  @media (min-width: 640px) {
    .header {
      padding: 20px 20px 0;
    }
  }

  h1 {
    font-size: 1.1rem;
    margin: 0;
  }
</style>
