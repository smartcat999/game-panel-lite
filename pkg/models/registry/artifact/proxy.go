package artifact

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

func (o *operator) RetryProxy(ctx context.Context, name string) (*registryv1.Artifact, error) {
	if repoName, _, reqVer, pErr := registryv1.ParseArtifactName(name); pErr == nil && addonmirror.IsLatestAlias(reqVer) {
		if repoObj, rErr := o.repoDB.Get(ctx, repoName); rErr == nil &&
			repoObj.Spec.Mode == registryv1.RepositoryModeProxy &&
			repoObj.Spec.Format != registryv1.FormatServerPackage {
			return o.pullThroughProxy(ctx, name, true)
		}
	}
	art, err := o.db.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return o.pullThroughProxy(ctx, name, true)
		}
		return nil, err
	}
	if art.Metadata.DeletionTimestamp != nil || art.Status.Phase == registryv1.ArtifactPhaseSyncing || art.Status.Phase == registryv1.ArtifactPhasePending || art.Spec.Descriptor != nil {
		return art, nil
	}
	repoObj, err := o.repoDB.Get(ctx, art.Spec.Repository)
	if err != nil {
		return nil, err
	}
	if repoObj.Spec.Mode != registryv1.RepositoryModeProxy {
		return nil, fmt.Errorf("%w: only Proxy repository artifacts support pull-through retry", models.ErrBadRequest)
	}
	return o.requeueFailedProxy(ctx, art)
}

func (o *operator) requeueFailedProxy(ctx context.Context, art *registryv1.Artifact) (*registryv1.Artifact, error) {
	nextAnn := maps.Clone(art.Metadata.Annotations)
	delete(nextAnn, registryv1.AnnotationSyncClaimedBy)
	delete(nextAnn, registryv1.AnnotationSyncLeaseUntil)
	art.Metadata.Annotations = nextAnn
	updated, err := o.db.UpdateObject(ctx, art)
	if err != nil {
		return nil, err
	}
	updated.Status.Phase = registryv1.ArtifactPhasePending
	updated.Status.Stage = registryv1.ArtifactStageQueued
	updated.Status.Progress = 0
	updated.Status.ProcessedBytes = 0
	updated.Status.TotalBytes = 0
	updated.Status.SpeedBytesPerSec = 0
	updated.Status.StartedAt = nil
	updated.Status.Message = "排队等待拉取"
	updated.Status.Conditions = nil
	res, err := o.db.UpdateStatus(ctx, updated)
	if err != nil {
		return nil, err
	}
	if o.logs != nil {
		_ = o.logs.Reset(ctx, artifactLogKey(res.Metadata.Name))
	}
	o.publish(ctx, mq.EventStateChanged, res)
	o.wakeRepository(ctx, res.Spec.Repository)
	return res, nil
}

func (o *operator) pullThroughProxyArtifact(ctx context.Context, name string) (*registryv1.Artifact, error) {
	return o.pullThroughProxy(ctx, name, false)
}

func (o *operator) pullThroughProxy(ctx context.Context, name string, bypassCache bool) (*registryv1.Artifact, error) {
	repoObj, up, pkgName, reqVer, err := o.loadProxyResolveContext(ctx, name)
	if err != nil {
		return nil, err
	}
	rel, err := o.resolveProxyUpstreamReleaseWithCache(ctx, repoObj, up, pkgName, reqVer, !bypassCache)
	if err != nil || rel == nil {
		return nil, db.ErrNotFound
	}
	targetName, targetVer := resolveProxyTargetCoord(repoObj, name, pkgName, reqVer, rel)
	if existing, gErr := o.db.Get(ctx, targetName); gErr == nil && existing != nil {
		if !bypassCache {
			return existing, nil
		}
		o.upsertProxyPackage(ctx, repoObj.Metadata.Name, rel)
		if existing.Status.Phase == registryv1.ArtifactPhaseFailed {
			return o.requeueFailedProxy(ctx, existing)
		}
		return existing, nil
	}
	o.upsertProxyPackage(ctx, repoObj.Metadata.Name, rel)
	art := buildProxyPendingArtifact(targetName, repoObj.Metadata.Name, pkgName, targetVer, rel)
	created, cErr := o.db.Create(ctx, art)
	if cErr != nil {
		if errors.Is(cErr, db.ErrAlreadyExists) || errors.Is(cErr, db.ErrConflict) {
			return o.db.Get(ctx, targetName)
		}
		return nil, cErr
	}
	if o.logs != nil {
		_ = o.logs.Reset(ctx, artifactLogKey(created.Metadata.Name))
	}
	o.publish(ctx, mq.EventCreated, created)
	o.wakeRepository(ctx, repoObj.Metadata.Name)
	return created, nil
}

