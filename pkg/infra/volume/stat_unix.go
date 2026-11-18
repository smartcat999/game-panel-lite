//go:build unix

package volume

import (
	"os"
	"syscall"
)

func hasSingleHardlink(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st != nil && st.Nlink == 1
}

func fileDeviceID(info os.FileInfo) (uint64, bool) {
	if info == nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, false
	}
	return uint64(st.Dev), true //nolint:unconvert // Darwin uses int32 for Dev; Linux uses uint64.
}
