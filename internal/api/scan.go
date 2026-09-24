package api

import "net/http"

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	if s.indexer == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The index is not available.")
		return
	}
	writeJSON(w, http.StatusOK, s.indexer.Status())
}

func (s *Server) handleScanTrigger(w http.ResponseWriter, r *http.Request) {
	if s.indexer == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "The index is not available.")
		return
	}
	s.indexer.TryStartScan(s.appCtx)
	writeJSON(w, http.StatusAccepted, s.indexer.Status())
}
