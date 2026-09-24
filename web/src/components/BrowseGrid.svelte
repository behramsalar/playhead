<script lang="ts">
  // Folders and videos are deliberately two separate sections now (not
  // one mixed grid of same-size tiles): folders render first, as a
  // wrapped row of wide cards, then videos in the responsive grid below.
  import type { FolderEntry, VideoEntry } from '../api/types'
  import type { BrowseQuery } from '../lib/router.svelte'
  import FolderCard from './FolderCard.svelte'
  import VideoCard from './VideoCard.svelte'

  interface Props {
    root: string
    folders: FolderEntry[]
    videos: VideoEntry[]
    query: BrowseQuery
    selectable?: boolean
    selectedIds?: Set<string>
    onToggleSelect?: (id: string) => void
  }

  let { root, folders, videos, query, selectable = false, selectedIds, onToggleSelect }: Props = $props()
</script>

{#if folders.length > 0}
  <div class="folders">
    {#each folders as folder (folder.path)}
      <FolderCard {root} {folder} {query} />
    {/each}
  </div>
{/if}

{#if videos.length > 0}
  <div class="grid" class:large={query.view === 'large'}>
    {#each videos as video (video.id)}
      <VideoCard {video} {selectable} selected={selectedIds?.has(video.id) ?? false} {onToggleSelect} />
    {/each}
  </div>
{/if}

<style>
  .folders {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    padding: 16px 16px 0;
  }

  .folders > :global(.folder-card) {
    flex: 1 1 220px;
    max-width: 340px;
  }

  @media (min-width: 640px) {
    .folders {
      padding: 20px 20px 0;
    }
  }

  /* minmax(0, 1fr), not a plain 1fr: a grid track's automatic minimum
     width otherwise defaults to its content's min-content size, which
     for a nowrap-ellipsis filename is the filename's full unwrapped
     width — long enough to blow the grid past the viewport and force
     horizontal scrolling regardless of the column count below. This is
     the standard "grid blowout" fix. */
  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
    padding: 16px;
  }

  @media (min-width: 640px) {
    .grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 14px;
      padding: 20px;
    }
  }

  @media (min-width: 900px) {
    .grid {
      grid-template-columns: repeat(4, minmax(0, 1fr));
    }
  }

  @media (min-width: 1200px) {
    .grid {
      grid-template-columns: repeat(6, minmax(0, 1fr));
    }
  }

  .grid.large {
    grid-template-columns: repeat(1, minmax(0, 1fr));
  }

  @media (min-width: 640px) {
    .grid.large {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (min-width: 900px) {
    .grid.large {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (min-width: 1200px) {
    .grid.large {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
</style>
