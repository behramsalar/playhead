package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"playhead/internal/config"
)

// setUpTwoRootsOneHidden indexes a video under each of two genuinely
// separate roots ("visible" and "hidden"), then hides the second one via
// the real settings API — the same path a user actually takes from the
// Settings page. Hiding a root drops it from the live root list
// (settings.MergeRoots) so it stops being scanned and can no longer be
// browsed into or played from directly, but its already-indexed rows
// are deliberately left in the database (so unhiding it later doesn't
// need a full rescan) — which is exactly what let them leak into
// cross-root views like Recently Added and search before this fix.
func setUpTwoRootsOneHidden(t *testing.T) http.Handler {
	t.Helper()
	parent := t.TempDir()
	visibleDir := filepath.Join(parent, "visible")
	hiddenDir := filepath.Join(parent, "hidden")
	mustMkdir(t, visibleDir)
	mustMkdir(t, hiddenDir)
	restore := config.SetDeviceIDFuncForTesting(func(path string) (uint64, bool) {
		switch path {
		case visibleDir:
			return 2, true
		case hiddenDir:
			return 3, true
		default:
			return 1, true
		}
	})
	t.Cleanup(restore)

	if err := os.WriteFile(filepath.Join(visibleDir, "v.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "h.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler, idx := newConfiguredTestServerWithIndexer(t, parent, "admin", "hunter2")
	idx.Scan(t.Context())

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{
		"roots": map[string]any{"hidden": map[string]any{"hidden": true}},
	})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("hiding root: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	return handler
}

func TestRecentExcludesHiddenRoot(t *testing.T) {
	handler := setUpTwoRootsOneHidden(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp recentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	for _, v := range resp.Videos {
		if v.Root == "hidden" {
			t.Fatalf("expected the hidden root's video excluded from /api/recent, got %+v", resp.Videos)
		}
	}
	if len(resp.Videos) != 1 || resp.Videos[0].Name != "v.mp4" {
		t.Fatalf("expected only the visible root's video, got %+v", resp.Videos)
	}
}

func TestSearchExcludesHiddenRoot(t *testing.T) {
	handler := setUpTwoRootsOneHidden(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/search?q=mp4", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Results []struct {
			Root string `json:"root"`
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	for _, v := range resp.Results {
		if v.Root == "hidden" {
			t.Fatalf("expected the hidden root's video excluded from search, got %+v", resp.Results)
		}
	}
	if len(resp.Results) != 1 || resp.Results[0].Name != "v.mp4" {
		t.Fatalf("expected only the visible root's video, got %+v", resp.Results)
	}
}

// TestRecentStillIncludesExplicitlyRequestedVisibleRoot guards against a
// too-broad fix: scoping /api/recent to a single, still-visible root
// (?root=visible) must keep working normally — the hidden-root filter
// only matters when spanning every root (the no-?root= case).
func TestRecentStillIncludesExplicitlyRequestedVisibleRoot(t *testing.T) {
	handler := setUpTwoRootsOneHidden(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent?root=visible", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp recentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Videos) != 1 || resp.Videos[0].Name != "v.mp4" {
		t.Fatalf("expected the visible root's own video when explicitly requested, got %+v", resp.Videos)
	}
}

// TestRecentRejectsExplicitlyRequestedHiddenRoot: requesting the hidden
// root by ID directly (?root=hidden) must still 404, the same as any
// other unknown root — hiding isn't a partial/soft-visible state.
func TestRecentRejectsExplicitlyRequestedHiddenRoot(t *testing.T) {
	handler := setUpTwoRootsOneHidden(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/recent?root=hidden", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
	}
}
