package policy

import (
	"context"
	"errors"
	"fmt"
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
)

// IsDue reports whether an enabled ReplicationPolicy should run now.
func IsDue(pol *registryv1.ReplicationPolicy, now time.Time) bool {
	if pol == nil || !pol.Enabled {
		return false
	}
	if pol.LastRunAt == nil {
		return true
	}
	if pol.IntervalMinutes <= 0 {
		return false
	}
	return !now.Before(pol.LastRunAt.Add(time.Duration(pol.IntervalMinutes) * time.Minute))
}

func (o *operator) Reconcile(ctx context.Context, name string) error {
	pol, err := o.db.Get(ctx, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	now := time.Now().UTC()
	if !IsDue(pol, now) {
		return nil
	}
	msg, runErr := o.executePolicy(ctx, pol)
	outcomeMsg := msg
	if runErr != nil {
		outcomeMsg = "error: " + runErr.Error()
	}
	_, _ = o.db.RecordRunOutcome(ctx, pol.Metadata.Name, pol.Metadata.UID, now, outcomeMsg)
	return runErr
}

func (o *operator) executePolicy(ctx context.Context, pol *registryv1.ReplicationPolicy) (string, error) {
	repoObj, err := o.repoDB.Get(ctx, pol.Repository)
	if err != nil {
		return "", fmt.Errorf("repository %q: %w", pol.Repository, err)
	}
	if repoObj.Spec.UpstreamName == "" {
		return "skipped: repository has no upstreamName", nil
	}
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return "", fmt.Errorf("upstream %q: %w", repoObj.Spec.UpstreamName, err)
	}
	svc, src := mirrorForPolicy(o.cache, repoObj, up)
	switch pol.Mode {
	case registryv1.ReplicationModeTrackReleases:
		return o.runTrackReleases(ctx, svc, src, repoObj, pol)
	case registryv1.ReplicationModeTopPopular:
		return o.runTopPopular(ctx, svc, src, repoObj, pol)
	case registryv1.ReplicationModeExplicitPackages:
		return o.runExplicitPackages(ctx, svc, src, repoObj, pol)
	default:
		return "unsupported mode", nil
	}
}

