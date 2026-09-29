package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"playhead/internal/config"
	"playhead/internal/indexer"
)

// newConfiguredTestServer completes setup against a real MEDIA_ROOT
// parent and returns a handler with a valid auth cookie already attached
// (via withAuthCookie), plus the settings.Store so tests can inspect
// config.json directly.
func newConfiguredTestServer(t *testing.T, mediaRootParent, username, password string) http.Handler {
	t.Helper()
	handler, settingsStore, _ := newUnconfiguredTestServer(t, mediaRootParent)

	body := jsonBody(t, map[string]any{"serverName": "Original Name", "username": username, "password": password})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", rec.Code, rec.Body.String())
	}

	return withAuthCookie(t, handler, settingsStore)
}

// newConfiguredTestServerWithIndexer is newConfiguredTestServer plus
// access to the indexer, for tests that need real indexed content to
// still be present when a root gets hidden partway through (see
// hidden_root_test.go).
func newConfiguredTestServerWithIndexer(t *testing.T, mediaRootParent, username, password string) (http.Handler, *indexer.Indexer) {
	t.Helper()
	handler, settingsStore, idx := newUnconfiguredTestServerWithIndexer(t, mediaRootParent)

	body := jsonBody(t, map[string]any{"serverName": "Original Name", "username": username, "password": password})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/setup", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", rec.Code, rec.Body.String())
	}

	return withAuthCookie(t, handler, settingsStore), idx
}

type settingsResponseDTO struct {
	ServerName string `json:"serverName"`
	Username   string `json:"username"`
	Roots      []struct {
		ID          string `json:"id"`
		FolderName  string `json:"folderName"`
		DisplayName string `json:"displayName"`
		Hidden      bool   `json:"hidden"`
	} `json:"roots"`
}

func TestSettingsRenameServer(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler := newConfiguredTestServer(t, root, "admin", "hunter2")

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{"serverName": "Renamed Homelab"})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[settingsResponseDTO](t, rec.Body.Bytes())
	if got.ServerName != "Renamed Homelab" {
		t.Fatalf("serverName = %q, want %q", got.ServerName, "Renamed Homelab")
	}

	// GET /api/config (what the running app actually uses) reflects it too.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	cfgResp := decodeJSON[configResponseDTO](t, rec2.Body.Bytes())
	if cfgResp.ServerName != "Renamed Homelab" {
		t.Fatalf("GET /api/config serverName = %q, want %q", cfgResp.ServerName, "Renamed Homelab")
	}
}

func TestSettingsRenameAndHideRoot(t *testing.T) {
	root := t.TempDir()
	animeDir := filepath.Join(root, "anime")
	miscDir := filepath.Join(root, "misc")
	mustMkdir(t, animeDir)
	mustMkdir(t, miscDir)
	// anime/misc are faked as genuinely separate mounts (distinct device
	// IDs) so each becomes its own promoted root — a plain t.TempDir()
	// alone can't produce that, since real subdirectories always share
	// one device; see config.SetDeviceIDFuncForTesting.
	restore := config.SetDeviceIDFuncForTesting(func(path string) (uint64, bool) {
		switch path {
		case animeDir:
			return 2, true
		case miscDir:
			return 3, true
		default:
			return 1, true
		}
	})
	t.Cleanup(restore)

	handler := newConfiguredTestServer(t, root, "admin", "hunter2")

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{
		"roots": map[string]any{
			"anime": map[string]any{"displayName": "Anime Collection"},
			"misc":  map[string]any{"hidden": true},
		},
	})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[settingsResponseDTO](t, rec.Body.Bytes())
	byID := map[string]struct {
		ID          string
		FolderName  string
		DisplayName string
		Hidden      bool
	}{}
	for _, r := range got.Roots {
		byID[r.ID] = struct {
			ID          string
			FolderName  string
			DisplayName string
			Hidden      bool
		}{r.ID, r.FolderName, r.DisplayName, r.Hidden}
	}
	if byID["anime"].DisplayName != "Anime Collection" {
		t.Fatalf("anime displayName = %q, want %q", byID["anime"].DisplayName, "Anime Collection")
	}
	if !byID["misc"].Hidden {
		t.Fatal("expected misc to be hidden in the settings response")
	}

	// GET /api/config must exclude the hidden root entirely (that's what
	// the running app's root list/switcher actually uses).
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	cfgResp := decodeJSON[configResponseDTO](t, rec2.Body.Bytes())
	for _, r := range cfgResp.Roots {
		if r.ID == "misc" {
			t.Fatalf("expected hidden root 'misc' excluded from GET /api/config, got %+v", cfgResp.Roots)
		}
		if r.ID == "anime" && r.Name != "Anime Collection" {
			t.Fatalf("expected the display-name override to apply to GET /api/config too, got %q", r.Name)
		}
	}
}

