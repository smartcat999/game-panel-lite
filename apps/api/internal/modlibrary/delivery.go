package modlibrary

import (
	"context"
	"errors"
	"os"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var ErrArtifactUnavailable = errors.New("artifact file is unavailable or changed")

type DeliveryRepository interface {
	ResolveArtifactForNode(context.Context, string, string, int, string, string, int64) (domain.ModFile, workload.Artifact, error)
}
type DeliveryFiles interface {
	OpenLibrary(domain.ModFile) (*os.File, error)
}
type Delivery struct {
	repo  DeliveryRepository
	files DeliveryFiles
}

func NewDelivery(repo DeliveryRepository, files DeliveryFiles) *Delivery {
	return &Delivery{repo: repo, files: files}
}

// Open validates both before and after opening the owned file under the caller's
// active execution lease. The caller owns the returned handle.
func (d *Delivery) Open(ctx context.Context, nodeID, uid string, generation int, id, holderID string, fence int64) (*os.File, workload.Artifact, error) {
	item, ref, err := d.repo.ResolveArtifactForNode(ctx, nodeID, uid, generation, id, holderID, fence)
	if err != nil {
		return nil, ref, err
	}
	file, err := d.files.OpenLibrary(item)
	if err != nil {
		return nil, ref, ErrArtifactUnavailable
	}
	fail := func(err error) (*os.File, workload.Artifact, error) { return nil, ref, errors.Join(err, file.Close()) }
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != ref.SizeBytes {
		return fail(ErrArtifactUnavailable)
	}
	current, currentRef, err := d.repo.ResolveArtifactForNode(ctx, nodeID, uid, generation, id, holderID, fence)
	if err != nil {
		return fail(err)
	}
	if currentRef != ref || current.ID != item.ID || current.OrganizationID != item.OrganizationID || current.ProviderKey != item.ProviderKey || current.FileName != item.FileName || current.ContentHash != item.ContentHash || current.SizeBytes != item.SizeBytes {
		return fail(ErrArtifactUnavailable)
	}
	return file, ref, nil
}
