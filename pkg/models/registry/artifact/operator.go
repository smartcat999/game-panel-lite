package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	blobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/blob"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Artifact], error)
	Get(ctx context.Context, name string) (*registryv1.Artifact, error)
	GetExisting(ctx context.Context, name string) (*registryv1.Artifact, error)
	HydrateMetadata(ctx context.Context, name string) (*registryv1.Artifact, error)
	RetryProxy(ctx context.Context, name string) (*registryv1.Artifact, error)
	Create(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error)
	Update(ctx context.Context, name string, in *registryv1.Artifact) (*registryv1.Artifact, error)
	UpdateStatus(ctx context.Context, name string, in *registryv1.Artifact) (*registryv1.Artifact, error)
	Delete(ctx context.Context, name string) error
	Reconcile(ctx context.Context, name string) error
	AppendLogs(ctx context.Context, name string, req *registryv1.ArtifactLogChunk) error
	StreamLogs(ctx context.Context, name string, follow bool, tailLines int) (io.ReadCloser, error)
}

type operator struct {
	db         *artifactdb.DB
	repoDB     *repositorydb.DB
	upstreamDB *upstreamdb.DB
	pkgDB      *pkgdb.DB
	blobDB     *blobdb.DB
	cache      cache.Interface
	bus        mq.Interface
	logs       tasklog.Interface
}

func NewOperator(reg *repo.Registry, bus mq.Interface, caches ...cache.Interface) Operator {
	var kv cache.Interface
	if len(caches) > 0 && caches[0] != nil {
		kv = caches[0]
	} else {
		kv = cache.Default()
	}
	op := &operator{bus: bus, cache: kv, logs: tasklog.New(kv, bus)}
	if reg != nil {
		op.db = artifactdb.NewDB(reg.Pool())
		op.repoDB = repositorydb.NewDB(reg.Pool())
		op.upstreamDB = upstreamdb.NewDB(reg.Pool())
		op.pkgDB = pkgdb.NewDB(reg.Pool())
		op.blobDB = blobdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) publish(ctx context.Context, evType mq.EventType, a *registryv1.Artifact) {
	if o.bus == nil || a == nil {
		return
	}
	ev := mq.NewEvent(evType, registryv1.GroupVersion, registryv1.KindArtifact, "", a.Metadata.Name, a.Metadata.Generation, a.Metadata.ResourceVersion)
	ev.Topic = registryv1.TopicArtifacts
	_ = o.bus.Publish(ctx, registryv1.TopicArtifacts, ev)
}

func (o *operator) wakeRepository(ctx context.Context, repoName string) {
	if o.bus == nil || repoName == "" {
		return
	}
	ev := mq.NewEvent(mq.EventReconcile, registryv1.GroupVersion, registryv1.KindRepository, "", repoName, 0, "")
	ev.Topic = registryv1.TopicRepositories
	_ = o.bus.Publish(ctx, registryv1.TopicRepositories, ev)
}

func normalizeArtifactURLs(art *registryv1.Artifact) {
	if art != nil {
		art.Status.UpstreamURL = addonmirror.NormalizeSteamNewsURL(art.Status.UpstreamURL)
	}
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Artifact], error) {
	list, err := o.db.List(ctx, opts)
	if err == nil && list != nil {
		for _, item := range list.Items {
			normalizeArtifactURLs(item)
		}
	}
	return list, err
}

func (o *operator) GetExisting(ctx context.Context, name string) (*registryv1.Artifact, error) {
	art, err := o.db.Get(ctx, name)
	if err == nil {
		normalizeArtifactURLs(art)
	}
	return art, err
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.Artifact, error) {
	art, err := o.db.Get(ctx, name)
	if err == nil {
		normalizeArtifactURLs(art)
		return art, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return art, err
	}
	pulled, pErr := o.pullThroughProxyArtifact(ctx, name)
	normalizeArtifactURLs(pulled)
	return pulled, pErr
}

func (o *operator) Create(ctx context.Context, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	cp := *in
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if strings.TrimSpace(cp.Metadata.Annotations[registryv1.AnnotationRetryRequestedAt]) != "" {
		return o.RetryProxy(ctx, cp.Metadata.Name)
	}
	if _, err := o.repoDB.Get(ctx, cp.Spec.Repository); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("%w: repository %q not found", models.ErrBadRequest, cp.Spec.Repository)
		}
		return nil, err
	}
	if err := o.prepareInitialDescriptor(ctx, &cp); err != nil {
		return nil, err
	}
	if !slices.Contains(cp.Metadata.Finalizers, registryv1.FinalizerArtifactBlobRef) {
		cp.Metadata.Finalizers = append(cp.Metadata.Finalizers, registryv1.FinalizerArtifactBlobRef)
	}
	o.ensurePackageRow(ctx, &cp)
	created, err := o.db.Create(ctx, &cp)
	if err != nil {
		if errors.Is(err, db.ErrAlreadyExists) || errors.Is(err, db.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if created.Status.Phase == registryv1.ArtifactPhasePending && o.logs != nil {
		_ = o.logs.Reset(ctx, artifactLogKey(created.Metadata.Name))
	}
	o.publish(ctx, mq.EventCreated, created)
	o.wakeRepository(ctx, created.Spec.Repository)
	return created, nil
}

func (o *operator) prepareInitialDescriptor(ctx context.Context, cp *registryv1.Artifact) error {
	if cp.Spec.Descriptor == nil {
		cp.Status.Phase = registryv1.ArtifactPhasePending
		cp.Status.Stage = registryv1.ArtifactStageQueued
		return nil
	}
	b, err := o.blobDB.Get(ctx, cp.Spec.Descriptor.Digest)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return fmt.Errorf("%w: referenced blob %q does not exist", models.ErrBadRequest, cp.Spec.Descriptor.Digest)
		}
		return err
	}
	cp.Spec.Descriptor.Digest = b.Digest
	cp.Spec.Descriptor.SizeBytes = b.SizeBytes
	if cp.Spec.Descriptor.ArchiveFormat == "" {
		cp.Spec.Descriptor.ArchiveFormat = cp.Spec.ArchiveFormat
	}
	now := time.Now().UTC()
	cp.Status.Phase = registryv1.ArtifactPhaseReady
	cp.Status.Stage = registryv1.ArtifactStageCompleted
	cp.Status.Progress = 100
	cp.Status.Descriptor = cp.Spec.Descriptor
	cp.Status.SyncedAt = &now
	return nil
}

