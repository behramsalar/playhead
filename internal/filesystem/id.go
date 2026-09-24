package filesystem

import (
	"encoding/base64"
	"strings"
)

// EncodeVideoID implements this app's fixed video ID scheme: the
// base64url encoding (no padding) of "<rootID>/<relativePath>".
//
// Do not change this scheme in later phases; it is the app's public,
// filesystem-derived identifier for a video.
func EncodeVideoID(rootID, relPath string) string {
	raw := rootID + "/" + relPath
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeVideoID reverses EncodeVideoID and validates the embedded relative
// path exactly as any client-supplied path is validated — a decoded ID
// is untrusted input.
func DecodeVideoID(id string) (rootID, relPath string, err error) {
	raw, decErr := base64.RawURLEncoding.DecodeString(id)
	if decErr != nil {
		return "", "", ErrInvalidPath
	}

	s := string(raw)
	idx := strings.IndexByte(s, '/')
	if idx <= 0 || idx == len(s)-1 {
		// No root separator, empty root ID, or empty relative path.
		return "", "", ErrInvalidPath
	}

	rootID = s[:idx]
	relPath = s[idx+1:]

	cleaned, err := CleanRelPath(relPath)
	if err != nil {
		return "", "", err
	}
	if cleaned == "" {
		return "", "", ErrInvalidPath
	}

	return rootID, cleaned, nil
}
