<script lang="ts">
  import { clearPreviews } from '../api/client'
  import { browsePath, router, type BrowseQuery } from '../lib/router.svelte'

  interface Props {
    root: string
    path: string
    query: BrowseQuery
    selecting?: boolean
    onToggleSelecting?: () => void
  }

  let { root, path, query, selecting = false, onToggleSelecting }: Props = $props()

  function go(next: Partial<BrowseQuery>) {
    router.navigate(browsePath(root, path, { ...query, ...next }), { replace: true })
  }

  let rebuilding = $state(false)

  async function onRebuildPreviews() {
    if (rebuilding) return
    if (!confirm('Clear all thumbnails and preview sprites and regenerate them? This may take a while for a large library.')) {
      return
    }
    rebuilding = true
    try {
      await clearPreviews()
    } catch {
      // Best-effort control; the thumbnails/sprites status endpoints
      // remain the source of truth for whether it actually worked.
    } finally {
      rebuilding = false
    }
  }
</script>

<div class="toolbar">
  <button type="button" class="flatten" class:active={query.flatten} onclick={() => go({ flatten: !query.flatten })}>
    Show all videos
  </button>

  <button type="button" class="toggle-btn" class:active={selecting} onclick={() => onToggleSelecting?.()}>
    {selecting ? 'Cancel' : 'Select'}
  </button>

  <button type="button" class="toggle-btn" disabled={rebuilding} onclick={onRebuildPreviews}>
    {rebuilding ? 'Clearing…' : 'Rebuild previews'}
  </button>
</div>

<style>
  /* Wraps onto multiple lines rather than relying on flex-shrink plus a
     horizontal scrollbar — every control renders at its natural size, or
     moves to the next line, on any viewport width. */
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    padding: 8px 16px;
    border-bottom: 1px solid var(--border);
  }

  .flatten,
  .toggle-btn {
    flex-shrink: 0;
    padding: 6px 12px;
    font-size: 0.8rem;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    cursor: pointer;
    white-space: nowrap;
  }

  .flatten.active,
  .toggle-btn.active {
    background: var(--accent);
    border-color: var(--accent);
    color: white;
  }

  .toggle-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
