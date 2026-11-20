package registryworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func (w *Worker) processClaimedArtifact(ctx context.Context, rep *syncReporter, art *registryv1.Artifact, tmpDir string) error {
	repoObj, err := w.cli.RegistryV1().Repositories().Get(ctx, art.Spec.Repository)
	if err != nil {
		return fmt.Errorf("get repository %q: %w", art.Spec.Repository, err)
	}
	var up *registryv1.Upstream
	if repoObj.Spec.UpstreamName != "" {
		up, err = w.cli.RegistryV1().Upstreams().Get(ctx, repoObj.Spec.UpstreamName)
		if err != nil {
			return fmt.Errorf("get upstream %q: %w", repoObj.Spec.UpstreamName, err)
		}
	}
	drv, err := LookupSyncDriver(art, repoObj, up)
	if err != nil {
		return err
	}
	svc, src := w.mirrorForArtifact(repoObj, up)
	var repIfc driver.Reporter
	if rep != nil {
		repIfc = rep
	}
	payload, err := drv.Sync(ctx, SyncRequest{
		Runtime:  w,
		Reporter: repIfc,
		Mirror:   svc,
		Source:   src,
		Artifact: art,
		Repo:     repoObj,
		Upstream: up,
		TmpDir:   tmpDir,
	})
	if err != nil {
		return err
	}
	digest, err := w.ensureBlobUploaded(ctx, rep, payload)
	if err != nil {
		return err
	}
	cleanupPayloadStagingDir(payload.StagingDir)
	w.queueMissingDependencies(ctx, repoObj, up, svc, src, payload.Dependencies)
	return w.markReady(ctx, rep, art, digest, payload)
}

func cleanupPayloadStagingDir(stagingDir string) {
	driver.CleanupStagingDir(stagingDir)
}

func (w *Worker) mirrorForArtifact(repoObj *registryv1.Repository, up *registryv1.Upstream) (*addonmirror.Service, addonmirror.Source) {
	var p addonmirror.RegistryParams
	if repoObj != nil {
		p.Format = repoObj.Spec.Format
	}
	if up != nil {
		p.Provider = up.Provider
		p.Endpoint = up.Endpoint
		p.SteamAppID = up.SteamAppID
		p.ModrinthLoader = up.ModrinthLoader
		p.ModrinthProjectType = up.ModrinthProjectType
		p.ProxyURL = up.ProxyURL
		p.APIKey = up.APIKey
	}
	return addonmirror.NewForRegistry(nil, w.cfg.Registry, p)
}

// ReusableArtifactPayload checks if an artifact's upstream build ID matches its already-stored ready blob.
func (w *Worker) ReusableArtifactPayload(ctx context.Context, rep driver.Reporter, art *registryv1.Artifact, buildID string, pubAt *time.Time, upstreamURL string) *driver.BuiltPayload {
	if w.cli == nil || art == nil || strings.TrimSpace(buildID) == "" {
		return nil
	}
	if strings.TrimSpace(art.Status.Build) != strings.TrimSpace(buildID) || art.Status.Descriptor == nil || art.Status.Descriptor.Digest == "" {
		return nil
	}
	canonDigest, hexDigest, err := registryv1.NormalizeDigest(art.Status.Descriptor.Digest)
	if err != nil {
		return nil
	}
	blob, err := w.cli.RegistryV1().Blobs().GetMetadata(ctx, canonDigest)
	if err != nil || blob == nil || blob.SizeBytes <= 0 {
		return nil
	}
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageUploading, 96, "上游版本未变更，复用已有缓存")
		rep.Logf("上游版本未变更 (Build %s)，复用已有对象存储缓存 %s", strings.TrimSpace(buildID), canonDigest)
	}
	desc := art.Status.Descriptor
	return &driver.BuiltPayload{
		FileName:      driver.FirstStr(desc.FileName, "package.tar.gz"),
		MediaType:     driver.FirstStr(desc.MediaType, blob.MediaType, "application/octet-stream"),
		ArchiveFormat: driver.FirstStr(desc.ArchiveFormat, art.Spec.ArchiveFormat, registryv1.ArchiveFormatRaw),
		TargetPath:    driver.FirstStr(art.Spec.TargetPath, "."),
		SizeBytes:     blob.SizeBytes,
		SHA256Hex:     hexDigest,
		Build:         strings.TrimSpace(buildID),
		ModIDs:        art.Status.ModIDs,
		MapFolders:    art.Status.MapFolders,
		Dependencies:  art.Status.Dependencies,
		ConfigSchema:  art.Status.ConfigSchema,
		PublishedAt:   driver.FirstTimePtr(pubAt, art.Status.PublishedAt),
		Changelog:     art.Status.Changelog,
		UpstreamURL:   driver.FirstStr(addonmirror.NormalizeSteamNewsURL(upstreamURL), addonmirror.NormalizeSteamNewsURL(art.Status.UpstreamURL)),
	}
}

