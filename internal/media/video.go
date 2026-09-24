// Package media identifies recognized video files and their HTTP content
// types. Codec/container inspection (ffprobe) arrives in Phase 2.
package media

import "strings"

// contentTypes maps recognized extensions (lowercase, with leading dot) to
// their HTTP content type. Extensions not listed here still browse and
// stream, falling back to application/octet-stream.
var contentTypes = map[string]string{
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".ts":   "video/mp2t",
}

// IsVideoExt reports whether name has a recognized video extension,
// matched case-insensitively.
func IsVideoExt(name string) bool {
	_, ok := contentTypes[strings.ToLower(ext(name))]
	return ok
}

// ContentType returns the HTTP content type for name's extension, falling
// back to application/octet-stream for unrecognized extensions.
func ContentType(name string) string {
	if ct, ok := contentTypes[strings.ToLower(ext(name))]; ok {
		return ct
	}
	return "application/octet-stream"
}

func ext(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[i:]
}
