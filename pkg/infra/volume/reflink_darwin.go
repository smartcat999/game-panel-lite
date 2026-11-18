//go:build darwin

package volume

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func tryCloneFileInRoot(srcPath string, root *os.Root, parentRel, tmpName, tmpRel string, perm os.FileMode) (bool, error) {
	srcDir, err := os.Open(filepath.Dir(srcPath))
	if err != nil {
		return false, err
	}
	defer func() { _ = srcDir.Close() }()

	dstDir, err := root.OpenFile(filepath.FromSlash(parentRel), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, err
	}
	defer func() { _ = dstDir.Close() }()

	if cloneErr := unix.Clonefileat(int(srcDir.Fd()), filepath.Base(srcPath), int(dstDir.Fd()), tmpName, unix.CLONE_NOFOLLOW|unix.CLONE_NOOWNERCOPY); cloneErr != nil {
		if isDarwinReflinkUnsupported(cloneErr) {
			return false, nil
		}
		return false, cloneErr
	}
	f, err := root.OpenFile(tmpRel, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		_ = root.Remove(tmpRel)
		return false, err
	}
	st, statErr := f.Stat()
	chmodErr := f.Chmod(perm)
	closeErr := f.Close()
	if statErr != nil || chmodErr != nil || closeErr != nil || !st.Mode().IsRegular() || !hasSingleHardlink(st) {
		_ = root.Remove(tmpRel)
		if err := errors.Join(statErr, chmodErr, closeErr); err != nil {
			return false, err
		}
		return false, errors.New("cloned entry is not an isolated regular file")
	}
	return true, nil
}

func isDarwinReflinkUnsupported(err error) bool {
	return errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EOPNOTSUPP) ||
		errors.Is(err, unix.EXDEV) ||
		errors.Is(err, unix.ENOSYS)
}
