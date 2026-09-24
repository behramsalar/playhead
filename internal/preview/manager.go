// Package preview generates and serves static thumbnails, preview sprite
// sheets, and container-only remuxes, with a small bounded worker pool
// shared between all three, so generation never competes unboundedly with
// an actively streaming video and never blocks a browse request.
// Thumbnails (Phase 3), sprites (Phase 4), and remuxes (Phase 6) share one
// Manager and one semaphore deliberately: independent pools would defeat
// the point of bounding total FFmpeg concurrency (see ARCHITECTURE.md).
package preview

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/filesystem"
	"playhead/internal/media"
)

// requestWaitTimeout bounds how long a single HTTP request will wait for
// generation to finish before falling back to "not ready yet" (the caller
// can retry on a later request/reload).
const requestWaitTimeout = 6 * time.Second

// Per-job FFmpeg timeouts. Every job previously ran under m.baseCtx
// directly — the app's whole-lifetime context, which only ever cancels on
// shutdown — so a single stalled FFmpeg invocation (a corrupt file, a
// pathological seek, a stall on whatever the media is stored on) could
// hold one of the worker pool's few slots indefinitely, with nothing to
// intervene. With only a couple of workers total, even one stuck job
// backs up everything queued behind it, which reads as "generation
// became extremely slow" even though most individual jobs are still
// fine. These bound each job's total wall-clock time; a job that exceeds
// its budget is killed (exec.CommandContext) and recorded as a normal
// failure through the same error path as any other failure, eligible for
// the automatic retry sweep below like any other error. Sized generously
// — normal generation is seconds, not minutes — specifically so a
// slow-but-fine large file is never wrongly killed:
//   - thumbnailJobTimeout: one single-frame extraction (plus GenerateWithFallback's
//     one retry attempt at most).
//   - spriteJobTimeout: up to ~150 sequential single-frame extractions
//     plus one montage assembly pass (see ComputeSpriteLayout's frame cap).
//   - remuxJobTimeout: a `-c copy` container remux — no decode/encode
//     work, but still proportional to file size for a very large file.
//
// var, not const: TestJobTimeoutKillsHungFFmpeg overrides these to a few
// milliseconds around a deliberately slow command, rather than a real
// test waiting out a multi-minute timeout to prove the kill happens.
var (
	thumbnailJobTimeout = 5 * time.Minute
	spriteJobTimeout    = 20 * time.Minute
	remuxJobTimeout     = 30 * time.Minute
)

// maxAutoRetries bounds the post-scan retry sweep (RequeueFailedForRetry):
// a video whose thumbnail/sprite/remux has failed this many times without
// an intervening success is left in its error state rather than requeued
// again on every future scan — still fixable with an explicit per-video
// retry (which resets its counter), just no longer retried silently
// forever for what's presumably a genuinely broken file rather than a
// transient/environmental problem.
const maxAutoRetries = 3

// SetThumbnailJobTimeoutForTesting overrides thumbnailJobTimeout for the
// life of a test; the caller must invoke the returned restore func (e.g.
// via t.Cleanup) — same cross-package test-seam pattern as
// internal/config's SetDeviceIDFuncForTesting, used here because a real
// multi-minute timeout has no place in a test run, but the timeout being
// wired through to the FFmpeg subprocess's context is exactly the
// behavior worth verifying (see TestJobTimeoutKillsHungGeneration).
func SetThumbnailJobTimeoutForTesting(d time.Duration) (restore func()) {
	orig := thumbnailJobTimeout
	thumbnailJobTimeout = d
	return func() { thumbnailJobTimeout = orig }
}

// Status is a snapshot of coverage across the whole index, for either
// thumbnails or sprites.
type Status struct {
	Ready         int `json:"ready"`
	Pending       int `json:"pending"`
	Errored       int `json:"errored"`
	ActiveWorkers int `json:"activeWorkers"`
}

// Result is the outcome of a thumbnail request.
type Result struct {
	Path  string // absolute path to the cached WebP; valid only if Ready
	Ready bool
	// Failed for this request/right now — not retried on the spot. It may
	// still be retried automatically on a later scan (RequeueFailedForRetry,
	// bounded by maxAutoRetries) or explicitly via the per-video retry
	// endpoint, either of which resets it back to pending for the next
	// request to pick up.
	Failed bool
}

