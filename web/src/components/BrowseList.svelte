<script lang="ts">
  import type { FolderEntry, VideoEntry } from '../api/types'
  import type { BrowseQuery } from '../lib/router.svelte'
  import FolderRow from './FolderRow.svelte'
  import VideoRow from './VideoRow.svelte'

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

<div class="list">
  {#each folders as folder (folder.path)}
    <FolderRow {root} {folder} {query} />
  {/each}
  {#each videos as video (video.id)}
    <VideoRow {video} {selectable} selected={selectedIds?.has(video.id) ?? false} {onToggleSelect} />
  {/each}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
  }
</style>