// TestSettingsRescanPicksUpNewFolder covers acceptance criterion 3:
// mounting a new *separate volume* under MEDIA_ROOT and rescanning makes
// it appear as its own root without restarting — a plain new subfolder
// on the same device needs no rescan at all, since browsing already
// reflects the live filesystem; rescan specifically exists to notice a
// new mount boundary. Both "anime" and the not-yet-mounted
// "newly-mounted" are faked as distinct devices from root (see
// config.SetDeviceIDFuncForTesting) so this genuinely exercises root
// promotion, not just "a folder now exists".
func TestSettingsRescanPicksUpNewFolder(t *testing.T) {
	root := t.TempDir()
	animeDir := filepath.Join(root, "anime")
	newDir := filepath.Join(root, "newly-mounted")
	mustMkdir(t, animeDir)

	deviceOf := map[string]uint64{animeDir: 2}
	restore := config.SetDeviceIDFuncForTesting(func(path string) (uint64, bool) {
		if dev, ok := deviceOf[path]; ok {
			return dev, true
		}
		return 1, true
	})
	t.Cleanup(restore)

	handler := newConfiguredTestServer(t, root, "admin", "hunter2")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	before := decodeJSON[configResponseDTO](t, rec.Body.Bytes())
	if len(before.Roots) != 1 {
		t.Fatalf("expected 1 root before mounting a new volume, got %+v", before.Roots)
	}

	// "Mount a new volume" without restarting — a real deployment would
	// have just run `docker run -v newhost:/media/newly-mounted ...`; here
	// that's simulated by creating the folder and giving it its own
	// device ID.
	mustMkdir(t, newDir)
	deviceOf[newDir] = 3

	rescanRec := httptest.NewRecorder()
	handler.ServeHTTP(rescanRec, httptest.NewRequest(http.MethodPost, "/api/settings/rescan", nil))
	if rescanRec.Code != http.StatusOK {
		t.Fatalf("rescan status = %d, body = %s", rescanRec.Code, rescanRec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	after := decodeJSON[configResponseDTO](t, rec2.Body.Bytes())
	if len(after.Roots) != 2 {
		t.Fatalf("expected 2 roots after rescan, got %+v", after.Roots)
	}
	found := false
	for _, r := range after.Roots {
		if r.ID == "newly-mounted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the newly-mounted folder to appear as a root, got %+v", after.Roots)
	}
}

// TestChangePasswordRotatesSessionAndRejectsOldPassword covers acceptance
// criterion 5: changing the password logs out every existing session and
// the old password stops working.
func TestChangePasswordRotatesSessionAndRejectsOldPassword(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler := newConfiguredTestServer(t, root, "admin", "old-password")

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{"currentPassword": "old-password", "newPassword": "new-password"})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/password", body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// The request that made the change still carries the cookie it was
	// auto-injected with (signed against the OLD secret) — a subsequent
	// request with that same cookie must now be rejected, proving the
	// secret actually rotated and invalidated the in-flight session too.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/browse?root=anime&path=", nil))
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("status with the pre-rotation cookie = %d, want 401", rec2.Code)
	}

	// The old password must no longer work at all, even via a fresh login.
	loginRec := httptest.NewRecorder()
	loginBody := jsonBody(t, map[string]any{"username": "admin", "password": "old-password"})
	handler.ServeHTTP(loginRec, httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody))
	if loginRec.Code != http.StatusUnauthorized {
		t.Fatalf("login with old password status = %d, want 401", loginRec.Code)
	}

	// The new password does work.
	loginRec2 := httptest.NewRecorder()
	loginBody2 := jsonBody(t, map[string]any{"username": "admin", "password": "new-password"})
	handler.ServeHTTP(loginRec2, httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody2))
	if loginRec2.Code != http.StatusOK {
		t.Fatalf("login with new password status = %d, body = %s", loginRec2.Code, loginRec2.Body.String())
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler := newConfiguredTestServer(t, root, "admin", "correct-password")

	rec := httptest.NewRecorder()
	body := jsonBody(t, map[string]any{"currentPassword": "wrong-password", "newPassword": "new-password"})
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/password", body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body = %s", rec.Code, rec.Body.String())
	}
	var errBody errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if errBody.Error.Code == "" {
		t.Fatal("expected a stable JSON error code")
	}
}

func TestSettingsEndpointsRequireAuth(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "anime"))
	handler, _, _ := newUnconfiguredTestServer(t, root)

	setupBody := jsonBody(t, map[string]any{"serverName": "S", "username": "admin", "password": "hunter2"})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/setup", setupBody))

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/settings", nil),
		httptest.NewRequest(http.MethodPost, "/api/settings", jsonBody(t, map[string]any{"serverName": "x"})),
		httptest.NewRequest(http.MethodPost, "/api/settings/rescan", nil),
		httptest.NewRequest(http.MethodPost, "/api/settings/password", jsonBody(t, map[string]any{"currentPassword": "a", "newPassword": "b"})),
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a session: status = %d, want 401", req.Method, req.URL.Path, rec.Code)
		}
	}
}