func mirrorForPolicy(c cache.Interface, repoObj *registryv1.Repository, up *registryv1.Upstream) (*addonmirror.Service, addonmirror.Source) {
	format := ""
	if repoObj != nil {
		format = repoObj.Spec.Format
	}
	return addonmirror.NewForRegistry(c, config.Get().Registry, addonmirror.RegistryParams{
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

func (o *operator) runTrackReleases(ctx context.Context, svc *addonmirror.Service, src addonmirror.Source, repoObj *registryv1.Repository, pol *registryv1.ReplicationPolicy) (string, error) {
	pkgName := repoObj.Metadata.Name
	if len(pol.Packages) > 0 && strings.TrimSpace(pol.Packages[0]) != "" {
		pkgName = strings.TrimSpace(pol.Packages[0])
	}
	serverSrc := src
	serverSrc.Format = registryv1.FormatServerPackage
	rel, err := svc.ResolveRelease(ctx, serverSrc, addonmirror.ResolveReleaseRequest{
		Package:       pkgName,
		Version:       pol.GameVersion,
		AllowBranches: true,
	})
	if err != nil {
		if errors.Is(err, addonmirror.ErrUnsupportedSource) {
			return "", fmt.Errorf("provider %q does not support TrackReleases", src.Provider)
		}
		return "", err
	}
	created, err := o.ensureReleaseArtifact(ctx, repoObj.Metadata.Name, rel)
	if err != nil {
		return "", err
	}
	if created {
		return fmt.Sprintf("queued %s:%s", rel.Package, rel.Version), nil
	}
	return fmt.Sprintf("up-to-date %s:%s", rel.Package, rel.Version), nil
}

func (o *operator) runTopPopular(ctx context.Context, svc *addonmirror.Service, src addonmirror.Source, repoObj *registryv1.Repository, pol *registryv1.ReplicationPolicy) (string, error) {
	limit := int(pol.TopN)
	if limit <= 0 || limit > 30 {
		limit = 10
	}
	res, err := svc.Search(ctx, src, addonmirror.SearchParams{
		Sort:        addonmirror.SortPopular,
		GameVersion: pol.GameVersion,
		Page:        1,
		Limit:       limit,
	})
	if err != nil {
		return "", err
	}
	queued := 0
	visited := make(map[string]bool)
	for _, it := range res.Items {
		n, err := o.syncAddonTree(ctx, svc, src, repoObj.Metadata.Name, it.ID, "", pol.GameVersion, visited, 0)
		if err != nil {
			return "", err
		}
		queued += n
	}
	return fmt.Sprintf("checked %d popular packages (%d new artifacts queued)", len(res.Items), queued), nil
}

func (o *operator) runExplicitPackages(ctx context.Context, svc *addonmirror.Service, src addonmirror.Source, repoObj *registryv1.Repository, pol *registryv1.ReplicationPolicy) (string, error) {
	queued := 0
	visited := make(map[string]bool)
	for _, rawToken := range pol.Packages {
		pkgID, ver := splitPkgVersion(rawToken)
		if pkgID == "" {
			continue
		}
		if svc.SupportsCatalog(src) {
			n, err := o.syncAddonTree(ctx, svc, src, repoObj.Metadata.Name, pkgID, ver, pol.GameVersion, visited, 0)
			if err != nil {
				return "", err
			}
			queued += n
			continue
		}
		serverSrc := src
		serverSrc.Format = registryv1.FormatServerPackage
		rel, err := svc.ResolveRelease(ctx, serverSrc, addonmirror.ResolveReleaseRequest{
			Package:       pkgID,
			Version:       ver,
			GameVersion:   pol.GameVersion,
			AllowBranches: true,
		})
		if err != nil {
			return "", err
		}
		if created, err := o.ensureReleaseArtifact(ctx, repoObj.Metadata.Name, rel); err != nil {
			return "", err
		} else if created {
			queued++
		}
	}
	return fmt.Sprintf("synced %d explicit targets (%d new artifacts queued)", len(pol.Packages), queued), nil
}

func (o *operator) syncAddonTree(ctx context.Context, svc *addonmirror.Service, src addonmirror.Source, repoName, pkgID, reqVer, gameVer string, visited map[string]bool, depth int) (int, error) {
	if depth > 3 || visited[pkgID] {
		return 0, nil
	}
	visited[pkgID] = true
	rel, err := svc.ResolveRelease(ctx, src, addonmirror.ResolveReleaseRequest{
		Package:     pkgID,
		Version:     reqVer,
		GameVersion: gameVer,
	})
	if err != nil {
		if errors.Is(err, addonmirror.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	queued := 0
	if created, err := o.ensureReleaseArtifact(ctx, repoName, rel); err != nil {
		return 0, err
	} else if created {
		queued++
	}
	for _, dep := range rel.Dependencies {
		n, dErr := o.syncAddonTree(ctx, svc, src, repoName, dep.ID, "", gameVer, visited, depth+1)
		if dErr != nil {
			return queued, dErr
		}
		queued += n
	}
	return queued, nil
}

func (o *operator) ensureReleaseArtifact(ctx context.Context, repoName string, rel *addonmirror.ResolvedRelease) (bool, error) {
	o.upsertPackageFromRelease(ctx, repoName, rel)
	artName := registryv1.FormatArtifactName(repoName, rel.Package, rel.Version)
	if existing, err := o.artDB.Get(ctx, artName); err == nil && existing != nil {
		if existing.Status.Phase == registryv1.ArtifactPhaseFailed {
			existing.Status.Phase = registryv1.ArtifactPhasePending
			existing.Status.Message = "re-queued by replication policy"
			_, _ = o.artDB.UpdateStatus(ctx, existing)
			return true, nil
		}
		return false, nil
	}
	art := buildPendingArtifact(artName, repoName, rel)
	created, err := o.artDB.Create(ctx, art)
	if err != nil {
		if errors.Is(err, db.ErrAlreadyExists) || errors.Is(err, db.ErrConflict) {
			return false, nil
		}
		return false, err
	}
	if o.bus != nil {
		ev := mq.NewEvent(mq.EventCreated, registryv1.GroupVersion, registryv1.KindArtifact, "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion)
		ev.Topic = registryv1.TopicArtifacts
		_ = o.bus.Publish(ctx, registryv1.TopicArtifacts, ev)
	}
	return true, nil
}

func buildPendingArtifact(artName, repoName string, rel *addonmirror.ResolvedRelease) *registryv1.Artifact {
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: artName},
		Spec: registryv1.ArtifactSpec{
			Repository:    repoName,
			Package:       rel.Package,
			Version:       rel.Version,
			SourceURL:     rel.DownloadURL,
			ArchiveFormat: rel.ArchiveFormat,
			TargetPath:    rel.TargetPath,
			GameVersions:  rel.GameVersions,
			Loaders:       rel.Loaders,
		},
		Status: registryv1.ArtifactStatus{
			Phase:        registryv1.ArtifactPhasePending,
			Build:        rel.Build,
			ModIDs:       rel.ModIDs,
			MapFolders:   rel.MapFolders,
			Dependencies: toPackageDeps(rel.Dependencies),
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

func (o *operator) upsertPackageFromRelease(ctx context.Context, repoName string, rel *addonmirror.ResolvedRelease) {
	coord := registryv1.FormatPackageName(repoName, rel.Package)
	existing, err := o.pkgDB.Get(ctx, coord)
	if err == nil && existing != nil {
		existing.LatestVersion = rel.Version
		if rel.Item != nil {
			applyItemToPackage(existing, rel.Item)
		}
		if _, uErr := o.pkgDB.Update(ctx, existing); uErr != nil && !errors.Is(uErr, db.ErrConflict) {
			log.S().Warnw("failed to update synced package metadata", "package", coord, "error", uErr)
		}
		return
	}
	pkg := &registryv1.Package{
		Repository:    repoName,
		Package:       rel.Package,
		Title:         rel.Package,
		LatestVersion: rel.Version,
	}
	if rel.Item != nil {
		applyItemToPackage(pkg, rel.Item)
	}
	if _, cErr := o.pkgDB.Create(ctx, pkg); cErr != nil && !errors.Is(cErr, db.ErrAlreadyExists) {
		log.S().Warnw("failed to create synced package metadata", "package", coord, "error", cErr)
	}
}

func applyItemToPackage(pkg *registryv1.Package, it *addonmirror.Item) {
	if it.Title != "" {
		pkg.Title = it.Title
	}
	pkg.Summary = it.Summary
	pkg.Description = it.Description
	pkg.Author = it.Author
	pkg.IconURL = it.IconURL
	pkg.GalleryURLs = it.GalleryURLs
	pkg.SourceURL = it.SourceURL
	pkg.Downloads = it.Downloads
	if it.SizeBytes > 0 {
		pkg.SizeBytes = it.SizeBytes
	}
	pkg.UpdatedAt = it.UpdatedAt
	pkg.Tags = it.Tags
	if len(it.ModIDs) > 0 {
		pkg.ModIDs = it.ModIDs
	}
	if len(it.MapFolders) > 0 {
		pkg.MapFolders = it.MapFolders
	}
	pkg.Dependencies = toPackageDeps(it.Dependencies)
}

func splitPkgVersion(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if idx := strings.IndexByte(trimmed, '@'); idx > 0 {
		return strings.TrimSpace(trimmed[:idx]), strings.TrimSpace(trimmed[idx+1:])
	}
	return trimmed, ""
}

func toPackageDeps(deps []addonmirror.Dependency) []registryv1.PackageDependency {
	out := make([]registryv1.PackageDependency, 0, len(deps))
	for _, d := range deps {
		out = append(out, registryv1.PackageDependency{Package: d.ID, Title: d.Title})
	}
	return out
}
