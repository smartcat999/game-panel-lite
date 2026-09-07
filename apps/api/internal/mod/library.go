package mod

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

var ErrLibraryFile = errors.New("invalid workspace library file")
var ErrUploadTooLarge = errors.New("mod upload exceeds size limit")

// StoredFile describes immutable bytes, not a database commit. Callers publish
// metadata only after success and retain orphan cleanup/retry responsibility.
type StoredFile struct {
	SizeBytes   int64
	ContentHash string
}

// PutLibrary publishes a complete file once. Reusing a mod ID never replaces its
// contents, even concurrently or with a different filename. maxBytes is supplied
// by the application's upload policy; no provider size limit lives here.
func (s *Service) PutLibrary(ctx context.Context, item domain.ModFile, reader io.Reader, maxBytes int64) (StoredFile, error) {
	if maxBytes <= 0 {
		return StoredFile{}, ErrLibraryFile
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	root, err := s.libraryRoot(item, true)
	if err != nil {
		return StoredFile{}, err
	}
	defer root.Close()
	temp := ".upload-" + uuid.NewString()
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return StoredFile{}, err
	}
	defer root.Remove(temp)
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(&contextReader{ctx: ctx, reader: reader}, maxBytes))
	if copyErr == nil {
		var probe [1]byte
		n, err := io.ReadFull(&contextReader{ctx: ctx, reader: reader}, probe[:])
		if n != 0 {
			copyErr = fmt.Errorf("%w: %d bytes", ErrUploadTooLarge, maxBytes)
		} else if err != nil && err != io.EOF {
			copyErr = err
		}
	}
	if copyErr == nil {
		copyErr = ctx.Err()
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil {
		return StoredFile{}, copyErr
	}
	if closeErr != nil {
		return StoredFile{}, closeErr
	}
	if err := ctx.Err(); err != nil {
		return StoredFile{}, err
	}
	// Link is an atomic no-replace publication on supported local filesystems.
	// Filesystems without hard links fail explicitly; no overwrite fallback.
	if err := root.Link(temp, "content"); err != nil {
		return StoredFile{}, err
	}
	return StoredFile{SizeBytes: size, ContentHash: hex.EncodeToString(hash.Sum(nil))}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// OpenLibrary returns a handle confined to the record directory. It never
// returns a host path for callers to reopen outside the confinement mechanism.
func (s *Service) OpenLibrary(item domain.ModFile) (*os.File, error) {
	root, err := s.libraryRoot(item, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("content")
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrLibraryFile
	}
	file, err := root.Open("content")
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
		file.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrLibraryFile
	}
	return file, nil
}

// RemoveLibrary only removes this record's published bytes. The caller must
// authorize deletion and coordinate metadata/references before invoking it.
func (s *Service) RemoveLibrary(item domain.ModFile) error {
	root, err := s.libraryRoot(item, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Lstat("content")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrLibraryFile
	}
	return root.Remove("content")
}

func (s *Service) libraryRoot(item domain.ModFile, create bool) (*os.Root, error) {
	if item.OrganizationID == "" || item.ID == "" || item.InstanceID != "unassigned" || item.ProviderKey == "" {
		return nil, ErrLibraryFile
	}
	if _, err := s.validateFile(item.ProviderKey, item.FileName); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.dataDir)
	if err != nil {
		return nil, err
	}
	// Hash opaque identities so case-insensitive filesystems, separators and
	// platform-reserved names cannot alias another workspace's directory.
	for _, part := range []string{"mod-library", libraryKey(item.OrganizationID), libraryKey(string(item.ProviderKey)), libraryKey(item.ID)} {
		next, err := libraryChild(root, part, create)
		root.Close()
		if err != nil {
			return nil, err
		}
		root = next
	}
	return root, nil
}
func libraryKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func libraryChild(parent *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := parent.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrLibraryFile
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	actual, err := child.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		child.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrLibraryFile
	}
	return child, nil
}
