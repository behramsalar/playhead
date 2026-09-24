package preview

import (
	"context"
	"os"
	"path/filepath"
	"sort"
)

// CacheStatus reports current on-disk cache usage against the configured
// limit (0 = unlimited).
type CacheStatus struct {
	TotalBytes int64 `json:"totalBytes"`
	MaxBytes   int64 `json:"maxBytes,omitempty"`
}

// hashLen matches contentHash's fixed output length (hash.go).
const hashLen = 32

// cacheDirs lists every directory this package's cache-management sweeps
// (orphan cleanup, clear-all, size status/limit) cover. A remuxed file is
// typically tens to hundreds of MB — much larger than a thumbnail or
// sprite sheet — so CACHE_MAX_BYTES and orphan cleanup matter more once
// cache/remux/ is in play; see README.md.
func (m *Manager) cacheDirs() []string {
	return []string{m.thumbsDir, m.spritesDir, m.remuxDir}
}

// validHashes computes the current content hash for every indexed video —
// the set of thumbnail/sprite/remux cache filenames that are still
// actually referenced by the index.
func (m *Manager) validHashes(ctx context.Context) (map[string]bool, error) {
	keys, err := m.store.ListVideoCacheKeys(ctx)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]bool, len(keys))
	for _, k := range keys {
		valid[contentHash(k.ID, k.Size, k.MtimeUnix)] = true
	}
	return valid, nil
}

// hashPrefix extracts a cache filename's leading content-hash, or "" if
// the name is too short to be one of ours — a safety guard so cleanup
// only ever considers files this package could plausibly have generated
// (e.g. never a stray .gitkeep).
func hashPrefix(name string) string {
	if len(name) < hashLen {
		return ""
	}
	return name[:hashLen]
}

// CleanOrphans removes cache files whose hash doesn't correspond to any
// current video row — left behind when a file changes (its old thumbnail/
// sprite/remux is orphaned under its old hash) or is removed/renamed
// entirely. Safe to call anytime; never touches a file matching a
// current row.
func (m *Manager) CleanOrphans(ctx context.Context) (int, error) {
	valid, err := m.validHashes(ctx)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, dir := range m.cacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			hash := hashPrefix(e.Name())
			if hash == "" || valid[hash] {
				continue
			}
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

// ClearAll wipes every cached thumbnail/sprite/remux file and resets
// every video's generation status to pending, so the next folder view
// (or a pregenerate call) rebuilds everything from scratch — the
// "rebuild previews" control.
func (m *Manager) ClearAll(ctx context.Context) (int, error) {
	removed := 0
	for _, dir := range m.cacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	if err := m.store.ResetAllPreviewStatus(ctx); err != nil {
		return removed, err
	}
	return removed, nil
}

// RetryThumbnail resets one video's thumbnail status so it's regenerated
// on next view, even if it previously failed permanently — for a targeted
// retry (e.g. after fixing whatever made the source file unreadable)
// without waiting for the whole file to change.
func (m *Manager) RetryThumbnail(ctx context.Context, id string) error {
	return m.store.ResetThumbStatus(ctx, id)
}

// RetrySprite is RetryThumbnail's sprite equivalent.
func (m *Manager) RetrySprite(ctx context.Context, id string) error {
	return m.store.ResetSpriteStatus(ctx, id)
}

// RetryRemux is RetryThumbnail's remux equivalent.
func (m *Manager) RetryRemux(ctx context.Context, id string) error {
	return m.store.ResetRemuxStatus(ctx, id)
}

// Status reports current cache usage against the configured limit.
func (m *Manager) CacheStatus() (CacheStatus, error) {
	var total int64
	for _, dir := range m.cacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return CacheStatus{}, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if info, err := e.Info(); err == nil {
				total += info.Size()
			}
		}
	}
	return CacheStatus{TotalBytes: total, MaxBytes: m.cacheMaxBytes}, nil
}

// cacheFile pairs a cache entry's path with its generation time, for
// EnforceCacheLimit's eviction ordering.
type cacheFile struct {
	path    string
	size    int64
	modTime int64
}

// EnforceCacheLimit deletes the least-recently-generated cache files
// (oldest mtime first — see ARCHITECTURE.md for why generation time
// rather than access time) until total cache usage is back under the
// configured limit, with a 10% cushion so it doesn't immediately re-
// trigger on the next check. A no-op if no limit is configured or usage
// is already under it.
func (m *Manager) EnforceCacheLimit(ctx context.Context) (removedFiles int, removedBytes int64, err error) {
	if m.cacheMaxBytes <= 0 {
		return 0, 0, nil
	}

	var files []cacheFile
	var total int64
	for _, dir := range m.cacheDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, 0, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, cacheFile{path: filepath.Join(dir, e.Name()), size: info.Size(), modTime: info.ModTime().Unix()})
			total += info.Size()
		}
	}

	if total <= m.cacheMaxBytes {
		return 0, 0, nil
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modTime < files[j].modTime })

	target := m.cacheMaxBytes * 9 / 10
	for _, f := range files {
		if total <= target {
			break
		}
		if err := os.Remove(f.path); err != nil {
			continue
		}
		total -= f.size
		removedBytes += f.size
		removedFiles++
	}
	return removedFiles, removedBytes, nil
}
