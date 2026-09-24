package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"playhead/internal/auth"
	"playhead/internal/config"
	"playhead/internal/settings"
)

type rootSettingDTO struct {
	ID          string `json:"id"`
	FolderName  string `json:"folderName"`
	DisplayName string `json:"displayName"`
	Hidden      bool   `json:"hidden"`
}

type settingsResponse struct {
	ServerName string           `json:"serverName"`
	Username   string           `json:"username"`
	Roots      []rootSettingDTO `json:"roots"`
}

// buildSettingsResponse lists every currently-discovered root, including
// hidden ones (unlike GET /api/config's list, which is what the running
// app actually uses) — the settings page needs to see a hidden root to
// offer un-hiding it.
func (s *Server) buildSettingsResponse() (settingsResponse, error) {
	cfg, _ := s.settings.Get()

	discovered, err := config.DiscoverRoots(s.mediaRootParent)
	if err != nil {
		return settingsResponse{}, err
	}

	roots := make([]rootSettingDTO, 0, len(discovered))
	for _, d := range discovered {
		override := cfg.Roots[d.ID]
		displayName := override.DisplayName
		if displayName == "" {
			displayName = d.Name
		}
		roots = append(roots, rootSettingDTO{
			ID:          d.ID,
			FolderName:  d.Name,
			DisplayName: displayName,
			Hidden:      override.Hidden,
		})
	}

	return settingsResponse{ServerName: cfg.ServerName, Username: cfg.AuthUsername, Roots: roots}, nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := s.buildSettingsResponse()
	if err != nil {
		slog.Error("listing roots for settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not read settings.")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type updateSettingsRequest struct {
	ServerName *string                          `json:"serverName"`
	Roots      map[string]settings.RootOverride `json:"roots"`
}

// handleUpdateSettings renames the server and/or applies per-root
// display-name/hidden overrides in one call. Root overrides are merged
// key-by-key into the existing map (a caller updating one root's
// override doesn't need to resend every other root's).
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}

	cfg, ok := s.settings.Get()
	if !ok {
		writeError(w, http.StatusConflict, "not_configured", "Setup has not been completed yet.")
		return
	}

	if req.ServerName != nil {
		name := strings.TrimSpace(*req.ServerName)
		if name == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "Server name cannot be empty.")
			return
		}
		cfg.ServerName = name
	}

	if len(req.Roots) > 0 {
		if cfg.Roots == nil {
			cfg.Roots = make(map[string]settings.RootOverride, len(req.Roots))
		}
		for id, override := range req.Roots {
			cfg.Roots[id] = override
		}
	}

	if err := s.settings.Save(cfg); err != nil {
		slog.Error("saving settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not save settings.")
		return
	}

	if _, err := s.refreshRoots(); err != nil {
		slog.Error("refreshing roots after settings update failed", "error", err)
	}

	resp, err := s.buildSettingsResponse()
	if err != nil {
		slog.Error("listing roots for settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Settings saved, but could not read them back.")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleSettingsRescan re-discovers MEDIA_ROOT's subdirectories (picking
// up a newly mounted drive/folder without a restart) and kicks off an
// index scan so the new content is browsable with metadata as soon as
// possible — browsing itself doesn't need the index, but this avoids a
// silent wait for the next scheduled/manual scan.
func (s *Server) handleSettingsRescan(w http.ResponseWriter, r *http.Request) {
	if _, err := s.refreshRoots(); err != nil {
		slog.Error("rescanning roots failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not rescan roots.")
		return
	}
	if s.indexer != nil {
		s.indexer.TryStartScan(s.appCtx)
	}

	resp, err := s.buildSettingsResponse()
	if err != nil {
		slog.Error("listing roots for settings failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Rescan succeeded, but could not read settings back.")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type changePasswordRequest struct {
	CurrentPassword string  `json:"currentPassword"`
	NewUsername     *string `json:"newUsername"`
	NewPassword     *string `json:"newPassword"`
}

// handleChangePassword verifies the current password, then applies the
// requested username/password change and rotates the session secret —
// the simplest correct way to force every existing session (including
// this request's own) to re-authenticate, with no server-side session
// table to otherwise revoke from. The response itself still succeeds
// normally; it's up to the frontend to treat a successful response here
// as "you're about to be logged out" and redirect to /login rather than
// something failing confusingly mid-request.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}
	if req.NewUsername == nil && req.NewPassword == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Provide a new username and/or a new password.")
		return
	}

	cfg, ok := s.settings.Get()
	if !ok {
		writeError(w, http.StatusConflict, "not_configured", "Setup has not been completed yet.")
		return
	}
	if !auth.VerifyPassword(cfg.AuthPasswordHash, req.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Current password is incorrect.")
		return
	}

	if req.NewUsername != nil {
		username := strings.TrimSpace(*req.NewUsername)
		if username == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "Username cannot be empty.")
			return
		}
		cfg.AuthUsername = username
	}
	if req.NewPassword != nil {
		if *req.NewPassword == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "Password cannot be empty.")
			return
		}
		hash, err := auth.HashPassword(*req.NewPassword)
		if err != nil {
			slog.Error("hashing new password failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Could not change password.")
			return
		}
		cfg.AuthPasswordHash = hash
	}

	secret, err := auth.GenerateSecret()
	if err != nil {
		slog.Error("generating new session secret failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not change password.")
		return
	}
	cfg.SessionSecret = secret

	if err := s.settings.Save(cfg); err != nil {
		slog.Error("saving settings after password change failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not change password.")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
