<script lang="ts">
  import { thumbnailUrl } from '../api/client'
  import type { Tag, VideoEntry } from '../api/types'
  import { isLikelyIncompatible } from '../lib/codec'
  import { formatBytes, formatDuration, formatRelativeTime, truncateMiddle } from '../lib/format'
  import { router, watchPath } from '../lib/router.svelte'
  import Link from './Link.svelte'
  import ScrubPreview from './ScrubPreview.svelte'
  import TagDots from './TagDots.svelte'
  import TagPicker from './TagPicker.svelte'

  interface Props {
    video: VideoEntry
    selectable?: boolean
    selected?: boolean
    onToggleSelect?: (id: string) => void
  }

  let { video, selectable = false, selected = false, onToggleSelect }: Props = $props()

  let thumbLoaded = $state(false)
  let incompatible = $derived(isLikelyIncompatible(video.name, video))
  let localTags = $state<Tag[]>(video.tags ?? [])
  let pickerOpen = $state(false)
</script>

<div class="video-row" class:selected>
  {#if selectable}
    <button
      type="button"
      class="card-link select-toggle"
      aria-label={selected ? `Deselect ${video.name}` : `Select ${video.name}`}
      onclick={() => onToggleSelect?.(video.id)}
    ></button>
  {:else}
    <Link href={watchPath(video.id)} class="card-link" aria-label={video.name} />
  {/if}
  <div class="thumb" aria-hidden="true">
    <svg class="placeholder" viewBox="0 0 24 24" fill="none">
      <path d="M9 7.5v9l7.5-4.5z" fill="currentColor" />
    </svg>
    <img
      class="thumb-img"
      class:visible={thumbLoaded}
      src={thumbnailUrl(video.id)}
      loading="lazy"
      alt=""
      onload={() => (thumbLoaded = true)}
      onerror={() => (thumbLoaded = false)}
    />
    <ScrubPreview
      videoId={video.id}
      onActivate={selectable ? () => onToggleSelect?.(video.id) : () => router.navigate(watchPath(video.id))}
    />
    {#if video.duration != null}
      <span class="duration">{formatDuration(video.duration)}</span>
    {/if}
    <TagDots tags={localTags} max={2} />
    {#if selectable}
      <span class="select-check" class:checked={selected} aria-hidden="true">
        {#if selected}✓{/if}
      </span>
    {/if}
  </div>
  <div class="meta">
    <span class="name" title={video.name}>{truncateMiddle(video.name, 64)}</span>
    <span class="sub">
      {formatBytes(video.size)} · {formatRelativeTime(video.modified)}
      {#if video.width && video.height}
        · {video.width}×{video.height}
      {/if}
      {#if video.subfolder}
        · {video.subfolder}
      {/if}
    </span>
  </div>
  {#if incompatible}
    <span class="compat-badge" title="May not play on this device">⚠ may not play</span>
  {/if}
  {#if !selectable}
    <button
      type="button"
      class="tag-btn"
      aria-label="Edit tags"
      onclick={(e) => {
        e.preventDefault()
        e.stopPropagation()
        pickerOpen = !pickerOpen
      }}
    >
      <svg viewBox="0 0 24 24" fill="none" width="14" height="14">
        <path
          d="M11.5 3H6a2 2 0 0 0-2 2v5.5a1 1 0 0 0 .3.7l9 9a1 1 0 0 0 1.4 0l6.5-6.5a1 1 0 0 0 0-1.4l-9-9a1 1 0 0 0-.7-.3Z"
          stroke="currentColor"
          stroke-width="1.6"
          stroke-linejoin="round"
        />
        <circle cx="8" cy="8" r="1.3" fill="currentColor" />
      </svg>
    </button>
    {#if pickerOpen}
      <TagPicker
        videoIds={[video.id]}
        currentTags={localTags}
        onClose={() => (pickerOpen = false)}
        onAdd={(tag) => (localTags = [...localTags, tag])}
        onRemove={(tagId) => (localTags = localTags.filter((t) => t.id !== tagId))}
      />
    {/if}
  {/if}
</div>

<style>
  .video-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--border);
  }

  .video-row:hover {
    background: var(--bg-hover);
  }

  .video-row.selected {
    background: color-mix(in srgb, var(--accent) 12%, transparent);
  }

  :global(.video-row .card-link) {
    position: absolute;
    inset: 0;
    z-index: 1;
    cursor: pointer;
  }

  .select-toggle {
    border: none;
    background: none;
    padding: 0;
  }

  .select-check {
    position: absolute;
    z-index: 3;
    right: 3px;
    top: 3px;
    width: 14px;
    height: 14px;
    border-radius: 50%;
    border: 1.5px solid rgba(255, 255, 255, 0.85);
    background: rgba(0, 0, 0, 0.35);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.55rem;
  }

  .select-check.checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  .tag-btn {
    position: relative;
    z-index: 4;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    border: none;
    background: transparent;
    color: var(--text-tertiary);
    cursor: pointer;
  }

  .tag-btn:hover {
    background: var(--bg-hover);
    color: var(--text);
  }

  .thumb {
    position: relative;
    flex-shrink: 0;
    width: 96px;
    aspect-ratio: 16 / 9;
    border-radius: 6px;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    background: color-mix(in srgb, var(--bg-hover) 60%, transparent);
    color: var(--text-tertiary);
  }

  .placeholder {
    width: 22px;
    height: 22px;
  }

  .thumb-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    opacity: 0;
    transition: opacity 0.15s ease;
  }

  .thumb-img.visible {
    opacity: 1;
  }

  .duration {
    position: absolute;
    z-index: 3;
    right: 4px;
    bottom: 4px;
    padding: 0 4px;
    border-radius: 3px;
    background: rgba(0, 0, 0, 0.75);
    color: #fff;
    font-size: 0.65rem;
    font-variant-numeric: tabular-nums;
  }

  .meta {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .name {
    font-size: 0.88rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 0.75rem;
    color: var(--text-tertiary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .compat-badge {
    position: relative;
    z-index: 2;
    margin-left: auto;
    flex-shrink: 0;
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--warning-bg);
    color: var(--warning);
    font-size: 0.7rem;
    white-space: nowrap;
  }
</style>
