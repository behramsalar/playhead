<script lang="ts">
  import type { Breadcrumb } from '../api/types'
  import { browsePath, type BrowseQuery } from '../lib/router.svelte'
  import Link from './Link.svelte'

  interface Props {
    root: string
    crumbs: Breadcrumb[]
    query: BrowseQuery
  }

  let { root, crumbs, query }: Props = $props()
</script>

<nav class="breadcrumbs" aria-label="Folder path">
  {#each crumbs as crumb, i (crumb.path)}
    {#if i > 0}
      <span class="sep">/</span>
    {/if}
    {#if i === crumbs.length - 1}
      <span class="current">{crumb.name}</span>
    {:else}
      <Link href={browsePath(root, crumb.path, query)} class="crumb">{crumb.name}</Link>
    {/if}
  {/each}
</nav>

<style>
  .breadcrumbs {
    position: sticky;
    /* main (not the document) is the scroll container now, and TopBar
       lives outside main entirely (a separate flex sibling, always
       visible on its own) — so this only needs to clear main's own top
       edge, not TopBar's height again. Offsetting by --header-height
       here (as when the whole page used to scroll together) double-
       counted TopBar's height and pushed this down over the toolbar
       below it. */
    top: 0;
    z-index: 10;
    display: flex;
    align-items: center;
    gap: 6px;
    height: var(--breadcrumb-height);
    padding: 0 16px;
    background: var(--bg);
    border-bottom: 1px solid var(--border);
    overflow-x: auto;
    white-space: nowrap;
    font-size: 0.9rem;
  }

  :global(.crumb) {
    color: var(--text-secondary);
    padding: 4px 2px;
  }

  :global(.crumb:hover) {
    color: var(--text);
  }

  .current {
    color: var(--text);
    font-weight: 500;
  }

  .sep {
    color: var(--text-tertiary);
  }
</style>
