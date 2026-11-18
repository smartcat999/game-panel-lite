//go:build !linux && !darwin

package volume

import "os"

func tryCloneFileInRoot(_ string, _ *os.Root, _, _, _ string, _ os.FileMode) (bool, error) {
	return false, nil
}
