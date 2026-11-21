package fabricmc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func init() {
	driver.RegisterSyncDriver(registryv1.UpstreamProviderFabricMC, "*", newBundleDriver(""))
}

type bundleDriver struct {
	mojangManifestURL string
}

func newBundleDriver(mojangManifestURL string) *bundleDriver {
	return &bundleDriver{mojangManifestURL: strings.TrimSpace(mojangManifestURL)}
}

func (*bundleDriver) Name() string { return "fabricmc-bundle" }

func (d *bundleDriver) Sync(ctx context.Context, req driver.SyncRequest) (*driver.BuiltPayload, error) {
	rel, err := resolveArtifactRelease(ctx, req)
	if err != nil {
		return nil, err
	}
	if reused := tryReuseCachedPayload(ctx, req, rel); reused != nil {
		return reused, nil
	}
	client := req.Mirror.DownloadHTTPClient()
	profile, err := fetchProfile(ctx, client, rel.DownloadURL)
	if err != nil {
		return nil, err
	}
	vanillaRel, err := req.Mirror.ResolveVanillaMC(ctx, d.mojangManifestURL, rel.Version)
	if err != nil {
		return nil, fmt.Errorf("resolve Mojang vanilla server %q: %w", rel.Version, err)
	}
	stagingDir, err := stageBundleFiles(ctx, req, client, profile, vanillaRel)
	if err != nil {
		return nil, err
	}
	return packStagedBundle(ctx, req, rel, stagingDir)
}

func resolveArtifactRelease(ctx context.Context, req driver.SyncRequest) (*addonmirror.ResolvedRelease, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageResolving, 3, "正在解析 Fabric 服务端版本元数据")
	}
	gv := ""
	if len(req.Artifact.Spec.GameVersions) > 0 {
		gv = req.Artifact.Spec.GameVersions[0]
	}
	return req.Mirror.ResolveRelease(ctx, req.Source, addonmirror.ResolveReleaseRequest{
		Package:     req.Artifact.Spec.Package,
		Version:     req.Artifact.Spec.Version,
		GameVersion: gv,
		SourceURL:   req.Artifact.Spec.SourceURL,
	})
}

func tryReuseCachedPayload(ctx context.Context, req driver.SyncRequest, rel *addonmirror.ResolvedRelease) *driver.BuiltPayload {
	if rel.Build == "" {
		return nil
	}
	reused := req.Runtime.ReusableArtifactPayload(ctx, req.Reporter, req.Artifact, rel.Build, rel.PublishedAt, rel.UpstreamURL)
	if reused == nil {
		return nil
	}
	reused.GameVersions = driver.FirstSlice(rel.GameVersions, req.Artifact.Spec.GameVersions)
	reused.Loaders = driver.FirstSlice(rel.Loaders, req.Artifact.Spec.Loaders)
	return reused
}

type fabricProfile struct {
	ID        string `json:"id"`
	MainClass string `json:"mainClass"`
	Arguments struct {
		JVM []string `json:"jvm"`
	} `json:"arguments"`
	Libraries []fabricLibrary `json:"libraries"`
}

type fabricLibrary struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA1   string `json:"sha1"`
	SHA256 string `json:"sha256"`
}

func fetchProfile(ctx context.Context, client *http.Client, profileURL string) (*fabricProfile, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "GamePanel-RegistryWorker/1.0")
	httpReq.Header.Set("Accept", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch Fabric profile %q: %w", profileURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch Fabric profile %q returned HTTP %d", profileURL, resp.StatusCode)
	}
	var prof fabricProfile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&prof); err != nil {
		return nil, fmt.Errorf("decode Fabric profile %q: %w", profileURL, err)
	}
	if strings.TrimSpace(prof.MainClass) == "" {
		return nil, fmt.Errorf("fabric profile %q missing mainClass", profileURL)
	}
	return &prof, nil
}

func packStagedBundle(ctx context.Context, req driver.SyncRequest, rel *addonmirror.ResolvedRelease, stagingDir string) (*driver.BuiltPayload, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageArchiving, 72, "正在打包离线服务端整合包")
	}
	archivePath := filepath.Join(req.TmpDir, filepath.Base(rel.FileName))
	sizeBytes, shaHex, err := driver.ArchiveTreeTarGzWithProgress(ctx, stagingDir, archivePath, req.Reporter)
	if err != nil {
		return nil, fmt.Errorf("archive Fabric bundle: %w", err)
	}
	return &driver.BuiltPayload{
		LocalPath:     archivePath,
		StagingDir:    stagingDir,
		FileName:      filepath.Base(rel.FileName),
		MediaType:     "application/gzip",
		ArchiveFormat: registryv1.ArchiveFormatTarGz,
		TargetPath:    ".",
		SizeBytes:     sizeBytes,
		SHA256Hex:     shaHex,
		Build:         rel.Build,
		GameVersions:  []string{rel.Version},
		Loaders:       []string{registryv1.ModrinthLoaderFabric},
		PublishedAt:   rel.PublishedAt,
		UpstreamURL:   rel.UpstreamURL,
	}, nil
}
