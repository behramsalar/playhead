// Package api implements the HTTP surface: JSON endpoints for browsing and
// video metadata, range-based streaming, and serving the embedded frontend.
package api

import (
	"context"
	"io/fs"
	"net/http"
	"sync"

	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/indexer"
	"playhead/internal/preview"
	"playhead/internal/settings"
)

// Server holds the dependencies shared by all handlers.
type Server struct {
	// rootsMu guards roots/rootsOrdered, which a Phase 7 settings-page
	// rescan can replace at any time (see refreshRoots in roots.go) —
	// every other field here is set once at construction and never
	// mutated. rootsOrdered mirrors roots' values in a stable, discovery
	// order (see allRoots) — roots alone (a map) would otherwise iterate
	// in Go's randomized order.
	rootsMu      sync.RWMutex
	roots        map[string]config.Root
	rootsOrdered []config.Root

	// mediaRootParent is MEDIA_ROOT itself: the parent directory
	// refreshRoots re-scans for immediate subdirectories.
	mediaRootParent string
	settings        *settings.Store

	dist     fs.FS
	hasFE    bool
	store    *database.Store
	indexer  *indexer.Indexer
	previews *preview.Manager
	// appCtx scopes background work triggered by a request (POST
	// /api/scan, a thumbnail generation job) to the app's lifetime, not
	// the triggering request's.
	appCtx context.Context
}

// New builds the HTTP handler for the app. dist is the embedded frontend
// build (already rooted at its "dist" directory); it may be an empty
// placeholder in local dev before the frontend is built. store, idx, and
// previews may be nil, in which case metadata/previews are always
// absent and the related endpoints report the feature as unavailable.
// settingsStore drives onboarding/auth/settings (Phase 7); cfg.Roots is
// the already-discovered-and-merged initial root list (see main.go).
func New(cfg *config.Config, dist fs.FS, store *database.Store, idx *indexer.Indexer, previews *preview.Manager, settingsStore *settings.Store, appCtx context.Context) http.Handler {
	s := &Server{
		mediaRootParent: cfg.MediaRoot,
		settings:        settingsStore,
		dist:            dist,
		store:           store,
		indexer:         idx,
		previews:        previews,
		appCtx:          appCtx,
	}
	s.setRoots(cfg.Roots)
	if _, err := fs.Stat(dist, "index.html"); err == nil {
		s.hasFE = true
	}

	mux := http.NewServeMux()

	// Public: the SPA shell always needs these reachable regardless of
	// auth state, to render onboarding/login in the first place. Health
	// is also public — Docker's own HEALTHCHECK (and any external
	// monitoring) has no way to authenticate, and it leaks nothing beyond
	// what the healthcheck itself needs (root IDs and reachability).
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("POST /api/setup", s.handleSetup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)

	// Everything else requires a valid session.
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/settings", s.requireAuth(s.handleGetSettings))
	mux.HandleFunc("POST /api/settings", s.requireAuth(s.handleUpdateSettings))
	mux.HandleFunc("POST /api/settings/rescan", s.requireAuth(s.handleSettingsRescan))
	mux.HandleFunc("POST /api/settings/password", s.requireAuth(s.handleChangePassword))

	mux.HandleFunc("GET /api/roots", s.requireAuth(s.handleRoots))
	mux.HandleFunc("GET /api/browse", s.requireAuth(s.handleBrowse))
	mux.HandleFunc("GET /api/videos/{id}", s.requireAuth(s.handleVideoInfo))
	mux.HandleFunc("GET /api/videos/{id}/stream", s.requireAuth(s.handleStream))
	mux.HandleFunc("HEAD /api/videos/{id}/stream", s.requireAuth(s.handleStream))
	mux.HandleFunc("GET /api/videos/{id}/thumbnail", s.requireAuth(s.handleThumbnail))
	mux.HandleFunc("GET /api/videos/{id}/sprite", s.requireAuth(s.handleSpriteImage))
	mux.HandleFunc("GET /api/videos/{id}/sprite.json", s.requireAuth(s.handleSpriteMeta))
	mux.HandleFunc("GET /api/scan/status", s.requireAuth(s.handleScanStatus))
	mux.HandleFunc("POST /api/scan", s.requireAuth(s.handleScanTrigger))
	mux.HandleFunc("GET /api/thumbnails/status", s.requireAuth(s.handleThumbnailStatus))
	mux.HandleFunc("POST /api/thumbnails/pregenerate", s.requireAuth(s.handleThumbnailPregenerate))
	mux.HandleFunc("GET /api/sprites/status", s.requireAuth(s.handleSpriteStatus))
	mux.HandleFunc("POST /api/sprites/pregenerate", s.requireAuth(s.handleSpritePregenerate))
	mux.HandleFunc("PUT /api/videos/{id}/resume", s.requireAuth(s.handleResumeSave))
	mux.HandleFunc("DELETE /api/videos/{id}/resume", s.requireAuth(s.handleResumeClear))
	mux.HandleFunc("POST /api/videos/{id}/thumbnail/retry", s.requireAuth(s.handleThumbnailRetry))
	mux.HandleFunc("POST /api/videos/{id}/sprite/retry", s.requireAuth(s.handleSpriteRetry))
	mux.HandleFunc("POST /api/videos/{id}/remux/retry", s.requireAuth(s.handleRemuxRetry))
	mux.HandleFunc("GET /api/remux/status", s.requireAuth(s.handleRemuxStatus))
	mux.HandleFunc("POST /api/remux/pregenerate", s.requireAuth(s.handleRemuxPregenerate))
	mux.HandleFunc("GET /api/search", s.requireAuth(s.handleSearch))
	mux.HandleFunc("GET /api/recent", s.requireAuth(s.handleRecent))
	mux.HandleFunc("GET /api/tags", s.requireAuth(s.handleListTags))
	mux.HandleFunc("POST /api/tags/bulk", s.requireAuth(s.handleBulkTagVideos))
	mux.HandleFunc("DELETE /api/tags/{tagId}", s.requireAuth(s.handleDeleteTag))
	mux.HandleFunc("POST /api/videos/{id}/tags", s.requireAuth(s.handleAddTagToVideo))
	mux.HandleFunc("DELETE /api/videos/{id}/tags/{tagId}", s.requireAuth(s.handleRemoveTagFromVideo))
	mux.HandleFunc("GET /api/cache/status", s.requireAuth(s.handleCacheStatus))
	mux.HandleFunc("POST /api/cache/clean-orphans", s.requireAuth(s.handleCleanOrphans))
	mux.HandleFunc("POST /api/cache/enforce-limit", s.requireAuth(s.handleEnforceCacheLimit))
	mux.HandleFunc("POST /api/previews/clear", s.requireAuth(s.handlePreviewsClear))

	// Static (SPA shell + assets): also public, so the onboarding/login
	// screens themselves can load.
	mux.HandleFunc("/", s.handleStatic)

	return mux
}
