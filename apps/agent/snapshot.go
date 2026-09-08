package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io/fs"
	"os"

	"github.com/smartcat999/game-panel-lite/internal/archive"
	"github.com/smartcat999/game-panel-lite/internal/worker"
)

type stoppedSnapshotRuntime interface {
	ReadStoppedData(context.Context, worker.State, func(context.Context, fs.FS) error) error
}

type snapshotOptions struct {
	directory string // private, caller-configured staging directory
	subtree   string
	metadata  archive.Metadata
	maxBytes  int64 // compressed archive limit, including ZIP metadata
}

type preparedSnapshot struct {
	file   *os.File // reopened read-only; no partial archive is returned
	size   int64
	sha256 string
}

func (s *preparedSnapshot) ReadAt(p []byte, offset int64) (int, error) {
	return s.file.ReadAt(p, offset)
}
func (s *preparedSnapshot) Close() error {
	return errors.Join(s.file.Close(), os.Remove(s.file.Name()))
}

// prepareStoppedSnapshot requires an already bounded execution context and a
// checker bound to this task/assignment. It neither acquires nor extends grants.
// Runtime protects lifecycle mutations; only a successful return can be offered
// for delivery. The caller owns Close and must retain the file until delivery.
func prepareStoppedSnapshot(ctx context.Context, runtime stoppedSnapshotRuntime, observed worker.State, o snapshotOptions, authorize func(context.Context) error) (result *preparedSnapshot, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("snapshot execution deadline required")
	}
	if runtime == nil || authorize == nil || o.directory == "" || o.maxBytes < 1 || !fs.ValidPath(o.subtree) || o.metadata.Validate() != nil {
		return nil, errors.New("invalid snapshot preparation")
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(o.directory, ".snapshot-*.zip")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer func() {
		if result == nil {
			_ = file.Close()
			_ = os.Remove(name)
		}
	}()
	writer := &snapshotWriter{file: file, hash: sha256.New(), remaining: o.maxBytes, ctx: ctx}
	err = runtime.ReadStoppedData(ctx, observed, func(readCtx context.Context, source fs.FS) error {
		// Recheck after waiting for the lifecycle lock, before reading any bytes.
		if err := authorize(readCtx); err != nil {
			return err
		}
		return archive.Write(readCtx, writer, source, o.subtree, &o.metadata)
	})
	if err != nil {
		return nil, err
	}
	if err := authorize(ctx); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	readonly, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = readonly.Close()
		return nil, err
	}
	return &preparedSnapshot{file: readonly, size: o.maxBytes - writer.remaining, sha256: hex.EncodeToString(writer.hash.Sum(nil))}, nil
}

type snapshotWriter struct {
	file      *os.File
	hash      hash.Hash
	remaining int64
	ctx       context.Context
}

func (w *snapshotWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > w.remaining {
		return 0, errors.New("snapshot archive exceeds size limit")
	}
	n, err := w.file.Write(p)
	_, _ = w.hash.Write(p[:n])
	w.remaining -= int64(n)
	return n, err
}
