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

<div class="video-card" class:selected>
  {#if selectable}
    <!-- Selection mode replaces navigation entirely (not a link Link
         forwards onclick through internally, so composing "navigate or
         toggle selection" isn't possible via a shared onclick prop). -->
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
    {#if incompatible}
      <span class="compat-badge" title="May not play on this device">⚠</span>
    {/if}
    <TagDots tags={localTags} />
    {#if selectable}
      <span class="select-check" class:checked={selected} aria-hidden="true">
        {#if selected}✓{/if}
      </span>
    {/if}
  </div>
  <div class="meta">
    <span class="name" title={video.name}>{truncateMiddle(video.name)}</span>
    <span class="sub">
      {formatBytes(video.size)} · {formatRelativeTime(video.modified)}
      {#if video.width && video.height}
        · {video.width}×{video.height}
      {/if}
    </span>
    {#if video.subfolder}
      <span class="subfolder" title={video.subfolder}>{video.subfolder}</span>
    {/if}
  </div>

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
  .video-card {
    position: relative;
    display: flex;
    flex-direction: column;
    border-radius: var(--radius);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
  }

  .video-card:hover {
    background: var(--bg-hover);
  }

  .video-card.selected {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }

  /* The Link/button that makes the whole card clickable, layered under
     the tag button and picker so those stay independently clickable —
     see the script comment on why selection mode can't share one onclick
     with Link's own navigation handler. */
  :global(.video-card .card-link) {
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
    right: 6px;
    top: 6px;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 1.5px solid rgba(255, 255, 255, 0.85);
    background: rgba(0, 0, 0, 0.35);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.65rem;
  }

  .select-check.checked {
    background: var(--accent);
    border-color: var(--accent);
  }

  /* Always visible, not hover-only — this app has no critical
     hover-only controls, since hover doesn't meaningfully exist on
     mobile — kept small and subtle instead. */
  .tag-btn {
    position: absolute;
    z-index: 4;
    top: 6px;
    right: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    border: none;
    background: rgba(0, 0, 0, 0.55);
    color: #fff;
    cursor: pointer;
  }

  .tag-btn:hover {
    background: rgba(0, 0, 0, 0.8);
  }

  .thumb {
    position: relative;
    width: 100%;
    aspect-ratio: 16 / 9;
    display: flex;
    align-items: center;
    justify-content: center;
    background: color-mix(in srgb, var(--bg-hover) 60%, transparent);
    color: var(--text-tertiary);
    overflow: hidden;
    /* .video-card itself can no longer clip to its own rounded corners
       (it needs to let the tag picker popover escape below it), so the
       thumb rounds its own top corners to match instead. */
    border-radius: var(--radius) var(--radius) 0 0;
  }

  .placeholder {
    width: 36px;
    height: 36px;
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
    right: 6px;
    bottom: 6px;
    padding: 1px 5px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.75);
    color: #fff;
    font-size: 0.7rem;
    font-variant-numeric: tabular-nums;
  }

  .compat-badge {
    position: absolute;
    z-index: 3;
    left: 6px;
    top: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    background: var(--warning-bg);
    color: var(--warning);
    font-size: 0.7rem;
    line-height: 1;
  }

  .meta {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 10px 10px;
  }

  .name {
    font-size: 0.85rem;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 0.75rem;
    color: var(--text-tertiary);
  }

  .subfolder {
    font-size: 0.7rem;
    color: var(--accent);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
