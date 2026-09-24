<script lang="ts">
  import { tagRegistry } from '../lib/tags.svelte'

  interface Props {
    selectedIds: number[]
    onChange: (ids: number[]) => void
  }

  let { selectedIds, onChange }: Props = $props()

  $effect(() => {
    if (!tagRegistry.loaded) tagRegistry.refresh()
  })

  function toggle(id: number) {
    onChange(selectedIds.includes(id) ? selectedIds.filter((i) => i !== id) : [...selectedIds, id])
  }
</script>

{#if tagRegistry.tags.length > 0}
  <div class="filter-bar">
    <span class="label">Filter by tag</span>
    <div class="pills">
      {#each tagRegistry.tags as tag (tag.id)}
        <button
          type="button"
          class="pill"
          class:active={selectedIds.includes(tag.id)}
          onclick={() => toggle(tag.id)}
        >
          <span class="dot" style:background={tag.color}></span>
          {tag.name}
          <span class="count">{tag.count}</span>
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .filter-bar {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    padding: 10px 16px 0;
  }

  @media (min-width: 640px) {
    .filter-bar {
      padding: 12px 20px 0;
    }
  }

  .label {
    flex-shrink: 0;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-tertiary);
  }

  .pills {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }

  .pill {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
    font-size: 0.78rem;
    cursor: pointer;
  }

  .pill:hover {
    background: var(--bg-hover);
  }

  .pill.active {
    border-color: var(--accent);
    background: color-mix(in srgb, var(--accent) 16%, transparent);
    color: var(--text);
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .count {
    color: var(--text-tertiary);
    font-variant-numeric: tabular-nums;
  }
</style>
