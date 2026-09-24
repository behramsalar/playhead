<script lang="ts">
  // The player's only seek bar — native <video controls> is turned off
  // (Watch.svelte) specifically so there's one timeline, not two. Sits
  // directly below the video, above PlayerControls' button row. Hover/drag
  // shows the matching sprite frame, YouTube-style, floating above the
  // pointer. Enhancement only: with no sprite yet, this still seeks
  // correctly, it just shows no preview thumbnail.
  import { getSpriteMeta, spriteUrl } from '../api/client'
  import type { SpriteMeta } from '../api/types'
  import { formatDuration } from '../lib/format'
  import { fractionFromClientX } from '../lib/pointer'
  import { frameForFraction, tileStyle } from '../lib/sprite'

  interface Props {
    videoId: string
    videoEl: HTMLVideoElement | undefined
    duration: number | undefined // fallback (from indexed metadata) until the element's own duration loads
    currentTime: number
  }

  let { videoId, videoEl, duration, currentTime }: Props = $props()

  let effectiveDuration = $derived(videoEl?.duration && isFinite(videoEl.duration) ? videoEl.duration : (duration ?? 0))
  let progress = $derived(effectiveDuration > 0 ? Math.min(1, currentTime / effectiveDuration) : 0)

  let meta = $state<SpriteMeta | null>(null)
  let metaRequested = false
  let hovering = $state(false)
  let previewFraction = $state(0)
  let previewStyle = $state('')
  let trackEl: HTMLDivElement | undefined

  // The metadata fetch is async: a hover that arrives and then holds
  // still can resolve after the last pointermove already ran with meta
  // still null. Remember the last position so we can render as soon as
  // the fetch lands (same fix as ScrubPreview.svelte, same reasoning).
  let lastClientX: number | null = null

  async function ensureMeta() {
    if (metaRequested) return
    metaRequested = true
    try {
      meta = await getSpriteMeta(videoId)
      if (hovering && lastClientX !== null) updatePreview(lastClientX)
    } catch {
      // No sprite yet — scrubbing still works, just without a preview.
    }
  }

  function fractionAt(clientX: number): number {
    if (!trackEl) return 0
    return fractionFromClientX(clientX, trackEl.getBoundingClientRect())
  }

  function updatePreview(clientX: number) {
    previewFraction = fractionAt(clientX)
    if (meta && meta.frames.length > 0) {
      const frame = frameForFraction(meta, previewFraction)
      previewStyle = tileStyle(meta, frame, spriteUrl(videoId))
    }
  }

  function seekTo(fraction: number) {
    if (videoEl && effectiveDuration > 0) {
      videoEl.currentTime = fraction * effectiveDuration
    }
  }

  let pointerDown = false

  function onPointerEnter() {
    ensureMeta()
    hovering = true
  }
  function onPointerLeave() {
    if (!pointerDown) hovering = false
  }
  function onPointerMove(e: PointerEvent) {
    hovering = true
    lastClientX = e.clientX
    updatePreview(e.clientX)
    // Seek live while dragging (mouse held down or touch still in contact),
    // not just on the initial press — otherwise dragging to a new position
    // and releasing silently leaves playback wherever the drag *started*.
    if (pointerDown) seekTo(previewFraction)
  }
  function onPointerDown(e: PointerEvent) {
    pointerDown = true
    hovering = true
    lastClientX = e.clientX
    trackEl?.setPointerCapture(e.pointerId)
    ensureMeta()
    updatePreview(e.clientX)
    seekTo(previewFraction)
  }
  function onPointerUp(e: PointerEvent) {
    pointerDown = false
    trackEl?.releasePointerCapture(e.pointerId)
  }
  function onPointerCancel() {
    pointerDown = false
    hovering = false
  }
</script>

<div class="scrubber">
  <div
    class="track"
    bind:this={trackEl}
    onpointerenter={onPointerEnter}
    onpointerleave={onPointerLeave}
    onpointermove={onPointerMove}
    onpointerdown={onPointerDown}
    onpointerup={onPointerUp}
    onpointercancel={onPointerCancel}
    role="slider"
    aria-label="Seek"
    aria-valuemin="0"
    aria-valuemax={Math.round(effectiveDuration)}
    aria-valuenow={Math.round(currentTime)}
    tabindex="0"
  >
    <div class="bar">
      <div class="fill" style="width: {progress * 100}%"></div>
      <div class="playhead" style="left: {progress * 100}%"></div>
    </div>

    {#if hovering}
      <div class="preview" style="left: {previewFraction * 100}%">
        {#if meta}
          <div class="preview-frame" style={previewStyle}></div>
        {/if}
        <div class="preview-time">{formatDuration(previewFraction * effectiveDuration)}</div>
      </div>
    {/if}
  </div>
</div>

<style>
  .scrubber {
    padding: 2px 16px 0;
    background: var(--bg-elevated);
  }

  .track {
    /* Taller than the visible bar so the touch/click hit area is generous
       (a 6px-tall drag target is unusable on a phone) without thickening
       the bar itself. */
    position: relative;
    display: flex;
    align-items: center;
    height: 28px;
    cursor: pointer;
    touch-action: none;
  }

  .bar {
    position: relative;
    width: 100%;
    height: 6px;
    border-radius: 999px;
    background: var(--bg-hover);
  }

  .fill {
    position: absolute;
    inset: 0 auto 0 0;
    border-radius: 999px;
    background: var(--accent);
    pointer-events: none;
  }

  .playhead {
    position: absolute;
    top: 50%;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--accent);
    transform: translate(-50%, -50%);
    pointer-events: none;
  }

  .preview {
    position: absolute;
    bottom: calc(100% + 8px);
    transform: translateX(-50%);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    pointer-events: none;
  }

  .preview-frame {
    width: 140px;
    aspect-ratio: 16 / 9;
    border-radius: 6px;
    border: 2px solid var(--bg);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.4);
  }

  .preview-time {
    padding: 1px 6px;
    border-radius: 4px;
    background: rgba(0, 0, 0, 0.8);
    color: #fff;
    font-size: 0.7rem;
    font-variant-numeric: tabular-nums;
  }
</style>
