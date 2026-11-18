package registryworker

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

// InspectCachedWorkshopPayload downloads and inspects an already-cached S3 workshop blob
// when the upstream revision has not changed, avoiding a full SteamCMD re-download.
func (w *Worker) InspectCachedWorkshopPayload(
	ctx context.Context,
	rep driver.Reporter,
	art *registryv1.Artifact,
	steamAppID int,
	rel *addonmirror.ResolvedRelease,
	tmpDir string,
) *driver.BuiltPayload {
	if w.cli == nil || tmpDir == "" {
		return nil
	}
	canonDigest, hexDigest, desc, ok := canReuseCachedWorkshopBlob(art, rel)
	if !ok {
		return nil
	}
	insp, ok := driver.LookupWorkshopInspector(steamAppID)
	if !ok {
		return nil
	}
	blob, err := w.cli.RegistryV1().Blobs().GetMetadata(ctx, canonDigest)
	if err != nil || blob == nil || blob.SizeBytes <= 0 {
		return nil
	}
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageResolving, 35, "从对象存储缓存读取模组包并提取元数据")
		rep.Logf("命中对象存储缓存 %s (Build %s)，直接提取模组元数据...",
			canonDigest, driver.FirstStr(strings.TrimSpace(rel.Build), strings.TrimSpace(art.Status.Build), "unknown"))
	}
	archiveFmt := driver.FirstStr(desc.ArchiveFormat, art.Spec.ArchiveFormat, registryv1.ArchiveFormatTarGz)
	fileName := driver.FirstStr(desc.FileName, "package.tar.gz")
	res, err := inspectCachedBlobFile(ctx, w.cli, canonDigest, tmpDir, fileName, insp, archiveFmt)
	if err != nil {
		if rep != nil {
			rep.Logf("读取缓存对象提取元数据失败，回退至上游下载: %v", err)
		}
		return nil
	}
	p := buildCachedWorkshopPayload(art, rel, desc, blob.SizeBytes, blob.MediaType, hexDigest, res)
	if rep != nil {
		rep.Logf("缓存元数据提取完成: modIds=%d, mapFolders=%d, configSchema=%d",
			len(p.ModIDs), len(p.MapFolders), len(p.ConfigSchema))
	}
	return p
}

func canReuseCachedWorkshopBlob(art *registryv1.Artifact, rel *addonmirror.ResolvedRelease) (string, string, registryv1.BlobDescriptor, bool) {
	if art == nil || rel == nil || art.Status.Descriptor == nil {
		return "", "", registryv1.BlobDescriptor{}, false
	}
	canonDigest, hexDigest, err := registryv1.NormalizeDigest(art.Status.Descriptor.Digest)
	if err != nil {
		return "", "", registryv1.BlobDescriptor{}, false
	}
	if src := strings.TrimSpace(art.Spec.SourceURL); src != "" && art.Status.UpstreamURL != "" && src != art.Status.UpstreamURL {
		return "", "", registryv1.BlobDescriptor{}, false
	}
	relBuild := strings.TrimSpace(rel.Build)
	curBuild := strings.TrimSpace(art.Status.Build)
	if relBuild != "" && curBuild != "" && relBuild != curBuild {
		return "", "", registryv1.BlobDescriptor{}, false
	}
	return canonDigest, hexDigest, *art.Status.Descriptor, true
}

func inspectCachedBlobFile(
	ctx context.Context,
	cli client.Interface,
	canonDigest, tmpDir, fileName string,
	insp driver.WorkshopInspector,
	archiveFmt string,
) (driver.WorkshopInspection, error) {
	rc, err := cli.RegistryV1().Blobs().Download(ctx, canonDigest)
	if err != nil {
		return driver.WorkshopInspection{}, err
	}
	defer func() { _ = rc.Close() }()

	cachedPath := filepath.Join(tmpDir, "cached-"+filepath.Base(fileName))
	f, err := os.Create(cachedPath)
	if err != nil {
		return driver.WorkshopInspection{}, err
	}
	defer func() { _ = os.Remove(cachedPath) }()

	_, copyErr := io.Copy(f, rc)
	closeErr := f.Close()
	if copyErr != nil {
		return driver.WorkshopInspection{}, copyErr
	}
	if closeErr != nil {
		return driver.WorkshopInspection{}, closeErr
	}
	return insp.InspectArchive(archiveFmt, cachedPath), nil
}

func buildCachedWorkshopPayload(
	art *registryv1.Artifact,
	rel *addonmirror.ResolvedRelease,
	desc registryv1.BlobDescriptor,
	sizeBytes int64,
	blobMediaType, hexDigest string,
	res driver.WorkshopInspection,
) *driver.BuiltPayload {
	schema := art.Status.ConfigSchema
	if len(res.ConfigSchema) > 0 {
		schema = res.ConfigSchema
	}
	return &driver.BuiltPayload{
		FileName:      driver.FirstStr(desc.FileName, "package.tar.gz"),
		MediaType:     driver.FirstStr(desc.MediaType, blobMediaType, "application/gzip"),
		ArchiveFormat: driver.FirstStr(desc.ArchiveFormat, art.Spec.ArchiveFormat, registryv1.ArchiveFormatTarGz),
		TargetPath:    driver.FirstStr(art.Spec.TargetPath, "."),
		SizeBytes:     sizeBytes,
		SHA256Hex:     hexDigest,
		Build:         driver.FirstStr(strings.TrimSpace(rel.Build), strings.TrimSpace(art.Status.Build)),
		ModIDs:        driver.FirstSlice(res.ModIDs, driver.FirstSlice(rel.ModIDs, art.Status.ModIDs)),
		MapFolders:    driver.FirstSlice(res.MapFolders, driver.FirstSlice(rel.MapFolders, art.Status.MapFolders)),
		Dependencies:  driver.MergeDeps(rel.Dependencies, art.Status.Dependencies),
		ConfigSchema:  schema,
		PublishedAt:   driver.FirstTimePtr(rel.PublishedAt, art.Status.PublishedAt),
		Changelog:     driver.FirstStr(rel.Changelog, art.Status.Changelog),
		UpstreamURL:   driver.FirstStr(rel.UpstreamURL, art.Status.UpstreamURL),
	}
}
