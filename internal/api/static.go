package api

import (
	"io/fs"
	"net/http"
	"path"
)

// handleStatic serves the embedded frontend build. Any path that doesn't
// correspond to a real file is rewritten to "/" so the SPA's own router
// handles it and reloads/deep links survive.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if !s.hasFE {
		writeError(w, http.StatusInternalServerError, "internal_error", "The frontend has not been built.")
		return
	}

	fileServer := http.FileServer(http.FS(s.dist))

	cleaned := path.Clean(r.URL.Path)
	rel := cleaned[1:] // strip leading slash; fs.FS paths are relative

	if rel == "" {
		fileServer.ServeHTTP(w, r)
		return
	}
	if _, err := fs.Stat(s.dist, rel); err != nil {
		// Not a real asset: hand the SPA's own router the request by
		// serving index.html, without changing the browser's URL.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
		return
	}
	fileServer.ServeHTTP(w, r)
}
