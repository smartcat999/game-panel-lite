//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package docker

import (
	"errors"
	"os"
	"syscall"
)

func tryCreationLock(file *os.File) (func(), error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return nil, errCreateLockBusy
	}
	if err != nil {
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }, nil
}
