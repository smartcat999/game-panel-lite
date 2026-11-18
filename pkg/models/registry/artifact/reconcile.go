package artifact

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

func (o *operator) Reconcile(ctx context.Context, name string) error {
	art, err := o.db.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if art.Metadata.DeletionTimestamp != nil {
		return o.reconcileDelete(ctx, art)
	}
	if IsSyncLeaseExpired(art, time.Now().UTC()) {
		return o.recoverExpiredSyncLease(ctx, art)
	}
	return o.reconcileActive(ctx, art)
}

// IsSyncLeaseExpired reports whether an Artifact in Syncing phase has an absent or expired worker lease.
func IsSyncLeaseExpired(art *registryv1.Artifact, now time.Time) bool {
	if art == nil || art.Status.Phase != registryv1.ArtifactPhaseSyncing {
		return false
	}
	raw := art.Metadata.Annotations[registryv1.AnnotationSyncLeaseUntil]
	if raw == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return true
	}
	return !now.Before(until)
}

func (o *operator) recoverExpiredSyncLease(ctx context.Context, art *registryv1.Artifact) error {
	nextAnn := maps.Clone(art.Metadata.Annotations)
	delete(nextAnn, registryv1.AnnotationSyncClaimedBy)
	delete(nextAnn, registryv1.AnnotationSyncLeaseUntil)
	art.Metadata.Annotations = nextAnn
	updated, err := o.db.UpdateObject(ctx, art)
	if err != nil {
		return err
	}
	updated.Status.Phase = registryv1.ArtifactPhasePending
	updated.Status.Stage = registryv1.ArtifactStageQueued
	updated.Status.SpeedBytesPerSec = 0
	updated.Status.Message = "sync lease expired; queued for retry"
	res, err := o.db.UpdateStatus(ctx, updated)
	if err == nil && res != nil {
		o.publish(ctx, mq.EventStateChanged, res)
	}
	return err
}

func (o *operator) reconcileDelete(ctx context.Context, art *registryv1.Artifact) error {
	bound := art.Metadata.Annotations[registryv1.AnnotationBoundBlobDigest]
	if bound != "" {
		if _, err := o.blobDB.ReleaseRef(ctx, bound, art.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	nextAnn := maps.Clone(art.Metadata.Annotations)
	delete(nextAnn, registryv1.AnnotationBoundBlobDigest)
	nextFin := slices.DeleteFunc(append([]string(nil), art.Metadata.Finalizers...), func(f string) bool {
		return f == registryv1.FinalizerArtifactBlobRef
	})
	art.Metadata.Annotations = nextAnn
	art.Metadata.Finalizers = nextFin
	updated, err := o.db.UpdateObject(ctx, art)
	if err != nil {
		return err
	}
	if len(updated.Metadata.Finalizers) > 0 {
		return nil
	}
	if err := o.db.DeleteIfUID(ctx, updated.Metadata.Name, updated.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	if o.logs != nil {
		_ = o.logs.Reset(ctx, artifactLogKey(updated.Metadata.Name))
	}
	o.publish(ctx, mq.EventDeleted, updated)
	o.wakeRepository(ctx, updated.Spec.Repository)
	return nil
}

func (o *operator) reconcileActive(ctx context.Context, art *registryv1.Artifact) error {
	var desiredDigest string
	if art.Status.Phase == registryv1.ArtifactPhaseReady && art.Status.Descriptor != nil {
		desiredDigest = art.Status.Descriptor.Digest
	}
	bound := art.Metadata.Annotations[registryv1.AnnotationBoundBlobDigest]
	if desiredDigest == "" {
		return nil
	}
	if desiredDigest == bound {
		if o.clearSyncClaimIfPresent(ctx, art) {
			o.syncParentPackageMeta(ctx, art)
		}
		return nil
	}
	if _, err := o.blobDB.AcquireRef(ctx, desiredDigest, art.Metadata.UID); err != nil {
		return err
	}
	if bound != "" {
		if _, err := o.blobDB.ReleaseRef(ctx, bound, art.Metadata.UID); err != nil && !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	if art.Metadata.Annotations == nil {
		art.Metadata.Annotations = make(map[string]string, 1)
	}
	art.Metadata.Annotations[registryv1.AnnotationBoundBlobDigest] = desiredDigest
	delete(art.Metadata.Annotations, registryv1.AnnotationSyncClaimedBy)
	delete(art.Metadata.Annotations, registryv1.AnnotationSyncLeaseUntil)
	if _, err := o.db.UpdateObject(ctx, art); err != nil {
		return err
	}
	o.syncParentPackageMeta(ctx, art)
	o.wakeRepository(ctx, art.Spec.Repository)
	return nil
}

func (o *operator) clearSyncClaimIfPresent(ctx context.Context, art *registryv1.Artifact) bool {
	if art.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] == "" &&
		art.Metadata.Annotations[registryv1.AnnotationSyncLeaseUntil] == "" {
		o.syncParentPackageMeta(ctx, art)
		return false
	}
	delete(art.Metadata.Annotations, registryv1.AnnotationSyncClaimedBy)
	delete(art.Metadata.Annotations, registryv1.AnnotationSyncLeaseUntil)
	if _, err := o.db.UpdateObject(ctx, art); err != nil {
		return false
	}
	return true
}

func (o *operator) syncParentPackageMeta(ctx context.Context, art *registryv1.Artifact) {
	if o.pkgDB == nil || art == nil {
		return
	}
	pkgCoord := registryv1.FormatPackageName(art.Spec.Repository, art.Spec.Package)
	pkg, err := o.pkgDB.Get(ctx, pkgCoord)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			newPkg := &registryv1.Package{
				Repository:    art.Spec.Repository,
				Package:       art.Spec.Package,
				Title:         art.Spec.Package,
				LatestVersion: art.Spec.Version,
			}
			applyArtifactMetaToPackage(newPkg, art)
			_, _ = o.pkgDB.Create(ctx, newPkg)
		}
		return
	}
	if applyArtifactMetaToPackage(pkg, art) {
		_, _ = o.pkgDB.Update(ctx, pkg)
	}
}

func applyArtifactMetaToPackage(pkg *registryv1.Package, art *registryv1.Artifact) bool {
	changed := false
	if art.Status.Descriptor != nil && art.Status.Descriptor.SizeBytes > 0 && pkg.SizeBytes != art.Status.Descriptor.SizeBytes {
		pkg.SizeBytes = art.Status.Descriptor.SizeBytes
		changed = true
	}
	if len(art.Status.ModIDs) > 0 && !slices.Equal(pkg.ModIDs, art.Status.ModIDs) {
		pkg.ModIDs = append([]string(nil), art.Status.ModIDs...)
		changed = true
	}
	if len(art.Status.MapFolders) > 0 && !slices.Equal(pkg.MapFolders, art.Status.MapFolders) {
		pkg.MapFolders = append([]string(nil), art.Status.MapFolders...)
		changed = true
	}
	if len(art.Status.Dependencies) > 0 && len(pkg.Dependencies) == 0 {
		pkg.Dependencies = append([]registryv1.PackageDependency(nil), art.Status.Dependencies...)
		changed = true
	}
	if len(art.Status.ConfigSchema) > 0 && !reflect.DeepEqual(pkg.ConfigSchema, art.Status.ConfigSchema) {
		pkg.ConfigSchema = append([]registryv1.AddonConfigField(nil), art.Status.ConfigSchema...)
		changed = true
	}
	return changed
}
