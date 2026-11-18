//go:build linux

package volume

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type nativeXFSQuotaPlatform struct{}

func (nativeXFSQuotaPlatform) Probe(ctx context.Context, mount, volume string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	if err := verifyXFSMountInfo(data, mount, volume); err != nil {
		return err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(volume, &stat); err != nil {
		return err
	}
	if stat.Type != unix.XFS_SUPER_MAGIC || stat.Bsize < 1024 {
		return fmt.Errorf("volume is not a supported XFS filesystem: %w", ErrStorageQuotaUnsupported)
	}
	return ctx.Err()
}

func (nativeXFSQuotaPlatform) ProbeMount(ctx context.Context, mount string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	if err := verifyXFSMountRoot(data, mount); err != nil {
		return err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(mount, &stat); err != nil {
		return err
	}
	if stat.Type != unix.XFS_SUPER_MAGIC || stat.Bsize < 1024 {
		return fmt.Errorf("mount is not a supported XFS filesystem: %w", ErrStorageQuotaUnsupported)
	}
	return ctx.Err()
}

func (nativeXFSQuotaPlatform) SyncMount(mount string) error {
	f, err := os.Open(mount)
	if err != nil {
		return err
	}
	syncErr := unix.Syncfs(int(f.Fd()))
	closeErr := f.Close()
	return errors.Join(syncErr, closeErr)
}

func (nativeXFSQuotaPlatform) Lock(ctx context.Context, file *os.File) (func(), error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return nil, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
