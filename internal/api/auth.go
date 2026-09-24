package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"playhead/internal/auth"
	"playhead/internal/settings"
)

// requireAuth wraps a handler so it 401s (stable JSON error shape) unless
// the request carries a valid, unexpired session cookie — the single
// gate every protected route shares. Unconfigured (no config.json yet)
// always fails closed, same as an invalid session: there's no secret to
// verify against, so nothing protected is reachable until setup runs.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		next(w, r)
	}
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	if s.settings == nil {
		return false
	}
	cfg, ok := s.settings.Get()
	if !ok {
		return false
	}
	token, ok := auth.ReadSessionCookie(r)
	if !ok {
		return false
	}
	valid, err := auth.VerifySession(cfg.SessionSecret, token)
	return err == nil && valid
}

// login signs a fresh session token against the currently-stored secret
// and sets it as the response's cookie. Never called with s.settings nil
// or unconfigured — callers check first.
func (s *Server) login(w http.ResponseWriter) {
	cfg, _ := s.settings.Get()
	expiry := time.Now().Add(auth.SessionLifetime)
	token, err := auth.SignSession(cfg.SessionSecret, expiry)
	if err != nil {
		slog.Error("signing session token failed", "error", err)
		return
	}
	auth.SetSessionCookie(w, token, expiry)
}

type rootInfoDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// configResponse is GET /api/config's body — the one endpoint the SPA
// shell always needs regardless of auth state, so it can decide whether
// to render onboarding, login, or the normal app.
type configResponse struct {
	Configured    bool          `json:"configured"`
	Authenticated bool          `json:"authenticated"`
	ServerName    string        `json:"serverName,omitempty"`
	Roots         []rootInfoDTO `json:"roots"`
}

// buildConfigResponse takes authenticated explicitly rather than deriving
// it from the request's cookie: handleSetup/handleLogin call this right
// after setting a brand-new session cookie on the response, which the
// original *http.Request object was never resent with, so re-checking
// the request here would (incorrectly) report authenticated=false right
// after a successful login.
func (s *Server) buildConfigResponse(authenticated bool) configResponse {
	resp := configResponse{Roots: []rootInfoDTO{}, Authenticated: authenticated}
	if s.settings != nil {
		if cfg, ok := s.settings.Get(); ok {
			resp.Configured = true
			resp.ServerName = cfg.ServerName
		}
	}
	for _, root := range s.allRoots() {
		resp.Roots = append(resp.Roots, rootInfoDTO{ID: root.ID, Name: root.Name})
	}
	return resp
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildConfigResponse(s.isAuthenticated(r)))
}

type setupRequest struct {
	ServerName string                           `json:"serverName"`
	Username   string                           `json:"username"`
	Password   string                           `json:"password"`
	Roots      map[string]settings.RootOverride `json:"roots"`
}

// handleSetup performs first-run onboarding: hash the password, generate
// a session secret, write config.json, and log the caller straight in.
// Rejects outright once configured — this is not a reset endpoint;
// resetting is an intentional admin action (deleting config.json), not
// something reachable over the network.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Setup is not available.")
		return
	}
	if s.settings.Configured() {
		writeError(w, http.StatusConflict, "already_configured", "Setup has already been completed.")
		return
	}

	var req setupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}
	req.ServerName = strings.TrimSpace(req.ServerName)
	req.Username = strings.TrimSpace(req.Username)
	if req.ServerName == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Server name, username, and password are all required.")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("hashing password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not complete setup.")
		return
	}
	secret, err := auth.GenerateSecret()
	if err != nil {
		slog.Error("generating session secret failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not complete setup.")
		return
	}

	cfg := settings.Config{
		ServerName:       req.ServerName,
		AuthUsername:     req.Username,
		AuthPasswordHash: passwordHash,
		SessionSecret:    secret,
		Roots:            req.Roots,
	}
	if err := s.settings.Save(cfg); err != nil {
		slog.Error("saving config.json failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not complete setup.")
		return
	}

	if _, err := s.refreshRoots(); err != nil {
		slog.Error("refreshing roots after setup failed", "error", err)
	}

	s.login(w)
	writeJSON(w, http.StatusOK, s.buildConfigResponse(true))
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Login is not available.")
		return
	}
	cfg, ok := s.settings.Get()
	if !ok {
		writeError(w, http.StatusConflict, "not_configured", "Setup has not been completed yet.")
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}

	if req.Username != cfg.AuthUsername || !auth.VerifyPassword(cfg.AuthPasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Incorrect username or password.")
		return
	}

	s.login(w)
	writeJSON(w, http.StatusOK, s.buildConfigResponse(true))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
