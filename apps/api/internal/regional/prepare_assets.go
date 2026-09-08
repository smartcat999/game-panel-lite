package regional

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

// AssetSource must authorize the event's current regional access and its exact
// asset reference before returning bytes. Snapshot possession is not authority.
// Open must honor ctx; returned Close must unblock a concurrent Read.
type AssetSource interface {
	Open(context.Context, instances.RevisionAvailable, assets.PublishedVersion) (io.ReadCloser, error)
}

// AssetFiles takes ownership of the stream on every Put call and verifies size
// and digest before publishing. A nil error means that one file was published.
type AssetFiles interface {
	Put(context.Context, assets.PublishedVersion, io.ReadCloser) error
}

type AssetPreparer struct {
	RegionID                    string
	Source                      AssetSource
	Files                       AssetFiles
	MaxFiles                    int
	MaxFileBytes, MaxTotalBytes int64
	Timeout                     time.Duration
}

// Prepare validates the entire manifest before I/O, then prepares one stream at
// a time outside database transactions. Earlier successful files may remain on
// failure; no partial success grants permission to materialize or execute.
func (p AssetPreparer) Prepare(ctx context.Context, snapshot RevisionSnapshot) error {
	if p.RegionID == "" || p.Source == nil || p.Files == nil || p.MaxFiles <= 0 || p.MaxFileBytes <= 0 || p.MaxTotalBytes <= 0 || p.Timeout <= 0 {
		return errors.New("invalid asset preparation settings")
	}
	if err := snapshot.ValidateFor(snapshot.Event); err != nil {
		return err
	}
	if snapshot.Event.RegionID != p.RegionID {
		return ErrRevisionUnavailable
	}
	if len(snapshot.Assets) > p.MaxFiles {
		return errors.New("asset count exceeds preparation limit")
	}
	remaining := p.MaxTotalBytes
	for _, version := range snapshot.Assets {
		if version.SizeBytes > p.MaxFileBytes || version.SizeBytes > remaining {
			return errors.New("asset bytes exceed preparation limit")
		}
		remaining -= version.SizeBytes
	}
	workCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	for _, version := range snapshot.Assets {
		if err := workCtx.Err(); err != nil {
			return err
		}
		stream, err := p.Source.Open(workCtx, snapshot.Event, version)
		if err != nil {
			if stream != nil {
				_ = stream.Close()
			}
			return err
		}
		if stream == nil {
			return errors.New("asset source returned no stream")
		}
		if err := p.Files.Put(workCtx, version, stream); err != nil {
			return err
		}
	}
	return workCtx.Err()
}