// SpriteResult is the outcome of a sprite request.
type SpriteResult struct {
	ImagePath string // valid only if Ready
	MetaPath  string // valid only if Ready
	Ready     bool
	Failed    bool // see Result.Failed
}

// RemuxResult is the outcome of a remux request — same shape as Result
// (a single cached file), kept as a distinct type since "the cached
// thumbnail" and "the cached remux" are different files, not overloading
// one type across two purposes.
type RemuxResult struct {
	Path   string // absolute path to the cached MP4; valid only if Ready
	Ready  bool
	Failed bool // see Result.Failed
}

// thumbJob identifies one video's thumbnail generation, with everything
// needed both to run ffmpeg and to persist the outcome without requiring
// the metadata indexer to have created a row first.
type thumbJob struct {
	videoID      string
	rootID       string
	relPath      string
	absVideoPath string
	thumbPath    string
	size         int64
	mtimeUnix    int64
}

// spriteJob is thumbJob's sprite-sheet equivalent.
type spriteJob struct {
	videoID      string
	rootID       string
	relPath      string
	absVideoPath string
	imagePath    string
	metaPath     string
	size         int64
	mtimeUnix    int64
}

// remuxJob is thumbJob's container-remux equivalent.
type remuxJob struct {
	videoID      string
	rootID       string
	relPath      string
	absVideoPath string
	remuxPath    string
	size         int64
	mtimeUnix    int64
}

// Manager owns the bounded worker pool and the on-disk preview cache.
type Manager struct {
	store *database.Store

	// rootsMu guards roots separately from mu (job/inflight bookkeeping)
	// since a Phase 7 settings-page rescan can replace the root list at
	// any time, independent of and much less frequent than job dispatch.
	rootsMu    sync.RWMutex
	roots      map[string]config.Root
	thumbsDir  string
	spritesDir string
	remuxDir   string
	sem        chan struct{}
	available  bool
	// baseCtx scopes generation jobs to the app's lifetime, not to
	// whichever HTTP request happened to trigger or wait on one — a job
	// started by one request must keep running for other waiters (or a
	// later request) even if the original requester disconnects.
	baseCtx context.Context

	// activeWorkers counts semaphore slots currently held (ffmpeg
	// actually running), not jobs merely queued — see Status.
	activeWorkers int32

	// cacheMaxBytes optionally caps thumbsDir+spritesDir's combined size;
	// 0 means unlimited (see EnforceCacheLimit in cache.go).
	cacheMaxBytes int64

	mu       sync.Mutex
	inflight map[string]chan struct{} // keyed "thumb:<id>" or "sprite:<id>"
}

// New builds a Manager. workers bounds concurrent FFmpeg processes across
// both thumbnails and sprites combined. cacheMaxBytes optionally caps the
// on-disk preview cache size (0 = unlimited); see EnforceCacheLimit.
func New(baseCtx context.Context, store *database.Store, roots []config.Root, dataDir string, workers int, cacheMaxBytes int64) *Manager {
	if workers < 1 {
		workers = 1
	}
	available := FFmpegAvailable()
	if !available {
		slog.Warn("ffmpeg not found on PATH; previews will not be generated")
	}

	m := &Manager{
		store:         store,
		thumbsDir:     filepath.Join(dataDir, "cache", "thumbs"),
		spritesDir:    filepath.Join(dataDir, "cache", "sprites"),
		remuxDir:      filepath.Join(dataDir, "cache", "remux"),
		sem:           make(chan struct{}, workers),
		available:     available,
		baseCtx:       baseCtx,
		inflight:      make(map[string]chan struct{}),
		cacheMaxBytes: cacheMaxBytes,
	}
	m.SetRoots(roots)
	return m
}

// SetRoots replaces the root list Pregenerate/PregenerateSprites/
// PregenerateRemux resolve video paths against (Phase 7: a settings-page
// rescan can change this at runtime). EnqueueFolder/Request et al. don't
// consult this — they're always handed a root directly by the caller
// (the API layer, which resolves it from its own, separately-guarded
// root map), so this only matters for the pregenerate/status paths that
// look a root up purely from a stored RootID.
func (m *Manager) SetRoots(roots []config.Root) {
	byID := make(map[string]config.Root, len(roots))
	for _, r := range roots {
		byID[r.ID] = r
	}
	m.rootsMu.Lock()
	m.roots = byID
	m.rootsMu.Unlock()
}

