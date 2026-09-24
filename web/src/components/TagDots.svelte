<script lang="ts">
  import type { Tag } from '../api/types'

  interface Props {
    tags: Tag[] | undefined
    max?: number
  }

  let { tags, max = 3 }: Props = $props()
  let shown = $derived((tags ?? []).slice(0, max))
</script>

{#if shown.length > 0}
  <div class="tag-dots" aria-hidden="true">
    {#each shown as tag (tag.id)}
      <span class="dot" style:background={tag.color} title={tag.name}></span>
    {/each}
  </div>
{/if}

<style>
  .tag-dots {
    position: absolute;
    z-index: 3;
    left: 6px;
    bottom: 6px;
    display: flex;
    gap: 3px;
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    box-shadow: 0 0 0 1.5px rgba(0, 0, 0, 0.5);
  }
</style>