func resolveProxyTargetCoord(repoObj *registryv1.Repository, name, pkgName, reqVer string, rel *addonmirror.ResolvedRelease) (string, string) {
	if repoObj.Spec.Format != registryv1.FormatServerPackage && addonmirror.IsLatestAlias(reqVer) &&
		strings.TrimSpace(rel.Version) != "" && !addonmirror.IsLatestAlias(rel.Version) {
		targetVer := strings.TrimSpace(rel.Version)
		return registryv1.FormatArtifactName(repoObj.Metadata.Name, pkgName, targetVer), targetVer
	}
	return name, reqVer
}

func (o *operator) loadProxyResolveContext(ctx context.Context, name string) (*registryv1.Repository, *registryv1.Upstream, string, string, error) {
	if o.repoDB == nil || o.upstreamDB == nil {
		return nil, nil, "", "", db.ErrNotFound
	}
	repoName, pkgName, reqVer, err := registryv1.ParseArtifactName(name)
	if err != nil {
		return nil, nil, "", "", db.ErrNotFound
	}
	repoObj, err := o.repoDB.Get(ctx, repoName)
	if err != nil || repoObj.Spec.Mode != registryv1.RepositoryModeProxy || repoObj.Spec.UpstreamName == "" {
		return nil, nil, "", "", db.ErrNotFound
	}
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil, nil, "", "", db.ErrNotFound
	}
	return repoObj, up, pkgName, reqVer, nil
}

func (o *operator) resolveProxyUpstreamReleaseWithCache(
	ctx context.Context,
	repoObj *registryv1.Repository,
	up *registryv1.Upstream,
	pkgName, reqVer string,
	useCache bool,
) (*addonmirror.ResolvedRelease, error) {
	kv := o.cache
	if !useCache {
		kv = nil
	}
	svc, src := o.mirrorForUpstream(kv, repoObj, up)
	rel, err := svc.ResolveRelease(ctx, src, addonmirror.ResolveReleaseRequest{
		Package:       pkgName,
		Version:       reqVer,
		StrictVersion: true,
	})
	if err != nil {
		if errors.Is(err, addonmirror.ErrUnsupportedSource) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	return rel, nil
}

func (o *operator) mirrorForUpstream(kv cache.Interface, repoObj *registryv1.Repository, up *registryv1.Upstream) (*addonmirror.Service, addonmirror.Source) {
	format := ""
	if repoObj != nil {
		format = repoObj.Spec.Format
	}
	return addonmirror.NewForRegistry(kv, config.Get().Registry, addonmirror.RegistryParams{
		Provider:            up.Provider,
		Endpoint:            up.Endpoint,
		Format:              format,
		ProxyURL:            up.ProxyURL,
		APIKey:              up.APIKey,
		SteamAppID:          up.SteamAppID,
		ModrinthLoader:      up.ModrinthLoader,
		ModrinthProjectType: up.ModrinthProjectType,
	})
}

func buildProxyPendingArtifact(name, repoName, pkgName, reqVer string, rel *addonmirror.ResolvedRelease) *registryv1.Artifact {
	deps := make([]registryv1.PackageDependency, 0, len(rel.Dependencies))
	for _, d := range rel.Dependencies {
		deps = append(deps, registryv1.PackageDependency{Package: d.ID, Title: d.Title})
	}
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{
			Name:       name,
			Finalizers: []string{registryv1.FinalizerArtifactBlobRef},
		},
		Spec: registryv1.ArtifactSpec{
			Repository:    repoName,
			Package:       pkgName,
			Version:       reqVer,
			SourceURL:     rel.DownloadURL,
			ArchiveFormat: rel.ArchiveFormat,
			TargetPath:    rel.TargetPath,
			GameVersions:  rel.GameVersions,
			Loaders:       rel.Loaders,
		},
		Status: registryv1.ArtifactStatus{
			Phase:        registryv1.ArtifactPhasePending,
			Stage:        registryv1.ArtifactStageQueued,
			Build:        rel.Build,
			ModIDs:       rel.ModIDs,
			MapFolders:   rel.MapFolders,
			Dependencies: deps,
			PublishedAt:  rel.PublishedAt,
			Changelog:    rel.Changelog,
			UpstreamURL:  rel.UpstreamURL,
		},
	}
	art.DefaultMeta()
	if !slices.Contains(art.Metadata.Finalizers, registryv1.FinalizerArtifactBlobRef) {
		art.Metadata.Finalizers = append(art.Metadata.Finalizers, registryv1.FinalizerArtifactBlobRef)
	}
	return art
}

func (o *operator) upsertProxyPackage(ctx context.Context, repoName string, rel *addonmirror.ResolvedRelease) {
	if o.pkgDB == nil || rel == nil {
		return
	}
	coord := registryv1.FormatPackageName(repoName, rel.Package)
	existing, err := o.pkgDB.Get(ctx, coord)
	if err == nil && existing != nil {
		if rel.Version != "" && !addonmirror.IsLatestAlias(rel.Version) {
			existing.LatestVersion = rel.Version
		}
		applyMirrorItemToPackage(existing, rel.Item)
		if _, uErr := o.pkgDB.Update(ctx, existing); uErr != nil && !errors.Is(uErr, db.ErrConflict) {
			log.S().Warnw("failed to update proxy package metadata", "package", coord, "error", uErr)
		}
		return
	}
	pkg := &registryv1.Package{
		Repository:    repoName,
		Package:       rel.Package,
		Title:         rel.Package,
		LatestVersion: rel.Version,
	}
	applyMirrorItemToPackage(pkg, rel.Item)
	if _, cErr := o.pkgDB.Create(ctx, pkg); cErr != nil && !errors.Is(cErr, db.ErrAlreadyExists) {
		log.S().Warnw("failed to create proxy package metadata", "package", coord, "error", cErr)
	}
}

