<script lang="ts">
  import type { Tag } from '../api/types'
  import { tagRegistry } from '../lib/tags.svelte'

  interface Props {
    // One ID for a single video's editor (with remove buttons on its
    // current tags); several for the bulk "tag all selected" flow (add
    // only — the selected videos don't share one tag list to show).
    videoIds: string[]
    currentTags?: Tag[]
    onClose: () => void
    // Fired with the exact tag added/removed, so a card can update its
    // own local copy without refetching the whole listing.
    onAdd?: (tag: Tag) => void
    onRemove?: (tagId: number) => void
    // 'end' (default) anchors the popover's right edge to its trigger —
    // right for a button near a container's right edge (VideoCard's
    // top-right tag button). 'start' anchors the left edge instead, for
    // a trigger near the left edge of a narrow container (Watch page's
    // "+ Tag" button), where 'end' would push the popover off-screen.
    align?: 'start' | 'end'
  }

  let { videoIds, currentTags = [], onClose, onAdd, onRemove, align = 'end' }: Props = $props()
  let bulk = videoIds.length > 1

  let input = $state('')
  let busy = $state(false)
  let panelEl: HTMLDivElement | undefined = $state()

  let suggestions = $derived(
    tagRegistry.tags
      .filter((t) => !currentTags.some((c) => c.id === t.id))
      .filter((t) => t.name.toLowerCase().includes(input.trim().toLowerCase()))
      .slice(0, 6),
  )

  async function applyTag(name: string) {
    const trimmed = name.trim()
    if (!trimmed || busy) return
    busy = true
    try {
      const tag = bulk ? await tagRegistry.bulkAdd(videoIds, trimmed) : await tagRegistry.addToVideo(videoIds[0], trimmed)
      input = ''
      onAdd?.(tag)
      // Bulk tagging is a one-shot action, not an editing session: once
      // the tag has landed on every selected video, there's nothing left
      // to do here, so close immediately rather than leaving the picker
      // (and the caller's selection UI) open until the user notices and
      // dismisses it manually. The single-video picker stays open, since
      // that one *is* meant for adding/removing several tags in a row.
      if (bulk) onClose()
    } catch {
      // Best-effort UI; the tag list simply won't update if this fails.
    } finally {
      busy = false
    }
  }

  async function removeTag(tagId: number) {
    if (bulk || busy) return
    busy = true
    try {
      await tagRegistry.removeFromVideo(videoIds[0], tagId)
      onRemove?.(tagId)
    } finally {
      busy = false
    }
  }

  function onSubmit(e: SubmitEvent) {
    e.preventDefault()
    applyTag(input)
  }

  function onWindowPointerDown(e: PointerEvent) {
    if (panelEl && !panelEl.contains(e.target as Node)) onClose()
  }

  function onWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onpointerdown={onWindowPointerDown} onkeydown={onWindowKeydown} />

<div class="picker" class:align-start={align === 'start'} bind:this={panelEl}>
  {#if bulk}
    <p class="hint">Tag {videoIds.length} selected videos</p>
  {:else if currentTags.length > 0}
    <div class="current">
      {#each currentTags as tag (tag.id)}
        <button type="button" class="chip" style:background={tag.color} onclick={() => removeTag(tag.id)}>
          {tag.name}
          <span class="remove" aria-hidden="true">×</span>
        </button>
      {/each}
    </div>
  {/if}

  <form onsubmit={onSubmit}>
    <input type="text" placeholder="Add a tag…" bind:value={input} onclick={(e) => e.stopPropagation()} />
  </form>

  {#if suggestions.length > 0}
    <ul class="suggestions">
      {#each suggestions as tag (tag.id)}
        <li>
          <button type="button" onclick={() => applyTag(tag.name)}>
            <span class="dot" style:background={tag.color}></span>
            {tag.name}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    position: absolute;
    z-index: 30;
    top: calc(100% + 4px);
    right: 0;
    width: 200px;
    max-width: calc(100vw - 32px);
    padding: 8px;
    border-radius: 10px;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .picker.align-start {
    right: auto;
    left: 0;
  }

  .hint {
    margin: 0;
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .current {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 2px 6px;
    border-radius: 999px;
    border: none;
    color: #fff;
    font-size: 0.7rem;
    cursor: pointer;
  }

  .remove {
    font-size: 0.85rem;
    line-height: 1;
  }

  input {
    width: 100%;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg);
    color: var(--text);
    font-size: 0.8rem;
  }

  .suggestions {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 140px;
    overflow-y: auto;
  }

  .suggestions button {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 5px 6px;
    border: none;
    background: none;
    color: var(--text);
    font-size: 0.8rem;
    text-align: left;
    border-radius: 6px;
    cursor: pointer;
  }

  .suggestions button:hover {
    background: var(--bg-hover);
  }

  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
  }
</style>
