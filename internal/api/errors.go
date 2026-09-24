package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"playhead/internal/filesystem"
)

// ErrorResponse is this API's stable JSON error shape, used everywhere.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encoding json response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

// writeFSError maps a filesystem package error to the appropriate HTTP
// status and a client-safe message. It never echoes host paths.
func writeFSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, filesystem.ErrInvalidPath):
		writeError(w, http.StatusBadRequest, "invalid_path", "The requested path is invalid.")
	case errors.Is(err, filesystem.ErrOutsideRoot):
		writeError(w, http.StatusForbidden, "forbidden", "The requested path is outside the media root.")
	case errors.Is(err, filesystem.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "The requested item was not found.")
	case errors.Is(err, filesystem.ErrRootInaccessible):
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The media root is currently inaccessible.")
	default:
		slog.Error("unexpected filesystem error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