// DownloadHTTPPayload streams a resolved HTTP download to tmpDir and reports byte progress.
func (w *Worker) DownloadHTTPPayload(ctx context.Context, rep driver.Reporter, svc *addonmirror.Service, art *registryv1.Artifact, rel *addonmirror.ResolvedRelease, tmpDir string) (*driver.BuiltPayload, error) {
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageDownloading, 5, "正在连接上游下载")
		rep.Logf("HTTP GET %s", rel.DownloadURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GamePanel-RegistryWorker/1.0")
	resp, err := svc.DownloadHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %q: %w", rel.DownloadURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %q returned HTTP %d", rel.DownloadURL, resp.StatusCode)
	}
	fname := driver.FirstStr(filepath.Base(rel.FileName), "artifact.bin")
	if rep != nil {
		rep.Logf("HTTP %d，文件 %s，总大小 %d 字节", resp.StatusCode, fname, resp.ContentLength)
	}
	dstPath := filepath.Join(tmpDir, fname)
	n, shaHex, err := streamHTTPBodyToFile(dstPath, rel.DownloadURL, resp, newHTTPProgressWriter(rep, resp.ContentLength))
	if err != nil {
		return nil, err
	}
	if rep != nil {
		rep.Logf("下载完成: 共 %d 字节", n)
	}
	return &driver.BuiltPayload{
		LocalPath:     dstPath,
		FileName:      fname,
		MediaType:     driver.FirstStr(resp.Header.Get("Content-Type"), "application/octet-stream"),
		ArchiveFormat: driver.FirstStr(rel.ArchiveFormat, art.Spec.ArchiveFormat, registryv1.ArchiveFormatRaw),
		TargetPath:    driver.FirstStr(rel.TargetPath, art.Spec.TargetPath),
		SizeBytes:     n,
		SHA256Hex:     shaHex,
		Build:         rel.Build,
		GameVersions:  rel.GameVersions,
		Loaders:       rel.Loaders,
		ModIDs:        driver.FirstSlice(rel.ModIDs, art.Status.ModIDs),
		MapFolders:    driver.FirstSlice(rel.MapFolders, art.Status.MapFolders),
		Dependencies:  driver.MergeDeps(rel.Dependencies, art.Status.Dependencies),
		ConfigSchema:  art.Status.ConfigSchema,
		PublishedAt:   driver.FirstTimePtr(rel.PublishedAt, art.Status.PublishedAt),
		Changelog:     driver.FirstStr(rel.Changelog, art.Status.Changelog),
		UpstreamURL:   driver.FirstStr(addonmirror.NormalizeSteamNewsURL(rel.UpstreamURL), addonmirror.NormalizeSteamNewsURL(art.Status.UpstreamURL)),
	}, nil
}

