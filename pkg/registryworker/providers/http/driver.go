package http

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func init() {
	drv := releaseDriver{}
	for _, p := range []string{
		registryv1.UpstreamProviderPaperMC,
		registryv1.UpstreamProviderPurpur,
		registryv1.UpstreamProviderVanilla,
		registryv1.UpstreamProviderHTTP,
		registryv1.UpstreamProviderModrinth,
		registryv1.UpstreamProviderThunderstore,
		registryv1.UpstreamProviderUMod,
		registryv1.UpstreamProviderFicsit,
		registryv1.UpstreamProviderCS2Plugins,
		registryv1.UpstreamProviderSevenDaysMods,
		registryv1.UpstreamProviderModIO,
		addonmirror.ProviderTerraria,
		addonmirror.ProviderTModLoader,
	} {
		driver.RegisterSyncDriver(p, "*", drv)
	}
	driver.RegisterSchemeSyncDriver("http", drv)
	driver.RegisterSchemeSyncDriver("https", drv)
}

type releaseDriver struct{}

func (releaseDriver) Name() string { return "http-release" }

func (releaseDriver) Sync(ctx context.Context, req driver.SyncRequest) (*driver.BuiltPayload, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageResolving, 3, "正在解析上游版本元数据")
	}
	rel, err := resolveRelease(ctx, req.Mirror, req.Source, req.Artifact, req.Upstream)
	if err != nil {
		return nil, err
	}
	if rel.Build != "" {
		if reused := req.Runtime.ReusableArtifactPayload(ctx, req.Reporter, req.Artifact, rel.Build, rel.PublishedAt, rel.UpstreamURL); reused != nil {
			reused.GameVersions = driver.FirstSlice(rel.GameVersions, req.Artifact.Spec.GameVersions)
			reused.Loaders = driver.FirstSlice(rel.Loaders, req.Artifact.Spec.Loaders)
			reused.ModIDs = driver.FirstSlice(rel.ModIDs, reused.ModIDs)
			reused.MapFolders = driver.FirstSlice(rel.MapFolders, reused.MapFolders)
			reused.Dependencies = driver.MergeDeps(rel.Dependencies, reused.Dependencies)
			reused.Changelog = driver.FirstStr(rel.Changelog, reused.Changelog)
			return reused, nil
		}
	}
	return req.Runtime.DownloadHTTPPayload(ctx, req.Reporter, req.Mirror, req.Artifact, rel, req.TmpDir)
}

func resolveRelease(ctx context.Context, svc *addonmirror.Service, src addonmirror.Source, art *registryv1.Artifact, up *registryv1.Upstream) (*addonmirror.ResolvedRelease, error) {
	if strings.HasPrefix(art.Spec.SourceURL, "http://") || strings.HasPrefix(art.Spec.SourceURL, "https://") {
		fname := filepath.Base(strings.Split(art.Spec.SourceURL, "?")[0])
		if fname == "" || fname == "." {
			fname = "package.bin"
		}
		return &addonmirror.ResolvedRelease{
			Package:       art.Spec.Package,
			Version:       art.Spec.Version,
			Build:         art.Status.Build,
			DownloadURL:   art.Spec.SourceURL,
			FileName:      fname,
			ArchiveFormat: driver.FirstStr(art.Spec.ArchiveFormat, registryv1.ArchiveFormatRaw),
			TargetPath:    art.Spec.TargetPath,
			GameVersions:  art.Spec.GameVersions,
			Loaders:       art.Spec.Loaders,
		}, nil
	}
	if up == nil {
		return nil, fmt.Errorf("artifact %q has no sourceURL and repository has no upstream", art.Metadata.Name)
	}
	gv := ""
	if len(art.Spec.GameVersions) > 0 {
		gv = art.Spec.GameVersions[0]
	}
	return svc.ResolveRelease(ctx, src, addonmirror.ResolveReleaseRequest{
		Package:     art.Spec.Package,
		Version:     art.Spec.Version,
		GameVersion: gv,
		SourceURL:   art.Spec.SourceURL,
	})
}
