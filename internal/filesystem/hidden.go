package filesystem

import "strings"

// hiddenNames lists NAS/OS clutter to hide from listings, in addition to
// any dotfile/dotfolder. Extend this list as new clutter is found, or use
// Configure for a deployment-specific, environment-configured addition.
var hiddenNames = map[string]bool{
	"@eadir":                    true,
	"#recycle":                  true,
	"#snapshot":                 true,
	"$recycle.bin":              true,
	"system volume information": true,
	"lost+found":                true,
}

// extraHiddenNames, excludedPathPrefixes, and excludedExtensions are set
// once at startup via Configure, from deployment-specific exclusion
// config (EXTRA_HIDDEN_NAMES / EXCLUDED_PATHS /
// EXCLUDED_EXTENSIONS). Package-level rather than threaded through every
// Browse call, matching hiddenNames' existing style — this process loads
// its config once at startup, not per-request.
var (
	extraHiddenNames     = map[string]bool{}
	excludedPathPrefixes = []string{}
	excludedExtensions   = map[string]bool{}
)

// Configure sets deployment-specific additions to the built-in exclusion
// rules. Call once at startup, before serving traffic; not safe for
// concurrent reconfiguration.
func Configure(extraHidden, excludedPaths, excludedExts []string) {
	extraHiddenNames = toLowerSet(extraHidden)
	excludedExtensions = toLowerSet(excludedExts)

	prefixes := make([]string, 0, len(excludedPaths))
	for _, p := range excludedPaths {
		if cleaned, err := CleanRelPath(p); err == nil && cleaned != "" {
			prefixes = append(prefixes, cleaned)
		}
	}
	excludedPathPrefixes = prefixes
}

func toLowerSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[strings.ToLower(item)] = true
	}
	return set
}

// IsHidden reports whether an entry name should be hidden from browsing.
func IsHidden(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	lower := strings.ToLower(name)
	return hiddenNames[lower] || extraHiddenNames[lower]
}

// IsExcludedPath reports whether relPath (already cleaned, forward-slash
// separated) falls under a configured EXCLUDED_PATHS prefix — matching the
// prefix itself or anything nested under it.
func IsExcludedPath(relPath string) bool {
	for _, prefix := range excludedPathPrefixes {
		if relPath == prefix || strings.HasPrefix(relPath, prefix+"/") {
			return true
		}
	}
	return false
}

// IsExcludedExt reports whether name's extension is configured as
// excluded via EXCLUDED_EXTENSIONS, even though it's otherwise a
// recognized, playable video container.
func IsExcludedExt(name string) bool {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return false
	}
	return excludedExtensions[strings.ToLower(name[i:])]
}