func (m *Manager) rootByID(id string) (config.Root, bool) {
	m.rootsMu.RLock()
	defer m.rootsMu.RUnlock()
	r, ok := m.roots[id]
	return r, ok
}

// ---- Thumbnails -----------------------------------------------------

// Request resolves (generating if necessary) the thumbnail for one video.
// If wait is true, it blocks up to requestWaitTimeout for an in-progress
// or newly-started generation to finish; if false, it ensures a job is
// queued and returns immediately with Ready=false — the caller (a folder
// listing) doesn't want to block on FFmpeg.
func (m *Manager) Request(ctx context.Context, videoID, rootID, relPath, absVideoPath string, wait bool) (Result, error) {
	if !m.available {
		return Result{Failed: true}, nil
	}

	info, err := os.Stat(absVideoPath)
	if err != nil {
		return Result{}, err
	}
	j := thumbJob{
		videoID:      videoID,
		rootID:       rootID,
		relPath:      relPath,
		absVideoPath: absVideoPath,
		thumbPath:    filepath.Join(m.thumbsDir, FileName(videoID, info.Size(), info.ModTime().Unix())),
		size:         info.Size(),
		mtimeUnix:    info.ModTime().Unix(),
	}

	if _, err := os.Stat(j.thumbPath); err == nil {
		return Result{Path: j.thumbPath, Ready: true}, nil
	}

	if status, _, ok, err := m.store.GetThumbStatus(ctx, videoID); err != nil {
		return Result{}, err
	} else if ok && status == "error" {
		return Result{Failed: true}, nil
	}

	done := m.ensureThumbJob(j)
	if !wait {
		return Result{Ready: false}, nil
	}

	select {
	case <-done:
		if _, err := os.Stat(j.thumbPath); err == nil {
			return Result{Path: j.thumbPath, Ready: true}, nil
		}
		// Generation finished but produced no file: check whether it was
		// recorded as a permanent failure so the caller doesn't keep
		// waiting on it every time.
		if status, _, ok, err := m.store.GetThumbStatus(ctx, videoID); err == nil && ok && status == "error" {
			return Result{Failed: true}, nil
		}
		return Result{Ready: false}, nil
	case <-time.After(requestWaitTimeout):
		return Result{Ready: false}, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// EnqueueFolder starts background generation for every video in a folder
// listing that doesn't already have (or hasn't already failed) a
// thumbnail or sprite. It never blocks — this is the "lazy generation on
// folder view" behavior.
func (m *Manager) EnqueueFolder(ctx context.Context, root config.Root, videos []filesystem.VideoEntry) {
	if !m.available || len(videos) == 0 {
		return
	}

	ids := make([]string, len(videos))
	byID := make(map[string]filesystem.VideoEntry, len(videos))
	for i, v := range videos {
		ids[i] = v.ID
		byID[v.ID] = v
	}

	needThumb, err := m.store.FilterNeedingThumbnail(ctx, ids)
	if err != nil {
		slog.Error("checking thumbnail status failed", "error", err)
	}
	for _, id := range needThumb {
		v := byID[id]
		absPath, err := filesystem.Resolve(root.Path, v.Path)
		if err != nil {
			continue
		}
		m.ensureThumbJob(thumbJob{
			videoID:      v.ID,
			rootID:       root.ID,
			relPath:      v.Path,
			absVideoPath: absPath,
			thumbPath:    filepath.Join(m.thumbsDir, FileName(v.ID, v.Size, v.Modified.Unix())),
			size:         v.Size,
			mtimeUnix:    v.Modified.Unix(),
		})
	}

	needSprite, err := m.store.FilterNeedingSprite(ctx, ids)
	if err != nil {
		slog.Error("checking sprite status failed", "error", err)
		return
	}
	for _, id := range needSprite {
		v := byID[id]
		absPath, err := filesystem.Resolve(root.Path, v.Path)
		if err != nil {
			continue
		}
		hash := contentHash(v.ID, v.Size, v.Modified.Unix())
		m.ensureSpriteJob(spriteJob{
			videoID:      v.ID,
			rootID:       root.ID,
			relPath:      v.Path,
			absVideoPath: absPath,
			imagePath:    filepath.Join(m.spritesDir, spriteImageName(hash)),
			metaPath:     filepath.Join(m.spritesDir, spriteMetaName(hash)),
			size:         v.Size,
			mtimeUnix:    v.Modified.Unix(),
		})
	}

	// Remux eligibility depends on the video/audio codec, which (unlike
	// thumbnails/sprites) is only known once the indexer has probed the
	// file — a plain filesystem.VideoEntry doesn't carry it. A file not
	// indexed yet is simply skipped here; it'll be picked up on a later
	// folder view once indexing catches up, same "absent until indexed"
	// timing the codec-compatibility badge already has.
	metaByID, err := m.store.GetMetadataBatch(ctx, ids)
	if err != nil {
		slog.Error("fetching metadata for remux eligibility failed", "error", err)
		return
	}
	eligible := make([]string, 0, len(ids))
	for _, id := range ids {
		meta, ok := metaByID[id]
		if !ok || meta.Status != "ok" || meta.VideoCodec == nil || meta.AudioCodec == nil {
			continue
		}
		if NeedsRemux(byID[id].Name, *meta.VideoCodec, *meta.AudioCodec) {
			eligible = append(eligible, id)
		}
	}
	needRemux, err := m.store.FilterNeedingRemux(ctx, eligible)
	if err != nil {
		slog.Error("checking remux status failed", "error", err)
		return
	}
	for _, id := range needRemux {
		v := byID[id]
		absPath, err := filesystem.Resolve(root.Path, v.Path)
		if err != nil {
			continue
		}
		m.ensureRemuxJob(remuxJob{
			videoID:      v.ID,
			rootID:       root.ID,
			relPath:      v.Path,
			absVideoPath: absPath,
			remuxPath:    filepath.Join(m.remuxDir, remuxFileName(v.ID, v.Size, v.Modified.Unix())),
			size:         v.Size,
			mtimeUnix:    v.Modified.Unix(),
		})
	}
}

// RequeueFailedForRetry resets every thumbnail/sprite/remux still in
// 'error' status — up to maxAutoRetries prior automatic retries — back to
// 'pending', so the caller's next Pregenerate/PregenerateSprites call (or
// just the ordinary lazy per-folder path) picks them up like any other
// pending job. Meant to be called once per scan (see cmd/server/main.go's
// AfterScan), so a batch of failures caused by something transient or
// environmental — a permissions problem during deployment, a storage
// hiccup — self-heals on the next scan (in practice, often just the next
// container restart) instead of requiring a manual per-video retry click
// for every affected file. Returns the total number of jobs requeued,
// for logging.
func (m *Manager) RequeueFailedForRetry(ctx context.Context) (int, error) {
	if !m.available {
		return 0, nil
	}

	total := int64(0)
	n, err := m.store.RequeueRetryableThumbnails(ctx, maxAutoRetries)
	if err != nil {
		return int(total), fmt.Errorf("requeuing failed thumbnails: %w", err)
	}
	total += n

	n, err = m.store.RequeueRetryableSprites(ctx, maxAutoRetries)
	if err != nil {
		return int(total), fmt.Errorf("requeuing failed sprites: %w", err)
	}
	total += n

	n, err = m.store.RequeueRetryableRemux(ctx, maxAutoRetries)
	if err != nil {
		return int(total), fmt.Errorf("requeuing failed remuxes: %w", err)
	}
	total += n

	return int(total), nil
}

// Pregenerate starts background thumbnail generation for every pending
// video across every root — the "optional full pre-generation" action.
func (m *Manager) Pregenerate(ctx context.Context) (int, error) {
	if !m.available {
		return 0, nil
	}

	targets, err := m.store.ListPendingThumbnails(ctx)
	if err != nil {
		return 0, err
	}

	started := 0
	for _, t := range targets {
		root, ok := m.rootByID(t.RootID)
		if !ok {
			continue
		}
		absPath, err := filesystem.Resolve(root.Path, t.RelPath)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		m.ensureThumbJob(thumbJob{
			videoID:      t.ID,
			rootID:       t.RootID,
			relPath:      t.RelPath,
			absVideoPath: absPath,
			thumbPath:    filepath.Join(m.thumbsDir, FileName(t.ID, info.Size(), info.ModTime().Unix())),
			size:         info.Size(),
			mtimeUnix:    info.ModTime().Unix(),
		})
		started++
	}
	return started, nil
}

// Status reports current thumbnail coverage across the whole index.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	ok, errored, pending, err := m.store.GetThumbCounts(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Ready: ok, Pending: pending, Errored: errored, ActiveWorkers: m.activeCount()}, nil
}

func (m *Manager) ensureThumbJob(j thumbJob) chan struct{} {
	return m.ensureJob("thumb:"+j.videoID, func(done chan struct{}) { m.runThumbJob(j, done) })
}

func (m *Manager) runThumbJob(j thumbJob, done chan struct{}) {
	defer close(done)

	if !m.acquire() {
		return
	}
	defer m.release()

	meta, _, _ := m.store.GetMetadata(m.baseCtx, j.videoID) // best-effort seek-time hint

	genCtx, cancel := context.WithTimeout(m.baseCtx, thumbnailJobTimeout)
	defer cancel()
	if err := GenerateWithFallback(genCtx, j.absVideoPath, j.thumbPath, SeekTime(meta.DurationSeconds)); err != nil {
		if setErr := m.store.SetThumbError(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix, err.Error()); setErr != nil {
			slog.Error("recording thumbnail error failed", "id", j.videoID, "error", setErr)
		}
		return
	}
	if err := m.store.SetThumbOK(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix); err != nil {
		slog.Error("recording thumbnail success failed", "id", j.videoID, "error", err)
	}
}

// ---- Sprites ----------------------------------------------------------

// RequestSprite resolves (generating if necessary) the preview sprite for
// one video. Same wait/timeout semantics as Request.
func (m *Manager) RequestSprite(ctx context.Context, videoID, rootID, relPath, absVideoPath string, wait bool) (SpriteResult, error) {
	if !m.available {
		return SpriteResult{Failed: true}, nil
	}

	info, err := os.Stat(absVideoPath)
	if err != nil {
		return SpriteResult{}, err
	}
	hash := contentHash(videoID, info.Size(), info.ModTime().Unix())
	j := spriteJob{
		videoID:      videoID,
		rootID:       rootID,
		relPath:      relPath,
		absVideoPath: absVideoPath,
		imagePath:    filepath.Join(m.spritesDir, spriteImageName(hash)),
		metaPath:     filepath.Join(m.spritesDir, spriteMetaName(hash)),
		size:         info.Size(),
		mtimeUnix:    info.ModTime().Unix(),
	}

	if spriteFilesReady(j.imagePath, j.metaPath) {
		return SpriteResult{ImagePath: j.imagePath, MetaPath: j.metaPath, Ready: true}, nil
	}

	if status, _, ok, err := m.store.GetSpriteStatus(ctx, videoID); err != nil {
		return SpriteResult{}, err
	} else if ok && status == "error" {
		return SpriteResult{Failed: true}, nil
	}

	done := m.ensureSpriteJob(j)
	if !wait {
		return SpriteResult{Ready: false}, nil
	}

	select {
	case <-done:
		if spriteFilesReady(j.imagePath, j.metaPath) {
			return SpriteResult{ImagePath: j.imagePath, MetaPath: j.metaPath, Ready: true}, nil
		}
		if status, _, ok, err := m.store.GetSpriteStatus(ctx, videoID); err == nil && ok && status == "error" {
			return SpriteResult{Failed: true}, nil
		}
		return SpriteResult{Ready: false}, nil
	case <-time.After(requestWaitTimeout):
		return SpriteResult{Ready: false}, nil
	case <-ctx.Done():
		return SpriteResult{}, ctx.Err()
	}
}

// PregenerateSprites starts background sprite generation for every
// pending video across every root.
func (m *Manager) PregenerateSprites(ctx context.Context) (int, error) {
	if !m.available {
		return 0, nil
	}

	targets, err := m.store.ListPendingSprites(ctx)
	if err != nil {
		return 0, err
	}

	started := 0
	for _, t := range targets {
		root, ok := m.rootByID(t.RootID)
		if !ok {
			continue
		}
		absPath, err := filesystem.Resolve(root.Path, t.RelPath)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		hash := contentHash(t.ID, info.Size(), info.ModTime().Unix())
		m.ensureSpriteJob(spriteJob{
			videoID:      t.ID,
			rootID:       t.RootID,
			relPath:      t.RelPath,
			absVideoPath: absPath,
			imagePath:    filepath.Join(m.spritesDir, spriteImageName(hash)),
			metaPath:     filepath.Join(m.spritesDir, spriteMetaName(hash)),
			size:         info.Size(),
			mtimeUnix:    info.ModTime().Unix(),
		})
		started++
	}
	return started, nil
}

// SpriteStatus reports current sprite coverage across the whole index.
func (m *Manager) SpriteStatus(ctx context.Context) (Status, error) {
	ok, errored, pending, err := m.store.GetSpriteCounts(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Ready: ok, Pending: pending, Errored: errored, ActiveWorkers: m.activeCount()}, nil
}

func (m *Manager) ensureSpriteJob(j spriteJob) chan struct{} {
	return m.ensureJob("sprite:"+j.videoID, func(done chan struct{}) { m.runSpriteJob(j, done) })
}

func (m *Manager) runSpriteJob(j spriteJob, done chan struct{}) {
	defer close(done)

	if !m.acquire() {
		return
	}
	defer m.release()

	meta, _, _ := m.store.GetMetadata(m.baseCtx, j.videoID)

	// An accurate duration matters here more than for thumbnails: each
	// sprite frame is its own independent seek, and a guessed duration
	// that overshoots the file's actual length would seek past EOF on
	// the later frames, failing the whole sheet. If the indexer hasn't
	// probed this file yet, probe it directly rather than guessing.
	durationSeconds := meta.DurationSeconds
	width, height := 160, 90
	if meta.Width != nil && meta.Height != nil && *meta.Width > 0 {
		height = int(float64(160) * float64(*meta.Height) / float64(*meta.Width))
	}
	if durationSeconds == nil {
		if probe, err := media.Probe(m.baseCtx, j.absVideoPath); err == nil {
			durationSeconds = &probe.DurationSeconds
			if probe.Width > 0 {
				height = int(float64(160) * float64(probe.Height) / float64(probe.Width))
			}
		}
	}
	duration := durationOrDefault(durationSeconds)
	layout := ComputeSpriteLayout(duration)

	genCtx, cancel := context.WithTimeout(m.baseCtx, spriteJobTimeout)
	defer cancel()
	if err := GenerateSprite(genCtx, j.absVideoPath, j.imagePath, j.metaPath, layout, width, height, duration); err != nil {
		if setErr := m.store.SetSpriteError(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix, err.Error()); setErr != nil {
			slog.Error("recording sprite error failed", "id", j.videoID, "error", setErr)
		}
		return
	}
	if err := m.store.SetSpriteOK(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix); err != nil {
		slog.Error("recording sprite success failed", "id", j.videoID, "error", err)
	}
}

func durationOrDefault(d *float64) float64 {
	if d == nil || *d <= 0 {
		return 60 // a minute is a reasonable guess when duration is unknown
	}
	return *d
}

func spriteFilesReady(imagePath, metaPath string) bool {
	if _, err := os.Stat(imagePath); err != nil {
		return false
	}
	if _, err := os.Stat(metaPath); err != nil {
		return false
	}
	return true
}

// ---- Remux --------------------------------------------------------------

// RequestRemux resolves (generating if necessary) the cached container
// remux for one video. Same wait/timeout semantics as Request. Callers
// are expected to have already checked NeedsRemux — this doesn't re-check
// eligibility, only resolves/generates the cached file, same division of
// responsibility as Request not checking "is this a video file".
func (m *Manager) RequestRemux(ctx context.Context, videoID, rootID, relPath, absVideoPath string, wait bool) (RemuxResult, error) {
	if !m.available {
		return RemuxResult{Failed: true}, nil
	}

	info, err := os.Stat(absVideoPath)
	if err != nil {
		return RemuxResult{}, err
	}
	j := remuxJob{
		videoID:      videoID,
		rootID:       rootID,
		relPath:      relPath,
		absVideoPath: absVideoPath,
		remuxPath:    filepath.Join(m.remuxDir, remuxFileName(videoID, info.Size(), info.ModTime().Unix())),
		size:         info.Size(),
		mtimeUnix:    info.ModTime().Unix(),
	}

	if _, err := os.Stat(j.remuxPath); err == nil {
		return RemuxResult{Path: j.remuxPath, Ready: true}, nil
	}

	if status, _, ok, err := m.store.GetRemuxStatus(ctx, videoID); err != nil {
		return RemuxResult{}, err
	} else if ok && status == "error" {
		return RemuxResult{Failed: true}, nil
	}

	done := m.ensureRemuxJob(j)
	if !wait {
		return RemuxResult{Ready: false}, nil
	}

	select {
	case <-done:
		if _, err := os.Stat(j.remuxPath); err == nil {
			return RemuxResult{Path: j.remuxPath, Ready: true}, nil
		}
		if status, _, ok, err := m.store.GetRemuxStatus(ctx, videoID); err == nil && ok && status == "error" {
			return RemuxResult{Failed: true}, nil
		}
		return RemuxResult{Ready: false}, nil
	case <-time.After(requestWaitTimeout):
		return RemuxResult{Ready: false}, nil
	case <-ctx.Done():
		return RemuxResult{}, ctx.Err()
	}
}

// PregenerateRemux starts background remux generation for every pending,
// eligible video across every root.
func (m *Manager) PregenerateRemux(ctx context.Context) (int, error) {
	if !m.available {
		return 0, nil
	}

	targets, err := m.store.ListPendingRemux(ctx)
	if err != nil {
		return 0, err
	}

	started := 0
	for _, t := range targets {
		root, ok := m.rootByID(t.RootID)
		if !ok {
			continue
		}
		meta, ok, err := m.store.GetMetadata(ctx, t.ID)
		if err != nil || !ok || meta.Status != "ok" || meta.VideoCodec == nil || meta.AudioCodec == nil {
			continue
		}
		if !NeedsRemux(t.RelPath, *meta.VideoCodec, *meta.AudioCodec) {
			continue
		}
		absPath, err := filesystem.Resolve(root.Path, t.RelPath)
		if err != nil {
			continue
		}
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}
		m.ensureRemuxJob(remuxJob{
			videoID:      t.ID,
			rootID:       t.RootID,
			relPath:      t.RelPath,
			absVideoPath: absPath,
			remuxPath:    filepath.Join(m.remuxDir, remuxFileName(t.ID, info.Size(), info.ModTime().Unix())),
			size:         info.Size(),
			mtimeUnix:    info.ModTime().Unix(),
		})
		started++
	}
	return started, nil
}

