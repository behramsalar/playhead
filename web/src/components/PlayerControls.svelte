<script lang="ts">
  // The button row of the custom player chrome: play/pause, time, volume,
  // fullscreen. Paired with TimelineScrubber (the seek+preview bar) above
  // it — together they replace the native <video controls> UI so there's
  // one timeline, not two (see ARCHITECTURE.md).
  //
  // Volume: iOS (Safari *and* Firefox, which is required by Apple to run
  // on the same WebKit engine there) disables the HTMLMediaElement.volume
  // setter entirely, so a slider would silently do nothing. A mute/unmute
  // toggle still works everywhere, including iOS, so touch-primary
  // devices (detected via `(pointer: coarse)`, not brittle UA sniffing)
  // get just the toggle; anything with a precise pointer gets a slider.
  // Hardware volume buttons are OS-level and unaffected either way.
  import { formatDuration } from '../lib/format'

  interface Props {
    videoEl: HTMLVideoElement | undefined
    paused: boolean
    muted: boolean
    volume: number
    currentTime: number
    duration: number
    isFullscreen: boolean
    onToggleFullscreen: () => void
  }

  let { videoEl, paused, muted, volume, currentTime, duration, isFullscreen, onToggleFullscreen }: Props = $props()

  let isCoarsePointer = $state(false)
  $effect(() => {
    const mq = window.matchMedia('(pointer: coarse)')
    isCoarsePointer = mq.matches
    const onChange = (e: MediaQueryListEvent) => (isCoarsePointer = e.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  })

  // Blur after activating: otherwise focus lingers on the button and a
  // following spacebar press (the page-wide play/pause shortcut) also
  // re-activates whichever button focus was left on, a jarring
  // double-trigger the moment you touch any control with a mouse.
  function togglePlay(e: MouseEvent) {
    ;(e.currentTarget as HTMLElement).blur()
    if (!videoEl) return
    if (videoEl.paused) videoEl.play()
    else videoEl.pause()
  }

  function toggleMute(e: MouseEvent) {
    ;(e.currentTarget as HTMLElement).blur()
    if (!videoEl) return
    videoEl.muted = !videoEl.muted
  }

  function onVolumeInput(e: Event) {
    if (!videoEl) return
    const value = Number((e.currentTarget as HTMLInputElement).value)
    videoEl.volume = value
    videoEl.muted = value === 0
  }

  function handleFullscreenClick(e: MouseEvent) {
    ;(e.currentTarget as HTMLElement).blur()
    onToggleFullscreen()
  }
</script>

<div class="controls">
  <button type="button" class="icon-btn" onclick={togglePlay} aria-label={paused ? 'Play' : 'Pause'}>
    {#if paused}
      <svg viewBox="0 0 24 24" fill="none"><path d="M8 5.5v13l11-6.5z" fill="currentColor" /></svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="none"
        ><path d="M7 5h3.5v14H7zM13.5 5H17v14h-3.5z" fill="currentColor" /></svg
      >
    {/if}
  </button>

  <span class="time">{formatDuration(currentTime)} / {formatDuration(duration)}</span>

  <div class="spacer"></div>

  <button type="button" class="icon-btn" onclick={toggleMute} aria-label={muted || volume === 0 ? 'Unmute' : 'Mute'}>
    {#if muted || volume === 0}
      <svg viewBox="0 0 24 24" fill="none"
        ><path
          d="M4 9v6h4l5 5V4L8 9zm13.5 3 2.5-2.5-1-1L16.5 11 14 8.5l-1 1L15.5 12 13 14.5l1 1L16.5 13l2.5 2.5 1-1z"
          fill="currentColor"
        /></svg
      >
    {:else}
      <svg viewBox="0 0 24 24" fill="none"
        ><path
          d="M4 9v6h4l5 5V4L8 9zm12 3a4 4 0 0 0-2.5-3.7v7.4A4 4 0 0 0 16 12z"
          fill="currentColor"
        /></svg
      >
    {/if}
  </button>

  {#if !isCoarsePointer}
    <input
      class="volume-slider"
      type="range"
      min="0"
      max="1"
      step="0.05"
      value={muted ? 0 : volume}
      oninput={onVolumeInput}
      aria-label="Volume"
    />
  {/if}

  <button
    type="button"
    class="icon-btn"
    onclick={handleFullscreenClick}
    aria-label={isFullscreen ? 'Exit fullscreen' : 'Fullscreen'}
  >
    {#if isFullscreen}
      <svg viewBox="0 0 24 24" fill="none"
        ><path
          d="M9 15H4v-2h7v7H9zm6 0v5h-2v-7h7v2zM9 9H4V7h5V2h2v7zm6 0V2h2v5h5v2h-7z"
          fill="currentColor"
        /></svg
      >
    {:else}
      <svg viewBox="0 0 24 24" fill="none"
        ><path
          d="M4 9V4h5v2H6v3zm14 0V6h-3V4h5v5zM4 15v5h5v-2H6v-3zm14 0v3h-3v2h5v-5z"
          fill="currentColor"
        /></svg
      >
    {/if}
  </button>
</div>

<style>
  .controls {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 12px 10px;
    background: var(--bg-elevated);
  }

  .icon-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    padding: 0;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }

  .icon-btn:hover {
    background: var(--bg-hover);
  }

  .icon-btn svg {
    width: 20px;
    height: 20px;
  }

  .time {
    font-size: 0.8rem;
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    padding: 0 4px;
  }

  .spacer {
    flex: 1;
  }

  .volume-slider {
    width: 80px;
    accent-color: var(--accent);
  }
</style>