func (o *operator) ensurePackageRow(ctx context.Context, cp *registryv1.Artifact) {
	pkgCoord := registryv1.FormatPackageName(cp.Spec.Repository, cp.Spec.Package)
	if pkg, err := o.pkgDB.Get(ctx, pkgCoord); err == nil {
		pkg.LatestVersion = cp.Spec.Version
		_, _ = o.pkgDB.Update(ctx, pkg)
		return
	}
	_, _ = o.pkgDB.Create(ctx, &registryv1.Package{
		Repository:    cp.Spec.Repository,
		Package:       cp.Spec.Package,
		Title:         cp.Spec.Package,
		LatestVersion: cp.Spec.Version,
	})
}

func preserveArtifactServerMeta(dst *metav1.ObjectMeta, existing metav1.ObjectMeta) {
	if len(dst.Finalizers) == 0 {
		dst.Finalizers = append([]string(nil), existing.Finalizers...)
	} else if slices.Contains(existing.Finalizers, registryv1.FinalizerArtifactBlobRef) &&
		!slices.Contains(dst.Finalizers, registryv1.FinalizerArtifactBlobRef) {
		dst.Finalizers = append(append([]string(nil), dst.Finalizers...), registryv1.FinalizerArtifactBlobRef)
	}
	dst.Annotations = maps.Clone(dst.Annotations)
	if bound := existing.Annotations[registryv1.AnnotationBoundBlobDigest]; bound != "" {
		if dst.Annotations == nil {
			dst.Annotations = make(map[string]string, 1)
		}
		dst.Annotations[registryv1.AnnotationBoundBlobDigest] = bound
	} else if dst.Annotations != nil {
		delete(dst.Annotations, registryv1.AnnotationBoundBlobDigest)
	}
	for _, key := range []string{registryv1.AnnotationSyncClaimedBy, registryv1.AnnotationSyncLeaseUntil} {
		if dst.Annotations != nil && dst.Annotations[key] == "" {
			delete(dst.Annotations, key)
		}
	}
}

func (o *operator) Update(ctx context.Context, name string, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	prevRetry := strings.TrimSpace(existing.Metadata.Annotations[registryv1.AnnotationRetryRequestedAt])
	prevHydrate := strings.TrimSpace(existing.Metadata.Annotations[registryv1.AnnotationHydrateRequestedAt])
	cp := *in
	cp.Metadata.Name = existing.Metadata.Name
	cp.Spec.Repository = existing.Spec.Repository
	cp.Spec.Package = existing.Spec.Package
	cp.Spec.Version = existing.Spec.Version
	cp.Metadata.DeletionTimestamp = existing.Metadata.DeletionTimestamp
	preserveArtifactServerMeta(&cp.Metadata, existing.Metadata)
	nextRetry := strings.TrimSpace(cp.Metadata.Annotations[registryv1.AnnotationRetryRequestedAt])
	nextHydrate := strings.TrimSpace(cp.Metadata.Annotations[registryv1.AnnotationHydrateRequestedAt])
	updated, err := o.db.Update(ctx, &cp)
	if err != nil {
		return nil, err
	}
	if nextRetry != "" && nextRetry != prevRetry {
		return o.RetryProxy(ctx, updated.Metadata.Name)
	}
	if nextHydrate != "" && nextHydrate != prevHydrate {
		return o.HydrateMetadata(ctx, updated.Metadata.Name)
	}
	o.publish(ctx, mq.EventUpdated, updated)
	return updated, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, in *registryv1.Artifact) (*registryv1.Artifact, error) {
	cp := *in
	cp.Metadata.Name = name
	cp.Status.UpstreamURL = addonmirror.NormalizeSteamNewsURL(cp.Status.UpstreamURL)
	updated, err := o.db.UpdateStatus(ctx, &cp)
	if err != nil {
		return nil, err
	}
	normalizeArtifactURLs(updated)
	o.publish(ctx, mq.EventStateChanged, updated)
	o.wakeRepository(ctx, updated.Spec.Repository)
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return err
	}
	if len(existing.Metadata.Finalizers) == 0 {
		if err := o.db.Delete(ctx, name); err != nil {
			return err
		}
		o.publish(ctx, mq.EventDeleted, existing)
		o.wakeRepository(ctx, existing.Spec.Repository)
		return nil
	}
	if existing.Metadata.DeletionTimestamp == nil {
		now := time.Now().UTC()
		existing.Metadata.DeletionTimestamp = &now
		updated, err := o.db.UpdateObject(ctx, existing)
		if err != nil {
			return err
		}
		existing = updated
	}
	o.publish(ctx, mq.EventDeleted, existing)
	return nil
}
