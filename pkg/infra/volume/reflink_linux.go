//go:build linux

package volume

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func tryCloneFileInRoot(srcPath string, root *os.Root, _, _, tmpRel string, perm os.FileMode) (bool, error) {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return false, err
	}
	defer func() { _ = srcFile.Close() }()

	tmpFile, err := root.OpenFile(tmpRel, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return false, err
	}
	if cloneErr := unix.IoctlFileClone(int(tmpFile.Fd()), int(srcFile.Fd())); cloneErr != nil {
		_ = tmpFile.Close()
		_ = root.Remove(tmpRel)
		if isLinuxReflinkUnsupported(cloneErr) {
			return false, nil
		}
		return false, cloneErr
	}
	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		_ = root.Remove(tmpRel)
		return false, err
	}
	if err := tmpFile.Close(); err != nil {
		_ = root.Remove(tmpRel)
		return false, err
	}
	return true, nil
}

func isLinuxReflinkUnsupported(err error) bool {
	return errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.EXDEV) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.ENOSYS) ||
		errors.Is(err, unix.ENOTTY) ||
		errors.Is(err, unix.EPERM) ||
		errors.Is(err, unix.EACCES)
}