func applyMirrorItemToPackage(pkg *registryv1.Package, it *addonmirror.Item) {
	if pkg == nil || it == nil {
		return
	}
	if it.Title != "" {
		pkg.Title = it.Title
	}
	pkg.Summary, pkg.Description, pkg.Author = it.Summary, it.Description, it.Author
	pkg.IconURL, pkg.GalleryURLs, pkg.SourceURL = it.IconURL, it.GalleryURLs, it.SourceURL
	pkg.Downloads, pkg.UpdatedAt, pkg.Tags = it.Downloads, it.UpdatedAt, it.Tags
	if it.SizeBytes > 0 {
		pkg.SizeBytes = it.SizeBytes
	}
	if len(it.ModIDs) > 0 {
		pkg.ModIDs = it.ModIDs
	}
	if len(it.MapFolders) > 0 {
		pkg.MapFolders = it.MapFolders
	}
	deps := make([]registryv1.PackageDependency, 0, len(it.Dependencies))
	for _, d := range it.Dependencies {
		deps = append(deps, registryv1.PackageDependency{Package: d.ID, Title: d.Title})
	}
	pkg.Dependencies = deps
}

func (o *operator) HydrateMetadata(ctx context.Context, name string) (*registryv1.Artifact, error) {
	art, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	normURL := addonmirror.NormalizeSteamNewsURL(art.Status.UpstreamURL)
	urlChanged := normURL != art.Status.UpstreamURL
	art.Status.UpstreamURL = normURL
	if o.repoDB == nil || o.upstreamDB == nil {
		return o.persistHydratedStatus(ctx, name, art, urlChanged)
	}
	if !urlChanged && art.Status.PublishedAt != nil && art.Status.Changelog != "" && art.Status.UpstreamURL != "" {
		return art, nil
	}
	changed := urlChanged
	if repoObj, rErr := o.repoDB.Get(ctx, art.Spec.Repository); rErr == nil && repoObj.Spec.Mode == registryv1.RepositoryModeProxy && repoObj.Spec.UpstreamName != "" {
		if up, uErr := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName); uErr == nil && o.populateArtifactUpstreamMeta(ctx, repoObj, up, art) {
			changed = true
		}
	}
	return o.persistHydratedStatus(ctx, name, art, changed)
}

func (o *operator) persistHydratedStatus(ctx context.Context, name string, art *registryv1.Artifact, changed bool) (*registryv1.Artifact, error) {
	if !changed {
		return art, nil
	}
	updated, err := o.db.UpdateStatus(ctx, art)
	if err != nil {
		if cur, gErr := o.db.Get(ctx, name); gErr == nil {
			normalizeArtifactURLs(cur)
			return cur, nil
		}
		return art, nil
	}
	normalizeArtifactURLs(updated)
	return updated, nil
}

func (o *operator) populateArtifactUpstreamMeta(
	ctx context.Context,
	repoObj *registryv1.Repository,
	up *registryv1.Upstream,
	art *registryv1.Artifact,
) bool {
	svc, src := o.mirrorForUpstream(o.cache, repoObj, up)
	rel, err := svc.ResolveRelease(ctx, src, addonmirror.ResolveReleaseRequest{
		Package:       art.Spec.Package,
		Version:       art.Spec.Version,
		StrictVersion: false,
	})
	if err != nil || rel == nil {
		return false
	}
	return applyResolvedFieldsToStatus(&art.Status, rel.UpstreamURL, !rel.VersionMismatch, rel.PublishedAt, rel.Changelog, rel.Build)
}

func applyResolvedFieldsToStatus(st *registryv1.ArtifactStatus, upstreamURL string, allowNotes bool, pubAt *time.Time, changelog, build string) bool {
	changed := false
	upstreamURL = addonmirror.NormalizeSteamNewsURL(upstreamURL)
	if curNorm := addonmirror.NormalizeSteamNewsURL(st.UpstreamURL); curNorm != st.UpstreamURL {
		st.UpstreamURL, changed = curNorm, true
	}
	if st.UpstreamURL == "" && upstreamURL != "" {
		st.UpstreamURL, changed = upstreamURL, true
	}
	if !allowNotes {
		return changed
	}
	if st.PublishedAt == nil && pubAt != nil {
		st.PublishedAt, changed = pubAt, true
	}
	if st.Changelog == "" && changelog != "" {
		st.Changelog, changed = changelog, true
	}
	if st.Build == "" && build != "" {
		st.Build, changed = build, true
	}
	return changed
}
