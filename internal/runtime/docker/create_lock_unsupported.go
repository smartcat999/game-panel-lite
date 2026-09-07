//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package docker

import (
	"fmt"
	"os"
)

func tryCreationLock(*os.File) (func(), error) {
	return nil, fmt.Errorf("cross-process instance creation locking is unsupported on this platform")
}
