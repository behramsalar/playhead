<script lang="ts">
  import { ApiError, search as searchApi } from '../api/client'
  import type { SearchResponse, SearchResult, VideoEntry } from '../api/types'
  import StateMessage from '../components/StateMessage.svelte'
  import VideoCard from '../components/VideoCard.svelte'

  interface Props {
    query: string
  }

  let { query }: Props = $props()

  let result = $state<SearchResponse | null>(null)
  let error = $state<ApiError | Error | null>(null)
  let loading = $state(false)

  $effect(() => {
    const q = query
    let cancelled = false

    if (!q.trim()) {
      result = null
      loading = false
      error = null
      return
    }

    loading = true
    error = null
    searchApi(q)
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

  // VideoCard expects a folder-listing VideoEntry; a search result carries
  // the same fields under different names (folderPath instead of
  // subfolder) plus which root it's in, which VideoCard doesn't need —
  // the video ID alone is enough to watch it regardless of source root.
  function asVideoEntry(r: SearchResult): VideoEntry {
    const { root: _root, folderPath, ...rest } = r
    return { ...rest, subfolder: folderPath || undefined }
  }
</script>

{#if !query.trim()}
  <StateMessage title="Search" detail="Type a filename above to search." />
{:else if loading}
  <StateMessage title="Searching…" />
{:else if error}
  <StateMessage title="Search failed" detail={error.message} />
{:else if result && result.results.length === 0}
  <StateMessage title="No matches" detail={`No indexed videos match "${result.query}".`} />
{:else if result}
  <div class="grid">
    {#each result.results as v (v.id)}
      <VideoCard video={asVideoEntry(v)} />
    {/each}
  </div>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
    padding: 16px;
  }

  @media (min-width: 640px) {
    .grid {
      grid-template-columns: repeat(3, 1fr);
      gap: 14px;
      padding: 20px;
    }
  }

  @media (min-width: 900px) {
    .grid {
      grid-template-columns: repeat(4, 1fr);
    }
  }

  @media (min-width: 1200px) {
    .grid {
      grid-template-columns: repeat(6, 1fr);
    }
  }
</style>
