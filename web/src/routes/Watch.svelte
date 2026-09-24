<script lang="ts">
  import { ApiError, clearResumePosition, getVideo, saveResumePosition, streamUrl } from '../api/client'
  import type { Tag, VideoInfo } from '../api/types'
  import Link from '../components/Link.svelte'
  import PlayerControls from '../components/PlayerControls.svelte'
  import StateMessage from '../components/StateMessage.svelte'
  import TagPicker from '../components/TagPicker.svelte'
  import TimelineScrubber from '../components/TimelineScrubber.svelte'
  import { formatDuration } from '../lib/format'
  import { browsePath } from '../lib/router.svelte'

  interface Props {
    id: string
  }

  let { id }: Props = $props()

  let video = $state<VideoInfo | null>(null)
  let tags = $state<Tag[]>([])
  let tagPickerOpen = $state(false)
  let loadError = $state<ApiError | Error | null>(null)
  let playbackError = $state<string | null>(null)
  let videoEl = $state<HTMLVideoElement | undefined>(undefined)
  let playerEl = $state<HTMLDivElement | undefined>(undefined)
  let currentTime = $state(0)
  let paused = $state(true)
  let muted = $state(false)
  let volume = $state(1)
  let isFullscreen = $state(false)

  // A saved position only worth offering if it's not near the very start
  // (nothing to resume) or the very end (it was basically finished).
  let resumeOffer = $state<number | null>(null)
  const RESUME_MIN_SECONDS = 5
  const RESUME_END_CUSHION = 8
  // Throttles periodic saving during playback — see onTimeUpdate.
  const SAVE_INTERVAL_SECONDS = 10
  let lastSavedAt = 0

  $effect(() => {
    const currentId = id
    let cancelled = false

    video = null
    loadError = null
    playbackError = null
    resumeOffer = null
    lastSavedAt = 0

    getVideo(currentId)
      .then((v) => {
        if (cancelled) return
        video = v
        tags = v.tags ?? []
        if (
          v.resumeSeconds != null &&
          v.resumeSeconds > RESUME_MIN_SECONDS &&
          (v.duration == null || v.resumeSeconds < v.duration - RESUME_END_CUSHION)
        ) {
          resumeOffer = v.resumeSeconds
        }
      })
      .catch((e) => {
        if (!cancelled) loadError = e
      })

    return () => {
      cancelled = true
      flushSave()
    }
  })

  function flushSave() {
    if (!videoEl || !video || resumeOffer != null) return
    const t = videoEl.currentTime
    if (t < RESUME_MIN_SECONDS) return
    // Fire-and-forget: nothing meaningful to do with a failure here, and
    // nothing to await through (component teardown / page unload).
    saveResumePosition(video.id, t).catch(() => {})
  }

  $effect(() => {
    // Best-effort save on an actual tab close/reload, not just in-app
    // navigation (which the effect cleanup above already covers).
    window.addEventListener('beforeunload', flushSave)
    return () => window.removeEventListener('beforeunload', flushSave)
  })

  function acceptResume() {
    if (videoEl && resumeOffer != null) videoEl.currentTime = resumeOffer
    resumeOffer = null
  }

  function declineResume() {
    resumeOffer = null
  }

  function onVideoError() {
    playbackError = "This file can't be played in this browser (likely an unsupported codec or container)."
  }

  function onTimeUpdate() {
    if (!videoEl) return
    currentTime = videoEl.currentTime
    if (!video || resumeOffer != null) return
    if (currentTime - lastSavedAt >= SAVE_INTERVAL_SECONDS) {
      lastSavedAt = currentTime
      saveResumePosition(video.id, currentTime).catch(() => {})
    }
  }

  function onEnded() {
    // A finished video shouldn't prompt to "resume" from near the end on
    // a rewatch.
    if (video) clearResumePosition(video.id).catch(() => {})
  }

  function onPlay() {
    paused = false
  }
  function onPause() {
    paused = true
  }
  function onVolumeChange() {
    if (!videoEl) return
    muted = videoEl.muted
    volume = videoEl.volume
  }

  // Spacebar play/pause and arrow-key seeking, skipped when a form control
  // has focus (none exist on this page today, but this is cheap insurance
  // against surprising a future one).
  function onKeydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement | null
    if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return
    if (!videoEl) return

    if (e.code === 'Space') {
      e.preventDefault()
      if (videoEl.paused) videoEl.play()
      else videoEl.pause()
    } else if (e.code === 'ArrowLeft') {
      e.preventDefault()
      videoEl.currentTime = Math.max(0, videoEl.currentTime - 5)
    } else if (e.code === 'ArrowRight') {
      e.preventDefault()
      const max = isFinite(videoEl.duration) ? videoEl.duration : Infinity
      videoEl.currentTime = Math.min(max, videoEl.currentTime + 5)
    }
  }

  $effect(() => {
    window.addEventListener('keydown', onKeydown)
    return () => window.removeEventListener('keydown', onKeydown)
  })

  $effect(() => {
    const onFullscreenChange = () => {
      isFullscreen = document.fullscreenElement === playerEl
      // Leaving fullscreen: the controls are laid out inline again (not an
      // overlay — see .bottom-controls below), so there's no "obstructing
      // the video" problem there and nothing should stay hidden.
      if (!isFullscreen) controlsVisible = true
    }
    document.addEventListener('fullscreenchange', onFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', onFullscreenChange)
  })

  // Auto-hide the timeline/controls in fullscreen after a few seconds of
  // no interaction — without this they sit permanently on top of the
  // video (see .bottom-controls' fullscreen positioning below), which on
  // a phone in landscape is exactly the "controls never go away" report
  // this was built to fix. Windowed playback is untouched: there the
  // controls are laid out below the video, not overlaying it, so there's
  // nothing to hide.
  let controlsVisible = $state(true)
  let hideTimer: ReturnType<typeof setTimeout> | undefined
  const INACTIVITY_HIDE_MS = 3000

  function scheduleHide() {
    clearTimeout(hideTimer)
    // Only auto-hide while actually playing in fullscreen — paused (or
    // windowed) always keeps them visible, matching how every other
    // player leaves you an obvious way to resume rather than hiding the
    // only control that gets you unstuck.
    if (!isFullscreen || paused) return
    hideTimer = setTimeout(() => {
      controlsVisible = false
    }, INACTIVITY_HIDE_MS)
  }

  function onPlayerActivity() {
    controlsVisible = true
    scheduleHide()
  }

  $effect(() => {
    // Re-arm whenever fullscreen or paused state changes (e.g. entering
    // fullscreen already mid-playback, or resuming from pause) — not just
    // on the next pointer move.
    isFullscreen
    paused
    onPlayerActivity()
  })

  $effect(() => () => clearTimeout(hideTimer))

  function toggleFullscreen() {
    if (document.fullscreenElement) {
      document.exitFullscreen()
      return
    }
    if (playerEl?.requestFullscreen) {
      playerEl.requestFullscreen()
    } else if (videoEl && 'webkitEnterFullscreen' in videoEl) {
      // iOS (Safari and Firefox alike, both required by Apple to run on
      // WebKit) has no arbitrary-element Fullscreen API — only the native
      // <video> can go fullscreen, taking over with its own native
      // controls until the user backs out of it.
      ;(videoEl as HTMLVideoElement & { webkitEnterFullscreen: () => void }).webkitEnterFullscreen()
    }
  }

  function onPlayerClick(e: MouseEvent) {
    if (e.target === videoEl && videoEl) {
      if (videoEl.paused) videoEl.play()
      else videoEl.pause()
    }
  }

  let backHref = $derived(video ? browsePath(video.root, video.folderPath) : '/')
  let effectiveDuration = $derived(
    videoEl?.duration && isFinite(videoEl.duration) ? videoEl.duration : (video?.duration ?? 0),
  )
</script>

{#if loadError}
  {#if loadError instanceof ApiError && loadError.status === 404}
    <StateMessage title="Video not found" detail="This video doesn't exist or was removed." />
  {:else}
    <StateMessage title="Couldn't load this video" detail={loadError.message} />
  {/if}
{:else if video}
  <div class="watch">
    <div class="bar">
      <Link href={backHref} class="back">&larr; Back to folder</Link>
      <h1 class="filename" title={video.name}>{video.name}</h1>
      <div class="tags-row">
        {#each tags as tag (tag.id)}
          <span class="tag-chip" style:background={tag.color}>{tag.name}</span>
        {/each}
        <div class="tag-editor">
          <button type="button" class="add-tag" onclick={() => (tagPickerOpen = !tagPickerOpen)}>
            + Tag
          </button>
          {#if tagPickerOpen}
            <TagPicker
              videoIds={[video.id]}
              currentTags={tags}
              align="start"
              onClose={() => (tagPickerOpen = false)}
              onAdd={(tag) => (tags = [...tags, tag])}
              onRemove={(tagId) => (tags = tags.filter((t) => t.id !== tagId))}
            />
          {/if}
        </div>
      </div>
    </div>

    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div
      class="player"
      class:hide-cursor={isFullscreen && !controlsVisible}
      bind:this={playerEl}
      onpointermove={onPlayerActivity}
      ontouchstart={onPlayerActivity}
    >
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="video-wrap" onclick={onPlayerClick}>
        <!-- svelte-ignore a11y_media_has_caption -->
        <video
          bind:this={videoEl}
          playsinline
          preload="metadata"
          src={streamUrl(video.id)}
          onerror={onVideoError}
          ontimeupdate={onTimeUpdate}
          onplay={onPlay}
          onpause={onPause}
          onvolumechange={onVolumeChange}
          onended={onEnded}
        ></video>
        {#if playbackError}
          <div class="playback-error">
            <StateMessage title="Playback failed" detail={playbackError}>
              <Link href={backHref} class="back-link">Back to folder</Link>
            </StateMessage>
          </div>
        {:else if resumeOffer != null}
          <div class="resume-banner">
            <span>Resume from {formatDuration(resumeOffer)}?</span>
            <div class="resume-actions">
              <button type="button" onclick={acceptResume}>Resume</button>
              <button type="button" class="secondary" onclick={declineResume}>Start over</button>
            </div>
          </div>
        {/if}
      </div>

      {#if !playbackError}
        <div class="bottom-controls" class:hidden={isFullscreen && !controlsVisible}>
          <TimelineScrubber videoId={video.id} {videoEl} duration={video.duration} {currentTime} />
          <PlayerControls
            {videoEl}
            {paused}
            {muted}
            {volume}
            {currentTime}
            duration={effectiveDuration}
            {isFullscreen}
            onToggleFullscreen={toggleFullscreen}
          />
        </div>
      {/if}
    </div>
  </div>
{:else}
  <StateMessage title="Loading…" />
{/if}

<style>
  .watch {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  .bar {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 14px 16px;
    border-bottom: 1px solid var(--border);
  }

  :global(.back) {
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  :global(.back:hover) {
    color: var(--text);
  }

  .filename {
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
    overflow-wrap: break-word;
  }

  .tags-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin-top: 2px;
  }

  .tag-chip {
    padding: 2px 8px;
    border-radius: 999px;
    color: #fff;
    font-size: 0.72rem;
    font-weight: 500;
  }

  .tag-editor {
    position: relative;
  }

  .add-tag {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px dashed var(--border);
    background: transparent;
    color: var(--text-tertiary);
    font-size: 0.72rem;
    cursor: pointer;
  }

  .add-tag:hover {
    color: var(--text);
    border-color: var(--text-tertiary);
  }

  .player {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    background: black;
  }

  .player:fullscreen {
    background: black;
  }

  .video-wrap {
    position: relative;
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 0;
  }

  /* Windowed: a normal flex child below the video, always visible — never
     obstructs anything, so there's nothing to auto-hide. Fullscreen: an
     overlay pinned to the bottom of the video instead (so hiding it
     actually gives the video the space back), on a gradient so it stays
     legible over bright content, and it's the thing .hidden below fades
     out after inactivity. */
  .player:fullscreen .bottom-controls {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 2;
    background: linear-gradient(to top, rgba(0, 0, 0, 0.65), transparent);
    transition:
      opacity 0.25s ease,
      visibility 0.25s ease;
  }

  .player:fullscreen .bottom-controls.hidden {
    opacity: 0;
    visibility: hidden;
    /* Still transitions out smoothly (not display:none, which would cut
       the fade short), but stops swallowing taps once invisible so a tap
       reaches the video underneath to bring the controls back rather
       than being absorbed by an invisible control still sitting on top. */
    pointer-events: none;
  }

  .player:fullscreen.hide-cursor {
    cursor: none;
  }

  /* width/height 100% of .video-wrap's actual flex-computed space (bounded
     by the viewport via #app's fixed height — see app.css), with
     object-fit doing the letterboxing — not a max-height guess (80vh)
     that didn't account for the filename bar/timeline/controls also
     taking vertical space, which could push controls below the fold
     entirely on a shorter viewport. */
  video {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }

  .playback-error {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
  }

  .resume-banner {
    position: absolute;
    left: 50%;
    bottom: 16px;
    transform: translateX(-50%);
    z-index: 2;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 14px;
    border-radius: var(--radius);
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .resume-actions {
    display: flex;
    gap: 8px;
  }

  .resume-actions button {
    padding: 5px 10px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--accent);
    color: white;
    font-size: 0.8rem;
    cursor: pointer;
  }

  .resume-actions button.secondary {
    background: transparent;
    color: var(--text-secondary);
  }

  :global(.back-link) {
    display: inline-block;
    margin-top: 8px;
    color: var(--accent);
    font-weight: 500;
  }
</style>
