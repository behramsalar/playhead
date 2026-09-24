package config

import (
	"os"
	"syscall"
)

// deviceIDFunc is deviceID, indirected through a variable so tests can
// simulate a genuinely separate mount (real device IDs can't be faked
// without an actual OS-level mount, which a unit test shouldn't need) —
// see mount_test.go.
var deviceIDFunc = deviceID

// SetDeviceIDFuncForTesting overrides the function DiscoverRoots uses to
// detect mount boundaries, for tests in *other* packages (e.g.
// internal/api) that need to exercise genuine multi-root behavior end to
// end without an actual separate OS-level mount, which a unit test
// shouldn't require. Returns a restore func; call it (typically via
// t.Cleanup) to put the real implementation back. Production code must
// never call this.
func SetDeviceIDFuncForTesting(fn func(path string) (dev uint64, ok bool)) (restore func()) {
	orig := deviceIDFunc
	deviceIDFunc = fn
	return func() { deviceIDFunc = orig }
}

// deviceID returns path's filesystem device number, used by DiscoverRoots
// to detect genuine mount boundaries: a directory bind-mounted from a
// separate host path reports a different device than its parent, while a
// plain subfolder on the same filesystem reports the same one — this is
// the same distinction tools like `findmnt`/`mountpoint` rely on. ok is
// false if this can't be determined (stat failure, or a platform whose
// os.FileInfo.Sys() isn't *syscall.Stat_t, e.g. Windows) — callers must
// then fall back to never promoting a subdirectory, the same behavior as
// if no separate mounts existed at all. This app only ever runs on Linux
// (Docker) or macOS (local dev), both of which populate Stat_t.Dev.
func deviceID(path string) (dev uint64, ok bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}
