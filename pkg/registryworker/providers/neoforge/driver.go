package neoforge

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const installerTimeout = 10 * time.Minute

type installerRunner func(ctx context.Context, installerJar, stagingDir string, rep driver.Reporter) error

func init() {
	driver.RegisterSyncDriver(registryv1.UpstreamProviderNeoForge, "*", newBundleDriver("", nil))
}

type bundleDriver struct {
	mojangManifestURL string
	runInstaller      installerRunner
}

func newBundleDriver(mojangManifestURL string, runner installerRunner) *bundleDriver {
	if runner == nil {
		runner = runOfflineJavaInstaller
	}
	return &bundleDriver{
		mojangManifestURL: strings.TrimSpace(mojangManifestURL),
		runInstaller:      runner,
	}
}

func (*bundleDriver) Name() string { return "neoforge-bundle" }

func (d *bundleDriver) Sync(ctx context.Context, req driver.SyncRequest) (*driver.BuiltPayload, error) {
	rel, err := resolveArtifactRelease(ctx, req)
	if err != nil {
		return nil, err
	}
	if reused := tryReuseCachedPayload(ctx, req, rel); reused != nil {
		return reused, nil
	}
	client := req.Mirror.DownloadHTTPClient()
	var preferMirror atomic.Bool
	installerPath, err := downloadInstallerJar(ctx, req, client, rel, &preferMirror)
	if err != nil {
		return nil, err
	}
	meta, err := readInstallerMeta(installerPath, rel)
	if err != nil {
		return nil, err
	}
	stagingDir, err := d.buildOfflineBundle(ctx, req, client, installerPath, meta, &preferMirror)
	if err != nil {
		return nil, err
	}
	return packStagedBundle(ctx, req, rel, stagingDir)
}

func (d *bundleDriver) buildOfflineBundle(
	ctx context.Context,
	req driver.SyncRequest,
	client *http.Client,
	installerPath string,
	meta *installerMeta,
	preferMirror *atomic.Bool,
) (string, error) {
	stagingDir := filepath.Join(req.TmpDir, "neoforge-bundle")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	if err := stageMojangFiles(ctx, req, client, d.mojangManifestURL, meta, stagingDir, preferMirror); err != nil {
		return "", err
	}
	if err := stageMavenLibraries(ctx, req.Reporter, client, installerPath, meta.Libraries, stagingDir, preferMirror); err != nil {
		return "", err
	}
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageArchiving, 72, "正在离线构建 NeoForge 服务端运行时")
	}
	if err := d.runInstaller(ctx, installerPath, stagingDir, req.Reporter); err != nil {
		return "", err
	}
	if err := finalizeStagedServerDir(stagingDir, meta.NFVersion); err != nil {
		return "", err
	}
	return stagingDir, nil
}

func resolveArtifactRelease(ctx context.Context, req driver.SyncRequest) (*addonmirror.ResolvedRelease, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageResolving, 3, "正在解析 NeoForge 服务端版本元数据")
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

func packStagedBundle(ctx context.Context, req driver.SyncRequest, rel *addonmirror.ResolvedRelease, stagingDir string) (*driver.BuiltPayload, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageArchiving, 85, "正在打包离线 NeoForge 服务端整合包")
	}
	archivePath := filepath.Join(req.TmpDir, filepath.Base(rel.FileName))
	sizeBytes, shaHex, err := driver.ArchiveTreeTarGzWithProgress(ctx, stagingDir, archivePath, req.Reporter)
	if err != nil {
		return nil, fmt.Errorf("archive NeoForge bundle: %w", err)
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
		Loaders:       []string{registryv1.ModrinthLoaderNeoForge},
		PublishedAt:   rel.PublishedAt,
		UpstreamURL:   rel.UpstreamURL,
	}, nil
}

func runOfflineJavaInstaller(ctx context.Context, installerJar, stagingDir string, rep driver.Reporter) error {
	runCtx, cancel := context.WithTimeout(ctx, installerTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "java", "-Xmx1G", "-jar", installerJar, "--installServer", ".", "--offline")
	cmd.Dir = stagingDir
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	err := cmd.Run()
	streamInstallerTailLogs(rep, outBuf.Bytes(), err != nil)
	if err != nil {
		return fmt.Errorf("neoforge offline installer failed: %w", err)
	}
	return nil
}

func streamInstallerTailLogs(rep driver.Reporter, raw []byte, failed bool) {
	if rep == nil || len(raw) == 0 {
		return
	}
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			lines = append(lines, line)
		}
	}
	keep := 8
	if failed {
		keep = 25
	}
	if len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	for _, line := range lines {
		rep.LogLine("[neoforge-installer] " + line)
	}
}