// RemuxStatus reports current remux coverage across the whole index.
func (m *Manager) RemuxStatus(ctx context.Context) (Status, error) {
	ok, errored, pending, err := m.store.GetRemuxCounts(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Ready: ok, Pending: pending, Errored: errored, ActiveWorkers: m.activeCount()}, nil
}

func (m *Manager) ensureRemuxJob(j remuxJob) chan struct{} {
	return m.ensureJob("remux:"+j.videoID, func(done chan struct{}) { m.runRemuxJob(j, done) })
}

func (m *Manager) runRemuxJob(j remuxJob, done chan struct{}) {
	defer close(done)

	if !m.acquire() {
		return
	}
	defer m.release()

	genCtx, cancel := context.WithTimeout(m.baseCtx, remuxJobTimeout)
	defer cancel()
	if err := remux(genCtx, j.absVideoPath, j.remuxPath); err != nil {
		if setErr := m.store.SetRemuxError(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix, err.Error()); setErr != nil {
			slog.Error("recording remux error failed", "id", j.videoID, "error", setErr)
		}
		return
	}
	if err := m.store.SetRemuxOK(m.baseCtx, j.videoID, j.rootID, j.relPath, j.size, j.mtimeUnix); err != nil {
		slog.Error("recording remux success failed", "id", j.videoID, "error", err)
	}
}

