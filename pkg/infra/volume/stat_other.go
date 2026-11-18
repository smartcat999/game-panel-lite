//go:build !unix

package volume

import "os"

func hasSingleHardlink(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular()
}

func fileDeviceID(info os.FileInfo) (uint64, bool) {
	return 0, false
}