func streamHTTPBodyToFile(dstPath, rawURL string, resp *http.Response, pw io.Writer) (int64, string, error) {
	f, err := os.Create(dstPath)
	if err != nil {
		return 0, "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hasher, pw), resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("stream %q: %w", rawURL, err)
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return 0, "", fmt.Errorf("stream %q truncated: got %d of %d bytes", rawURL, n, resp.ContentLength)
	}
	closed = true
	if err := f.Close(); err != nil {
		return 0, "", fmt.Errorf("close %q: %w", dstPath, err)
	}
	return n, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (w *Worker) markReady(ctx context.Context, rep *syncReporter, art *registryv1.Artifact, digest string, p *driver.BuiltPayload) error {
	cur, err := w.cli.RegistryV1().Artifacts().Get(ctx, art.Metadata.Name)
	if err == nil && cur.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] != w.workerID {
		return fmt.Errorf("sync lease for %s lost", art.Metadata.Name)
	}
	if err != nil {
		cur = art
	}
	now := time.Now().UTC()
	cur.Status.ObservedGeneration = cur.Metadata.Generation
	cur.Status.Phase = registryv1.ArtifactPhaseReady
	cur.Status.Stage = registryv1.ArtifactStageCompleted
	cur.Status.Progress = 100
	cur.Status.ProcessedBytes = p.SizeBytes
	cur.Status.TotalBytes = p.SizeBytes
	cur.Status.SpeedBytesPerSec = 0
	if rep != nil {
		cur.Status.StartedAt = &rep.startedAt
	}
	cur.Status.Build = driver.FirstStr(p.Build, cur.Status.Build)
	cur.Status.Descriptor = &registryv1.BlobDescriptor{
		Digest:        digest,
		SizeBytes:     p.SizeBytes,
		MediaType:     p.MediaType,
		ArchiveFormat: p.ArchiveFormat,
		FileName:      p.FileName,
	}
	if len(p.ModIDs) > 0 {
		cur.Status.ModIDs = p.ModIDs
	}
	if len(p.MapFolders) > 0 {
		cur.Status.MapFolders = p.MapFolders
	}
	if len(p.Dependencies) > 0 {
		cur.Status.Dependencies = p.Dependencies
	}
	if len(p.ConfigSchema) > 0 {
		cur.Status.ConfigSchema = p.ConfigSchema
	}
	cur.Status.PublishedAt = driver.FirstTimePtr(p.PublishedAt, cur.Status.PublishedAt)
	cur.Status.Changelog = driver.FirstStr(p.Changelog, cur.Status.Changelog)
	cur.Status.UpstreamURL = driver.FirstStr(addonmirror.NormalizeSteamNewsURL(p.UpstreamURL), addonmirror.NormalizeSteamNewsURL(cur.Status.UpstreamURL))
	cur.Status.SyncedAt = &now
	cur.Status.Message = ""
	cur.Status.Conditions = []metav1.Condition{{
		Type: metav1.ConditionTypeReady, Status: metav1.ConditionTrue,
		Reason: registryv1.ConditionReasonReady, Message: "artifact synchronized to S3 blob store", LastTransitionTime: now,
	}}
	_, err = w.cli.RegistryV1().Artifacts().UpdateStatus(ctx, cur)
	if rep != nil {
		rep.Finish(fmt.Sprintf("同步完成，Blob 摘要 %s (%d 字节)", digest, p.SizeBytes))
	}
	return err
}

func (w *Worker) queueMissingDependencies(ctx context.Context, repoObj *registryv1.Repository, up *registryv1.Upstream, svc *addonmirror.Service, src addonmirror.Source, deps []registryv1.PackageDependency) {
	if up == nil || len(deps) == 0 || !svc.SupportsCatalog(src) {
		return
	}
	for _, dep := range deps {
		pkgName := strings.TrimSpace(dep.Package)
		if pkgName == "" {
			continue
		}
		rel, err := svc.ResolveRelease(ctx, src, addonmirror.ResolveReleaseRequest{Package: pkgName})
		if err != nil || rel == nil || rel.Version == "" {
			continue
		}
		coord := registryv1.FormatArtifactName(repoObj.Metadata.Name, dep.Package, rel.Version)
		if _, err := w.cli.RegistryV1().Artifacts().Get(ctx, coord); err == nil {
			continue
		}
		_, _ = w.cli.RegistryV1().Artifacts().Create(ctx, &registryv1.Artifact{
			Metadata: metav1.ObjectMeta{Name: coord},
			Spec:     registryv1.ArtifactSpec{Repository: repoObj.Metadata.Name, Package: dep.Package, Version: rel.Version},
		})
	}
}

type byteProgressWriter struct {
	total     int64
	done      int64
	from, to  int32
	lastPct   int32
	lastLogAt int32
	speed     *driver.ByteSpeedTracker
	onStep    func(pct int32, done, total, speedBps int64, milestone bool)
}

func (p *byteProgressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	var spd int64
	if p.speed != nil {
		spd = p.speed.Update(p.done)
	}
	if p.total > 0 && p.onStep != nil {
		pct := driver.ScalePercent(p.done, p.total, p.from, p.to)
		milestone := pct-p.lastLogAt >= 15
		if milestone {
			p.lastLogAt = pct
		}
		if pct > p.lastPct || milestone {
			p.lastPct = pct
			p.onStep(pct, p.done, p.total, spd, milestone)
		}
	}
	return len(b), nil
}

func newHTTPProgressWriter(rep driver.Reporter, total int64) *byteProgressWriter {
	return &byteProgressWriter{
		total: total,
		from:  5,
		to:    70,
		speed: &driver.ByteSpeedTracker{},
		onStep: func(pct int32, done, totalBytes, spd int64, milestone bool) {
			if rep == nil {
				return
			}
			rep.Transfer(registryv1.ArtifactStageDownloading, pct, done, totalBytes, spd, fmt.Sprintf("正在下载 %d%%", pct))
			if milestone {
				rep.Logf("下载进度 %d%% (%d / %d 字节)", pct, done, totalBytes)
			}
		},
	}
}
