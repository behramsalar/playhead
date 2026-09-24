// Package filesystem implements the security boundary for all media access:
// path containment, video ID encoding, hidden-entry filtering, and folder
// browsing. Every filesystem read the API performs goes through here.
package filesystem

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
)

// Sentinel errors mapped to HTTP status codes by the API layer.
var (
	ErrInvalidPath      = errors.New("invalid path")
	ErrOutsideRoot      = errors.New("path escapes root")
	ErrNotFound         = errors.New("not found")
	ErrRootInaccessible = errors.New("root inaccessible")
)

// CleanRelPath validates and normalizes a client-supplied relative path.
//
// It rejects absolute paths, null bytes, backslashes, and any form of ".."
// traversal (including nested/nonobvious ones like "a/../../x") rather than
// silently clamping them to the root. The result always uses forward
// slashes and has no leading slash.
func CleanRelPath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if strings.ContainsRune(p, 0) {
		return "", ErrInvalidPath
	}
	if strings.Contains(p, "\\") {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	cleaned := path.Clean(p)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return "", ErrInvalidPath
	}
	return cleaned, nil
}

// Resolve validates relPath against rootPath, resolves symlinks, and
// verifies the canonical result is still inside the canonical root.
//
// It returns the canonical absolute path. existed reports whether the path
// currently exists on disk; when it does not (but its parent resolves
// safely inside the root), Resolve returns ErrNotFound.
func Resolve(rootPath, relPath string) (absPath string, err error) {
	cleaned, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}

	canonicalRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", ErrRootInaccessible
	}

	target := rootPath
	if cleaned != "" {
		target = filepath.Join(rootPath, filepath.FromSlash(cleaned))
	}

	canonical, existed, err := resolveCanonical(target)
	if err != nil {
		return "", err
	}

	if !isWithin(canonical, canonicalRoot) {
		return "", ErrOutsideRoot
	}
	if !existed {
		return "", ErrNotFound
	}
	return canonical, nil
}

// resolveCanonical resolves symlinks in absPath. When the final component
// does not exist, it resolves the parent instead and reports existed=false,
// so callers can still verify containment before reporting 404.
func resolveCanonical(absPath string) (canonical string, existed bool, err error) {
	canonical, err = filepath.EvalSymlinks(absPath)
	if err == nil {
		return canonical, true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false, err
	}

	parent := filepath.Dir(absPath)
	canonicalParent, perr := filepath.EvalSymlinks(parent)
	if perr != nil {
		if errors.Is(perr, fs.ErrNotExist) {
			return "", false, ErrNotFound
		}
		return "", false, perr
	}
	return filepath.Join(canonicalParent, filepath.Base(absPath)), false, nil
}

func isWithin(target, root string) bool {
	if target == root {
		return true
	}
	return strings.HasPrefix(target, root+string(filepath.Separator))
}
