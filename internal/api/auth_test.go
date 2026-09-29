package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"playhead/internal/api"
	"playhead/internal/config"
	"playhead/internal/database"
	"playhead/internal/indexer"
	"playhead/internal/settings"
)

// newUnconfiguredTestServer builds a server around a real MEDIA_ROOT
// parent (so root auto-discovery has something real to find) with no
// config.json yet — the genuine first-run state onboarding tests need.
// Unlike the rest of the suite's helpers, it returns the raw handler with
// no auto-injected auth cookie, since these tests are specifically about
// exercising auth/setup themselves.
func newUnconfiguredTestServer(t *testing.T, mediaRootParent string) (handler http.Handler, settingsStore *settings.Store, dataDir string) {
	t.Helper()
	discovered, err := config.DiscoverRoots(mediaRootParent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	cfg := &config.Config{
		Addr:      ":0",
		DataDir:   t.TempDir(),
		MediaRoot: mediaRootParent,
		Roots:     discovered,
	}
	db, err := database.Open(filepath.Join(cfg.DataDir, "index.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewStore(db)
	idx := indexer.New(store, cfg.Roots)
	settingsStore = settings.NewStore(cfg.DataDir)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	return api.New(cfg, dist, store, idx, nil, settingsStore, t.Context()), settingsStore, cfg.DataDir
}

// newUnconfiguredTestServerWithIndexer is newUnconfiguredTestServer plus
// access to the indexer — for tests that need to actually scan real files
// (e.g. to then hide the root they're indexed under and confirm they
// disappear from a cross-root view) rather than just exercise auth/setup.
func newUnconfiguredTestServerWithIndexer(t *testing.T, mediaRootParent string) (handler http.Handler, settingsStore *settings.Store, idx *indexer.Indexer) {
	t.Helper()
	discovered, err := config.DiscoverRoots(mediaRootParent)
	if err != nil {
		t.Fatalf("DiscoverRoots error: %v", err)
	}
	cfg := &config.Config{
		Addr:      ":0",
		DataDir:   t.TempDir(),
		MediaRoot: mediaRootParent,
		Roots:     discovered,
	}
	db, err := database.Open(filepath.Join(cfg.DataDir, "index.db"))
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewStore(db)
	idx = indexer.New(store, cfg.Roots)
	settingsStore = settings.NewStore(cfg.DataDir)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	return api.New(cfg, dist, store, idx, nil, settingsStore, t.Context()), settingsStore, idx
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshaling request body: %v", err)
	}
	return bytes.NewReader(data)
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decoding response body: %v (body: %s)", err, body)
	}
	return v
}

type configResponseDTO struct {
	Configured    bool   `json:"configured"`
	Authenticated bool   `json:"authenticated"`
	ServerName    string `json:"serverName"`
	Roots         []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"roots"`
}

func TestGetConfigBeforeSetup(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler, _, _ := newUnconfiguredTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[configResponseDTO](t, rec.Body.Bytes())
	if got.Configured {
		t.Fatal("expected configured=false before setup")
	}
	if got.Authenticated {
		t.Fatal("expected authenticated=false before setup")
	}
}

// TestHealthStaysPublic guards against a real regression caught while
// building this phase: /api/health briefly ended up behind requireAuth
// along with everything else, which would have made Docker's own
// HEALTHCHECK (wget, no cookie support) permanently report the container
// unhealthy. Health must stay reachable with no session, both before and
// after setup.
func TestHealthStaysPublic(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler, _, _ := newUnconfiguredTestServer(t, root)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("pre-setup status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	setupBody := jsonBody(t, map[string]any{"serverName": "S", "username": "admin", "password": "hunter2"})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/setup", setupBody))

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("post-setup, no-cookie status = %d, want 200, body = %s", rec2.Code, rec2.Body.String())
	}
}

// TestConfigRootsOrderIsStable guards against a real regression: roots
// were briefly served from Go's randomized map iteration order, making
// the home-redirect target and the root-switcher's order change on every
// single page load for no visible reason. GET /api/config's root order
// must match config.DiscoverRoots' sorted order every time, not just
// often. Each subdirectory is faked as its own genuinely separate mount
// (config.SetDeviceIDFuncForTesting) so this exercises real multiple
// promoted roots — a plain t.TempDir() alone can't produce that, since
// its subdirectories always share one real device.
func TestConfigRootsOrderIsStable(t *testing.T) {
	root := t.TempDir()
	devices := map[string]uint64{}
	for i, name := range []string{"zebra", "anime", "misc", "tv"} {
		dir := filepath.Join(root, name)
		mustMkdir(t, dir)
		devices[dir] = uint64(i + 2) // distinct from root's own (implicit 1/unmocked)
	}
	restore := config.SetDeviceIDFuncForTesting(func(path string) (uint64, bool) {
		if dev, ok := devices[path]; ok {
			return dev, true
		}
		return 1, true // root itself, and anything else
	})
	t.Cleanup(restore)

	handler, _, _ := newUnconfiguredTestServer(t, root)

	want := []string{"anime", "misc", "tv", "zebra"} // alphabetical, matching os.ReadDir
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		got := decodeJSON[configResponseDTO](t, rec.Body.Bytes())
		ids := make([]string, len(got.Roots))
		for j, r := range got.Roots {
			ids[j] = r.ID
		}
		if len(ids) != len(want) {
			t.Fatalf("iteration %d: got %v, want %v", i, ids, want)
		}
		for j := range want {
			if ids[j] != want[j] {
				t.Fatalf("iteration %d: order = %v, want %v", i, ids, want)
			}
		}
	}
}

// TestSetupThenRejectsSecondCall covers two acceptance criteria: a fresh
// DATA_DIR leads to a working setup call that logs the caller in, and
// setup can't be run a second time once configured.
func TestSetupThenRejectsSecondCall(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler, settingsStore, _ := newUnconfiguredTestServer(t, root)

	body := jsonBody(t, map[string]any{
		"serverName": "My Homelab",
		"username":   "admin",
		"password":   "hunter2",
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("first setup status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !settingsStore.Configured() {
		t.Fatal("expected settings.Store to report configured after setup")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected setup to set a session cookie")
	}
	got := decodeJSON[configResponseDTO](t, rec.Body.Bytes())
	if !got.Configured || !got.Authenticated {
		t.Fatalf("expected configured=true authenticated=true, got %+v", got)
	}
	if got.ServerName != "My Homelab" {
		t.Fatalf("serverName = %q, want %q", got.ServerName, "My Homelab")
	}

	// Second call must be rejected — setup is not a reset endpoint.
	body2 := jsonBody(t, map[string]any{"serverName": "Different", "username": "x", "password": "y"})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/setup", body2))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want 409, body = %s", rec2.Code, rec2.Body.String())
	}

	cfg, ok := settingsStore.Get()
	if !ok || cfg.ServerName != "My Homelab" {
		t.Fatalf("expected the original config to survive the rejected second setup, got %+v (ok=%v)", cfg, ok)
	}
}

func TestSetupRejectsMissingFields(t *testing.T) {
	root := t.TempDir()
	handler, _, _ := newUnconfiguredTestServer(t, root)

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{"serverName": "", "username": "admin", "password": "hunter2"})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

// TestProtectedRouteRequiresAuth covers the acceptance criterion that the
// auth middleware denies an unauthenticated request to a protected route
// and allows one with a valid session.
func TestProtectedRouteRequiresAuth(t *testing.T) {
	root := t.TempDir()
	// A loose file directly under MEDIA_ROOT earns it the synthetic
	// "main" root (see config.DiscoverRoots) without depending on a
	// same-named subdirectory, which would now just be an ordinary
	// browsable subfolder inside "main", not the root itself.
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, _, _ := newUnconfiguredTestServer(t, root)

	// Before setup: no secret exists at all, so this must fail closed.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("pre-setup protected route status = %d, want 401", rec.Code)
	}

	// Complete setup.
	setupBody := jsonBody(t, map[string]any{"serverName": "S", "username": "admin", "password": "hunter2"})
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, httptest.NewRequest(http.MethodPost, "/api/setup", setupBody))
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setupRec.Code, setupRec.Body.String())
	}

	// No cookie: still 401 even though configured now.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil))
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("no-cookie protected route status = %d, want 401", rec2.Code)
	}
	var errBody errorEnvelope
	if err := json.Unmarshal(rec2.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if errBody.Error.Code == "" {
		t.Fatal("expected a stable JSON error code in the 401 body")
	}

	// With the cookie setup returned: succeeds.
	cookies := setupRec.Result().Cookies()
	req := httptest.NewRequest(http.MethodGet, "/api/browse?root=main&path=", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusOK {
		t.Fatalf("authenticated protected route status = %d, body = %s", rec3.Code, rec3.Body.String())
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	root := t.TempDir()
	handler, _, _ := newUnconfiguredTestServer(t, root)

	setupBody := jsonBody(t, map[string]any{"serverName": "S", "username": "admin", "password": "correct-password"})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/setup", setupBody))

	rec := httptest.NewRecorder()
	loginBody := jsonBody(t, map[string]any{"username": "admin", "password": "wrong-password"})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("expected no session cookie to be set on a failed login")
	}
}

// TestLoginPersistsAcrossRestart covers acceptance criterion 2: restarting
// the container after setup goes straight to login (not onboarding again
// — Configured() is still true), and a valid existing session cookie
// skips even the login page (a fresh server instance, standing in for a
// restarted process, accepts a cookie signed before the "restart").
func TestLoginPersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	handler, _, dataDir := newUnconfiguredTestServer(t, root)

	setupBody := jsonBody(t, map[string]any{"serverName": "S", "username": "admin", "password": "hunter2"})
	setupRec := httptest.NewRecorder()
	handler.ServeHTTP(setupRec, httptest.NewRequest(http.MethodPost, "/api/setup", setupBody))
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setupRec.Code, setupRec.Body.String())
	}
	cookies := setupRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected setup to set a session cookie")
	}

	// Simulate a container restart: a brand-new settings.Store and a
	// brand-new Server, both built fresh against the same DATA_DIR/config.json.
	restartedSettings := settings.NewStore(dataDir)
	if err := restartedSettings.Load(); err != nil {
		t.Fatalf("Load on restarted store: %v", err)
	}
	if !restartedSettings.Configured() {
		t.Fatal("expected the restarted store to still report configured (setup shouldn't run again)")
	}

	db, err := database.Open(filepath.Join(dataDir, "index.db"))
	if err != nil {
		t.Fatalf("opening restarted database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewStore(db)
	cfg := &config.Config{Addr: ":0", DataDir: dataDir, MediaRoot: root}
	idx := indexer.New(store, nil)
	dist := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	restartedHandler := api.New(cfg, dist, store, idx, nil, restartedSettings, t.Context())

	// GET /api/config without the cookie: authenticated=false, but
	// configured=true (straight to login, not onboarding).
	rec := httptest.NewRecorder()
	restartedHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	got := decodeJSON[configResponseDTO](t, rec.Body.Bytes())
	if !got.Configured {
		t.Fatal("expected configured=true after restart")
	}
	if got.Authenticated {
		t.Fatal("expected authenticated=false without a cookie after restart")
	}

	// The cookie from before the "restart" still authenticates.
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	restartedHandler.ServeHTTP(rec2, req)
	got2 := decodeJSON[configResponseDTO](t, rec2.Body.Bytes())
	if !got2.Authenticated {
		t.Fatal("expected the pre-restart session cookie to still authenticate after restart")
	}
}
