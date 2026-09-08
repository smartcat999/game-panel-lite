// Package assetfiles stores verified immutable bytes in a private regional
// directory. Callers must authorize manifests before using this local adapter.
package assetfiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
)

var ErrContentMismatch = errors.New("asset bytes do not match published version")

type Store struct {
	root     *os.Root
	maxBytes int64
}

// New opens an existing, exclusively managed regional storage directory. The
// directory is deployment configuration, never a path supplied by a tenant.
func New(directory string, maxBytes int64) (*Store, error) {
	if maxBytes <= 0 {
		return nil, errors.New("positive asset byte limit required")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Store{root: root, maxBytes: maxBytes}, nil
}

func (s *Store) Close() error { return s.root.Close() }

// Put takes ownership of source. Close must be safe during Read and unblock it
// (as with an HTTP response body). Cancellation closes the source; partial bytes
// never become a readable asset. Concurrent identical publications are safe.
func (s *Store) Put(ctx context.Context, version assets.PublishedVersion, source io.ReadCloser) error {
	if source == nil {
		return errors.New("asset source is required")
	}
	closeSource := sync.OnceValue(source.Close)
	defer closeSource()
	stop := context.AfterFunc(ctx, func() { _ = closeSource() })
	defer stop()
	name, err := s.name(version)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	temporary := "pending-" + rand.Text()
	file, err := s.root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(temporary)
	defer file.Close()
	if err := errors.Join(verify(ctx, source, file, version), closeSource(), ctx.Err()); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.root.Rename(temporary, name); err != nil {
		return err
	}
	// Persist the directory entry before reporting publication success. If this
	// fails, bytes may already be visible; retrying the same manifest is safe.
	directory, err := s.root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

// Open revalidates stored bytes before returning a descriptor at offset zero.
// A private storage directory is required; external in-place writers are not
// supported. Returned files are read-only and must be closed by the caller.
func (s *Store) Open(ctx context.Context, version assets.PublishedVersion) (io.ReadCloser, error) {
	name, err := s.name(version)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := s.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrContentMismatch
	}
	file, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (io.ReadCloser, error) { return nil, errors.Join(err, file.Close()) }
	info, err = file.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || info.Size() != version.SizeBytes {
		return fail(ErrContentMismatch)
	}
	if err := verify(ctx, file, io.Discard, version); err != nil {
		return fail(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return file, nil
}

func (s *Store) name(version assets.PublishedVersion) (string, error) {
	if version.Validate() != nil || version.SizeBytes > s.maxBytes {
		return "", assets.ErrInvalidVersion
	}
	// All identity fields participate, including tenant and version. No ID or
	// user-controlled path becomes a filesystem component.
	encoded, err := json.Marshal(version)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "asset-" + hex.EncodeToString(digest[:]), nil
}

func verify(ctx context.Context, source io.Reader, target io.Writer, version assets.PublishedVersion) error {
	digest := sha256.New()
	reader := contextReader{ctx: ctx, source: source}
	written, err := io.Copy(io.MultiWriter(target, digest), io.LimitReader(reader, version.SizeBytes))
	if err != nil {
		return err
	}
	if written != version.SizeBytes {
		return ErrContentMismatch
	}
	var probe [1]byte
	n, err := io.ReadFull(reader, probe[:])
	if n != 0 {
		return ErrContentMismatch
	}
	if err != io.EOF {
		return err
	}
	if hex.EncodeToString(digest.Sum(nil)) != version.SHA256 {
		return ErrContentMismatch
	}
	return ctx.Err()
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
