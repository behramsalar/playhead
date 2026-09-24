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

<Link href={browsePath(root, folder.path, query)} class="folder-row">
  <div class="icon" aria-hidden="true" style:color>
    <svg viewBox="0 0 24 24" fill="none">
      <path
        d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.379a1.5 1.5 0 0 1 1.06.44l1.122 1.12A1.5 1.5 0 0 0 12.12 7H19.5A1.5 1.5 0 0 1 21 8.5v9A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5v-11Z"
        fill="currentColor"
      />
    </svg>
  </div>
  <span class="name">{folder.name}</span>
</Link>

<style>
  :global(.folder-row) {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    border-bottom: 1px solid var(--border);
  }

  :global(.folder-row:hover) {
    background: var(--bg-hover);
  }

  .icon {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
  }

  .name {
    font-size: 0.9rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