// ---- Shared job machinery ----------------------------------------------

// ensureJob starts a job unless one with the same key is already in
// flight, and returns a channel that closes when it finishes either way.
// Safe to call repeatedly for the same key (dedup).
func (m *Manager) ensureJob(key string, run func(done chan struct{})) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	if done, ok := m.inflight[key]; ok {
		return done
	}
	done := make(chan struct{})
	m.inflight[key] = done
	go func() {
		run(done)
		m.mu.Lock()
		delete(m.inflight, key)
		m.mu.Unlock()
	}()
	return done
}

// acquire blocks until a worker slot is free or baseCtx is cancelled
// (app shutting down), in which case it returns false and the caller
// should abandon the job without running ffmpeg.
func (m *Manager) acquire() bool {
	select {
	case m.sem <- struct{}{}:
		atomic.AddInt32(&m.activeWorkers, 1)
		return true
	case <-m.baseCtx.Done():
		return false
	}
}

func (m *Manager) release() {
	atomic.AddInt32(&m.activeWorkers, -1)
	<-m.sem
}

// activeCount reports how many FFmpeg processes are actually running
// right now (semaphore slots held) — bounded by the configured worker
// count — as opposed to how many jobs are merely queued/dispatched.
func (m *Manager) activeCount() int {
	return int(atomic.LoadInt32(&m.activeWorkers))
}
