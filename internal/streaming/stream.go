// Package streaming serves video files with HTTP range support.
package streaming

import (
	"net/http"
	"os"

	"playhead/internal/media"
)

// ServeFile serves absPath (already validated and resolved by the caller)
// with range support via http.ServeContent, which handles conditional
// requests, Content-Range, and 416 responses for us. The file is opened
// read-only and never buffered whole into memory.
func ServeFile(w http.ResponseWriter, r *http.Request, absPath, name string) error {
	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", media.ContentType(name))
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, name, info.ModTime(), f)
	return nil
}
