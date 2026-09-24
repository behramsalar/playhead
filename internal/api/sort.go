package api

import (
	"sort"

	"playhead/internal/filesystem"
)

// sortVideos orders videos in place per the requested sort key. Unknown
// or empty keys fall back to natural name order (the Phase 1 default).
// duration/size sort ascending with unindexed (nil) values sorted last,
// regardless of direction, so an unprobed file doesn't jump to the front.
// modified sorts newest-first, matching common "recently changed" UX.
func sortVideos(videos []videoEntryDTO, sortBy string) {
	switch sortBy {
	case "modified":
		sort.SliceStable(videos, func(i, j int) bool {
			return videos[i].Modified.After(videos[j].Modified)
		})
	case "added":
		// Newest-first, like "modified"; a video with no index row yet
		// (AddedAt nil) has no known add time, so it sorts last rather
		// than being guessed at.
		sort.SliceStable(videos, func(i, j int) bool {
			a, b := videos[i].AddedAt, videos[j].AddedAt
			if a == nil || b == nil {
				return a != nil
			}
			return a.After(*b)
		})
	case "duration":
		sort.SliceStable(videos, func(i, j int) bool {
			a, b := videos[i].Duration, videos[j].Duration
			if a == nil || b == nil {
				return a != nil // known durations sort before unknown ones
			}
			return *a < *b
		})
	case "size":
		sort.SliceStable(videos, func(i, j int) bool {
			return videos[i].Size < videos[j].Size
		})
	default: // "name" or unrecognized
		sort.SliceStable(videos, func(i, j int) bool {
			return filesystem.NaturalLess(videos[i].Name, videos[j].Name)
		})
	}
}
