<script lang="ts">
  import type { FolderEntry } from '../api/types'
  import { colorForFolderName } from '../lib/folderColor'
  import { browsePath, type BrowseQuery } from '../lib/router.svelte'
  import Link from './Link.svelte'

  interface Props {
    root: string
    folder: FolderEntry
    query: BrowseQuery
  }

  let { root, folder, query }: Props = $props()
  let color = $derived(colorForFolderName(folder.name))
</script>

<Link href={browsePath(root, folder.path, query)} class="folder-card">
  <div class="icon-wrap" aria-hidden="true" style:background="color-mix(in srgb, {color} 16%, transparent)">
    <svg class="icon" viewBox="0 0 24 24" fill="none" style:color>
      <path
        d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.379a1.5 1.5 0 0 1 1.06.44l1.122 1.12A1.5 1.5 0 0 0 12.12 7H19.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5v-11Z"
        fill="currentColor"
      />
    </svg>
  </div>
  <span class="name" title={folder.name}>{folder.name}</span>
  <svg class="chevron" viewBox="0 0 24 24" fill="none" aria-hidden="true">
    <path d="M9 6l6 6-6 6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
</Link>

<style>
  /* A wide horizontal card, deliberately not sized/shaped like a video
     tile — folders and videos are different kinds of things (a folder is
     a place to go, a video is content to watch), so this phase makes
     that visually obvious instead of mixing same-size tiles of both into
     one grid. */
  :global(.folder-card) {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    border-radius: var(--radius);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
  }

  :global(.folder-card:hover) {
    background: var(--bg-hover);
  }

  .icon-wrap {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    border-radius: 8px;
  }

  .icon {
    width: 18px;
    height: 18px;
  }

  .name {
    flex: 1;
    min-width: 0;
    font-size: 0.85rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chevron {
    flex-shrink: 0;
    width: 16px;
    height: 16px;
    color: var(--text-tertiary);
  }
</style>
