package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

var errCreateLockBusy = errors.New("instance creation lock is held")

// The lock inode is retained: unlinking it would let another process lock a
// different inode while a waiter still holds the old one.
func lockInstanceCreation(ctx context.Context, dataDir, serverID string) (func(), error) {
	if _, err := containerName(serverID); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := root.MkdirAll(".creation-locks", 0700); err != nil {
		return nil, err
	}
	relative := filepath.Join(".creation-locks", serverID+".lock")
	if err := rejectSymlink(root, relative); err != nil {
		return nil, err
	}
	file, err := root.OpenFile(relative, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			file.Close()
			return nil, err
		}
		unlock, err := tryCreationLock(file)
		if err == nil {
			return func() { unlock(); file.Close() }, nil
		}
		if !errors.Is(err, errCreateLockBusy) {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
