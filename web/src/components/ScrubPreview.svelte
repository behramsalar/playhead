<script lang="ts">
  // Layered over a video card's thumbnail. Desktop: hover tracks pointer
  // x-position and shows the matching sprite frame. Mobile: dragging
  // horizontally does the same, without scrolling the page or navigating;
  // a plain tap (no meaningful drag) falls through to the card's own
  // Link and opens the video normally. Enhancement only — if the sprite
  // fails to load or doesn't exist yet, this silently does nothing and
  // the static thumbnail underneath just keeps showing.
  import { getSpriteMeta, spriteUrl } from '../api/client'
  import type { SpriteMeta } from '../api/types'
  import { fractionFromClientX } from '../lib/pointer'
  import { frameForFraction, tileStyle } from '../lib/sprite'

  interface Props {
    videoId: string
    // This overlay sits on top of the thumbnail (it has to, to capture
    // hover/touch for scrubbing) — which means it, not the card's own
    // whole-card link underneath, is what actually receives a click
    // anywhere on the thumbnail. It must perform navigation/selection
    // itself, or a click on the thumbnail silently does nothing.
    onActivate?: () => void
  }

  let { videoId, onActivate }: Props = $props()

  let meta = $state<SpriteMeta | null>(null)
  let metaRequested = false
  let active = $state(false)
  let frameStyle = $state('')

  let containerEl: HTMLDivElement | undefined

  // Touch-drag gesture tracking. Plain (non-reactive) fields: read/written
  // only from within these handlers, never rendered.
  let touchStartX = 0
  let touchStartY = 0
  let dragging = false
  const DRAG_THRESHOLD_PX = 10

  // The metadata fetch is async, so a hover that starts and then holds
  // still (mouseenter + one mousemove, no further movement) can resolve
  // after the last mousemove already ran and bailed with meta still
  // null — remembering the last pointer position lets us render as soon
  // as the fetch lands, instead of waiting for movement that may never
  // come. Mouse-hover only: on touch this would risk flashing a preview
  // frame mid-tap, before a plain tap vs. a drag has even been decided.
  let lastClientX: number | null = null
  let stillHoveringMouse = false

  async function ensureMeta() {
    if (metaRequested) return
    metaRequested = true
    try {
      meta = await getSpriteMeta(videoId)
      if (stillHoveringMouse && lastClientX !== null) showFrameAt(lastClientX)
    } catch {
      // No sprite yet (still generating, or generation failed) — leave
      // meta null, and every handler below just no-ops.
    }
  }

  function showFrameAt(clientX: number) {
    if (!meta || !containerEl || meta.frames.length === 0) return
    const rect = containerEl.getBoundingClientRect()
    const frame = frameForFraction(meta, fractionFromClientX(clientX, rect))
    frameStyle = tileStyle(meta, frame, spriteUrl(videoId))
    active = true
  }

  function hide() {
    active = false
  }

  function onMouseEnter() {
    stillHoveringMouse = true
    ensureMeta()
  }
  function onMouseMove(e: MouseEvent) {
    lastClientX = e.clientX
    if (meta) showFrameAt(e.clientX)
  }
  function onMouseLeave() {
    stillHoveringMouse = false
    hide()
  }

  function onTouchStart(e: TouchEvent) {
    const t = e.touches[0]
    if (!t) return
    touchStartX = t.clientX
    touchStartY = t.clientY
    dragging = false
    ensureMeta()
  }

  function onTouchMove(e: TouchEvent) {
    const t = e.touches[0]
    if (!t) return
    const dx = t.clientX - touchStartX
    const dy = t.clientY - touchStartY

    if (!dragging) {
      // Not yet committed to a gesture: wait for clear horizontal intent
      // before deciding this is a scrub rather than a vertical page
      // scroll or an undecided tap.
      if (Math.abs(dx) < DRAG_THRESHOLD_PX || Math.abs(dx) <= Math.abs(dy)) return
      dragging = true
    }

    e.preventDefault() // committed to scrubbing: don't let the page scroll
    lastClientX = t.clientX
    if (meta) showFrameAt(t.clientX)
  }

  function onTouchEnd(e: TouchEvent) {
    if (dragging) {
      e.preventDefault() // suppress the synthetic click so this doesn't also navigate
      hide()
    }
    dragging = false
  }

  // Mouse clicks land here directly; a plain tap (no drag) falls through
  // to the browser's synthetic post-touchend click, which also lands
  // here — a dragged scrub gesture already had that synthetic click
  // suppressed above, so this never double-fires or fires after a drag.
  function onClick() {
    onActivate?.()
  }

  // touchmove/touchend must call preventDefault() to block page-scroll
  // during a drag and to suppress the tap-navigation click afterward.
  // Svelte's template event attributes (ontouchmove={...}) get registered
  // through its event delegation, which lands on a passive listener for
  // touch events — preventDefault() there is silently a no-op (confirmed
  // via the browser's own console warning). Attaching directly with
  // {passive: false} is the only reliable way to make it work.
  $effect(() => {
    const el = containerEl
    if (!el) return
    el.addEventListener('touchmove', onTouchMove, { passive: false })
    el.addEventListener('touchend', onTouchEnd, { passive: false })
    return () => {
      el.removeEventListener('touchmove', onTouchMove)
      el.removeEventListener('touchend', onTouchEnd)
    }
  })
</script>

<div
  class="scrub-layer"
  bind:this={containerEl}
  onmouseenter={onMouseEnter}
  onmousemove={onMouseMove}
  onmouseleave={onMouseLeave}
  ontouchstart={onTouchStart}
  onclick={onClick}
  role="presentation"
>
  {#if active && meta}
    <div class="frame" style={frameStyle}></div>
  {/if}
</div>

<style>
  .scrub-layer {
    position: absolute;
    inset: 0;
    z-index: 2;
    cursor: pointer;
    touch-action: pan-y; /* let vertical page scroll through until we commit to a horizontal drag */
  }

  .frame {
    position: absolute;
    inset: 0;
  }
</style>
