package backup

import (
	"context"
	"io"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
)

// StoredArchive identifies exact bytes, not permission to restore them.
// ObjectVersion is the storage backend version, separate from Asset.Version.
type StoredArchive struct {
	StorageID     string
	ObjectKey     string
	ObjectVersion string
	Asset         assets.PublishedVersion
}

// ArchiveStore is consumed by regional backup orchestration. Upload's source
// must remain immutable throughout the call and stays owned by the caller.
// Open returns untrusted bytes: the caller must verify size and digest before
// passing a staged archive to RestoreArchiveChecked. Close releases the stream.
type ArchiveStore interface {
	Upload(context.Context, string, assets.PublishedVersion, io.ReaderAt) (StoredArchive, error)
	Open(context.Context, StoredArchive) (io.ReadCloser, error)
}
