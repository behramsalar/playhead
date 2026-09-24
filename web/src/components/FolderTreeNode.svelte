<script lang="ts">
  // Self-referencing: renders one row plus its expanded children, if any.
  import { browsePath, type BrowseQuery } from '../lib/router.svelte'
  import Link from './Link.svelte'
  import type { TreeNode } from '../lib/tree'
  import FolderTreeNode from './FolderTreeNode.svelte'

  interface Props {
    root: string
    node: TreeNode
    currentPath: string
    query: BrowseQuery
    depth: number
    onToggle: (node: TreeNode) => void
  }

  let { root, node, currentPath, query, depth, onToggle }: Props = $props()

  let isCurrent = $derived(node.path === currentPath)
  // Only known to be childless once we've actually fetched — until then,
  // show the disclosure arrow; a lazy tree can't know in advance.
  let isLeaf = $derived(node.children !== null && node.children.length === 0)
</script>

<div class="row" style="padding-left: {depth * 16 + 4}px">
  {#if isLeaf}
    <span class="spacer"></span>
  {:else}
    <button
      type="button"
      class="disclosure"
      aria-label={node.expanded ? 'Collapse folder' : 'Expand folder'}
      aria-expanded={node.expanded}
      onclick={() => onToggle(node)}
    >
      {#if node.loading}
        <span class="spinner"></span>
      {:else}
        <svg class:expanded={node.expanded} viewBox="0 0 16 16" width="10" height="10">
          <path d="M4 2l8 6-8 6z" fill="currentColor" />
        </svg>
      {/if}
    </button>
  {/if}

  <Link href={browsePath(root, node.path, query)} class="name{isCurrent ? ' current' : ''}">
    {node.name}
  </Link>
</div>

{#if node.expanded && node.children}
  {#each node.children as child (child.path)}
    <FolderTreeNode {root} node={child} {currentPath} {query} depth={depth + 1} {onToggle} />
  {/each}
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 2px;
    padding-right: 8px;
  }

  .disclosure {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 20px;
    height: 28px;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
  }

  .disclosure svg {
    transition: transform 0.1s ease;
  }

  .disclosure svg.expanded {
    transform: rotate(90deg);
  }

  .spacer {
    flex-shrink: 0;
    width: 20px;
  }

  .spinner {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    border: 1.5px solid var(--text-tertiary);
    border-top-color: transparent;
    animation: spin 0.6s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  :global(.name) {
    flex: 1;
    min-width: 0;
    padding: 5px 4px;
    border-radius: 4px;
    font-size: 0.85rem;
    color: var(--text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  :global(.name:hover) {
    background: var(--bg-hover);
    color: var(--text);
  }

  :global(.name.current) {
    background: var(--bg-hover);
    color: var(--text);
    font-weight: 500;
  }
</style>
