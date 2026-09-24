package filesystem_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"playhead/internal/filesystem"
)

func TestVideoIDRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		rootID  string
		relPath string
	}{
		{"simple", "main", "movie.mp4"},
		{"nested", "main", "Shows/Season 1/ep 01.mkv"},
		{"unicode and spaces", "main", "日本語 movie – final (2024).mp4"},
		{"emoji", "main", "🎬 clip.mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := filesystem.EncodeVideoID(tc.rootID, tc.relPath)
			gotRoot, gotPath, err := filesystem.DecodeVideoID(id)
			if err != nil {
				t.Fatalf("DecodeVideoID(%q) error: %v", id, err)
			}
			if gotRoot != tc.rootID || gotPath != tc.relPath {
				t.Fatalf("got (%q, %q), want (%q, %q)", gotRoot, gotPath, tc.rootID, tc.relPath)
			}
		})
	}
}

func TestDecodeVideoIDMalformed(t *testing.T) {
	cases := []string{
		"not-valid-base64!!!",
		"====",
		base64.StdEncoding.EncodeToString([]byte("main/movie.mp4")), // wrong alphabet (padded, standard)
		base64.RawURLEncoding.EncodeToString([]byte("no-slash-here")),
		base64.RawURLEncoding.EncodeToString([]byte("/leadingslash.mp4")),
		base64.RawURLEncoding.EncodeToString([]byte("main/")),
	}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			_, _, err := filesystem.DecodeVideoID(id)
			if err == nil {
				t.Fatalf("DecodeVideoID(%q) expected error, got nil", id)
			}
		})
	}
}

func TestDecodeVideoIDTraversal(t *testing.T) {
	id := base64.RawURLEncoding.EncodeToString([]byte("main/../../etc/passwd"))
	_, _, err := filesystem.DecodeVideoID(id)
	if !errors.Is(err, filesystem.ErrInvalidPath) {
		t.Fatalf("got %v, want ErrInvalidPath", err)
	}
}
