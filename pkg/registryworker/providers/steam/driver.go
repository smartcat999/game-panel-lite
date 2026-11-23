package steam

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const maxSteamCMDAttempts = 6

var (
	steamBuildIDRe     = regexp.MustCompile(`"buildid"\s+"(\d+)"`)
	steamLastUpdatedRe = regexp.MustCompile(`"LastUpdated"\s+"(\d+)"`)
)

func init() {
	srvDrv := serverSyncDriver{}
	driver.RegisterSyncDriver(registryv1.UpstreamProviderSteam, "*", srvDrv)
	driver.RegisterSchemeSyncDriver(strings.TrimSuffix(registryv1.PackageSourceSteamCMDScheme, "://"), srvDrv)
	driver.RegisterSyncDriver(registryv1.UpstreamProviderSteam, registryv1.FormatWorkshopMod, workshopSyncDriver{})
}

type serverSyncDriver struct{}

func (serverSyncDriver) Name() string { return "steam-server" }

func (serverSyncDriver) Sync(ctx context.Context, req driver.SyncRequest) (*driver.BuiltPayload, error) {
	var appID string
	if strings.HasPrefix(req.Artifact.Spec.SourceURL, registryv1.PackageSourceSteamCMDScheme) {
		appID = strings.TrimPrefix(req.Artifact.Spec.SourceURL, registryv1.PackageSourceSteamCMDScheme)
	} else if req.Upstream != nil {
		appID = strconv.Itoa(req.Upstream.SteamAppID)
	}
	platforms := resolveSteamPlatforms(req.Artifact, req.Upstream)
	return syncServer(ctx, req.Runtime, req.Reporter, req.Artifact, appID, platforms, req.TmpDir)
}

type workshopSyncDriver struct{}

func (workshopSyncDriver) Name() string { return "steam-workshop" }

func (workshopSyncDriver) Sync(ctx context.Context, req driver.SyncRequest) (*driver.BuiltPayload, error) {
	return syncWorkshopMod(ctx, req.Runtime, req.Reporter, req.Mirror, req.Source, req.Artifact, req.Upstream, req.TmpDir)
}

func runWithRetry(ctx context.Context, rt driver.Runtime, rep driver.Reporter, basePct int32, label, installDir string, args []string) error {
	var lastErr error
	for attempt := 1; attempt <= maxSteamCMDAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := label
		if attempt > 1 {
			msg = fmt.Sprintf("%s（断点续传 %d/%d）", label, attempt, maxSteamCMDAttempts)
		}
		if rep != nil {
			rep.LogLine(msg)
		}
		if lastErr = ExecAttempt(ctx, rt, rep, basePct, label, installDir, args); lastErr == nil {
			return nil
		}
		if rep != nil {
			rep.Logf("SteamCMD 尝试 %d/%d 失败: %v", attempt, maxSteamCMDAttempts, lastErr)
		}
	}
	return lastErr
}

// ExecAttempt runs one SteamCMD pull invocation and forwards queue/download telemetry hooks to rep.
func ExecAttempt(ctx context.Context, rt driver.Runtime, rep driver.Reporter, basePct int32, label, installDir string, args []string) error {
	speed := &driver.ByteSpeedTracker{}
	hooks := driver.SteamHooks{
		OnWait: func() {
			if rep != nil {
				rep.Stage(registryv1.ArtifactStageQueued, 0, "排队等待下载通道")
				rep.LogLine("排队中：等待当前 SteamCMD 下载任务完成...")
			}
		},
		OnAcquired: func() {
			if rep != nil {
				rep.Stage(registryv1.ArtifactStageDownloading, basePct, label)
				rep.LogLine("已分配下载通道，开始执行 SteamCMD 拉取...")
			}
		},
		OnLine: func(line string) {
			if rep == nil {
				return
			}
			rep.LogLine(line)
			if rawPct, done, total, ok := driver.ParseSteamProgress(line); ok {
				pct := min(basePct+int32(rawPct*float64(70-basePct)/100.0), 70)
				spd := speed.Update(done)
				rep.Transfer(registryv1.ArtifactStageDownloading, pct, done, total, spd, fmt.Sprintf("%s %.1f%%", label, rawPct))
			}
		},
	}
	return rt.SteamExec().Run(ctx, installDir, args, hooks)
}

func resolveSteamPlatforms(art *registryv1.Artifact, up *registryv1.Upstream) string {
	if art != nil {
		if p := strings.TrimSpace(art.Metadata.Annotations[registryv1.AnnotationSteamPlatforms]); p != "" {
			return p
		}
	}
	if up != nil {
		return strings.TrimSpace(up.Metadata.Annotations[registryv1.AnnotationSteamPlatforms])
	}
	return ""
}

func parseSteamPlatforms(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildServerUpdateArgs(appID, branch, platforms string) []string {
	args := []string{"+app_info_update", "1"}
	plats := parseSteamPlatforms(platforms)
	if len(plats) >= 2 {
		for _, p := range plats[:len(plats)-1] {
			args = append(args, "+@sSteamCmdForcePlatformType", p, "+app_update", appID)
		}
	}
	if len(plats) >= 1 {
		args = append(args, "+@sSteamCmdForcePlatformType", plats[len(plats)-1])
	}
	args = append(args, "+app_update", appID)
	if branch != "" && branch != "latest" && branch != "stable" && branch != "public" {
		args = append(args, "-beta", branch)
	}
	return append(args, "validate")
}

func syncServer(ctx context.Context, rt driver.Runtime, rep driver.Reporter, art *registryv1.Artifact, appID, platforms, tmpDir string) (*driver.BuiltPayload, error) {
	if strings.TrimSpace(appID) == "" || appID == "0" {
		return nil, fmt.Errorf("steam appID is required for artifact %q", art.Metadata.Name)
	}
	branch := strings.TrimSpace(art.Spec.Version)
	staging := filepath.Join(rt.WorkDir(), "steam-servers", appID, filepath.Base(driver.FirstStr(branch, "stable")))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, err
	}
	args := buildServerUpdateArgs(appID, branch, platforms)
	if err := runWithRetry(ctx, rt, rep, 5, "下载游戏包 "+appID, staging, args); err != nil {
		if errors.Is(context.Cause(ctx), driver.ErrArtifactAborted) {
			driver.CleanupStagingDir(staging)
		}
		return nil, fmt.Errorf("steamcmd app_update %s: %w", appID, err)
	}
	buildID := readManifestBuildID(staging, appID)
	pubAt := readManifestPublishedAt(staging, appID)
	upstreamURL := addonmirror.SteamNewsURLFromString(appID)
	if reused := rt.ReusableArtifactPayload(ctx, rep, art, buildID, pubAt, upstreamURL); reused != nil {
		reused.StagingDir = staging
		return reused, nil
	}
	if rep != nil {
		rep.LogLine("正在将下载目录打包并计算 SHA-256 校验...")
		rep.Stage(registryv1.ArtifactStageArchiving, 72, "正在打包校验文件")
	}
	dst := filepath.Join(tmpDir, "package.tar.gz")
	size, shaHex, err := driver.ArchiveTreeTarGzWithProgress(ctx, staging, dst, rep)
	if err != nil {
		if errors.Is(context.Cause(ctx), driver.ErrArtifactAborted) {
			driver.CleanupStagingDir(staging)
		}
		return nil, fmt.Errorf("archive steam game %s: %w", appID, err)
	}
	if rep != nil {
		rep.Logf("打包完成: 大小 %d 字节 (Build: %s)", size, driver.FirstStr(buildID, "-"))
	}
	return &driver.BuiltPayload{
		LocalPath: dst, StagingDir: staging, FileName: "package.tar.gz", MediaType: "application/gzip",
		ArchiveFormat: registryv1.ArchiveFormatTarGz, TargetPath: ".",
		SizeBytes: size, SHA256Hex: shaHex, Build: buildID,
		PublishedAt: pubAt, UpstreamURL: upstreamURL,
	}, nil
}

func syncWorkshopMod(ctx context.Context, rt driver.Runtime, rep driver.Reporter, svc *addonmirror.Service, src addonmirror.Source, art *registryv1.Artifact, up *registryv1.Upstream, tmpDir string) (*driver.BuiltPayload, error) {
	if up == nil {
		return nil, fmt.Errorf("steam upstream is required for workshop artifact %q", art.Metadata.Name)
	}
	appID := strconv.Itoa(up.SteamAppID)
	workshopID := strings.TrimSpace(art.Spec.Package)
	if rep != nil {
		rep.Logf("正在解析创意工坊模组 %s (AppID %s)...", workshopID, appID)
	}
	item, _ := svc.ResolveSteamWorkshopItemWithSource(ctx, src, workshopID)
	rel := &addonmirror.ResolvedRelease{Package: art.Spec.Package, Version: art.Spec.Version}
	applyItemToRelease(rel, item)
	if cached := rt.InspectCachedWorkshopPayload(ctx, rep, art, up.SteamAppID, rel, tmpDir); cached != nil {
		return cached, nil
	}
	if up.SteamAppID == addonmirror.SteamAppCS2 {
		return buildCS2WorkshopManifestPayload(rep, art, workshopID, item, rel, tmpDir)
	}
	directURL := strings.TrimSpace(art.Spec.SourceURL)
	if directURL == "" && item != nil {
		directURL = item.FileURL
	}
	if strings.HasPrefix(directURL, "http://") || strings.HasPrefix(directURL, "https://") {
		return downloadDirectWorkshopUGC(ctx, rt, rep, svc, art, up.SteamAppID, directURL, item, tmpDir)
	}
	return downloadSteamCMDWorkshopItem(ctx, rt, rep, art, appID, workshopID, up.SteamAppID, item, tmpDir)
}

func downloadDirectWorkshopUGC(ctx context.Context, rt driver.Runtime, rep driver.Reporter, svc *addonmirror.Service, art *registryv1.Artifact, steamAppID int, directURL string, item *addonmirror.Item, tmpDir string) (*driver.BuiltPayload, error) {
	fileName := ""
	if item != nil {
		fileName = strings.ReplaceAll(strings.TrimSpace(item.FileName), "\\", "/")
	}
	fname := filepath.Base(defaultWorkshopFileName(steamAppID, art.Spec.Package, fileName))
	rel := &addonmirror.ResolvedRelease{
		Package: art.Spec.Package, Version: art.Spec.Version, DownloadURL: directURL,
		FileName: fname, ArchiveFormat: inferSteamArchiveFormat(fname, directURL),
	}
	applyItemToRelease(rel, item)
	p, err := rt.DownloadHTTPPayload(ctx, rep, svc, art, rel, tmpDir)
	if err != nil {
		return nil, err
	}
	if insp, ok := driver.LookupWorkshopInspector(steamAppID); ok {
		res := insp.InspectArchive(p.ArchiveFormat, p.LocalPath)
		p.ModIDs = driver.FirstSlice(res.ModIDs, p.ModIDs)
		p.MapFolders = driver.FirstSlice(res.MapFolders, p.MapFolders)
		if len(res.ConfigSchema) > 0 {
			p.ConfigSchema = res.ConfigSchema
		}
	}
	return p, nil
}

func downloadSteamCMDWorkshopItem(ctx context.Context, rt driver.Runtime, rep driver.Reporter, art *registryv1.Artifact, appID, workshopID string, steamAppID int, item *addonmirror.Item, tmpDir string) (*driver.BuiltPayload, error) {
	installDir := filepath.Join(rt.WorkDir(), "steam-workshop", appID)
	contentDir := filepath.Join(installDir, "steamapps", "workshop", "content", appID, workshopID)
	_ = os.RemoveAll(contentDir)

	label := fmt.Sprintf("下载创意工坊模组 %s", workshopID)
	args := []string{"+workshop_download_item", appID, workshopID, "validate"}
	if err := runWithRetry(ctx, rt, rep, 10, label, installDir, args); err != nil {
		return nil, fmt.Errorf("steamcmd workshop_download_item %s/%s: %w", appID, workshopID, err)
	}
	info, err := os.Stat(contentDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workshop content directory %q missing after download", contentDir)
	}
	rel := &addonmirror.ResolvedRelease{}
	applyItemToRelease(rel, item)
	modIDs, mapFolders := rel.ModIDs, rel.MapFolders
	var configSchema []registryv1.AddonConfigField
	if insp, ok := driver.LookupWorkshopInspector(steamAppID); ok {
		res := insp.InspectDir(contentDir)
		modIDs = driver.FirstSlice(res.ModIDs, modIDs)
		mapFolders = driver.FirstSlice(res.MapFolders, mapFolders)
		if len(res.ConfigSchema) > 0 {
			configSchema = res.ConfigSchema
		}
	}
	if rep != nil {
		rep.LogLine("正在将模组目录打包并计算 SHA-256 校验...")
		rep.Stage(registryv1.ArtifactStageArchiving, 72, "正在打包模组文件")
	}
	dst := filepath.Join(tmpDir, "workshop-"+workshopID+".tar.gz")
	size, shaHex, err := driver.ArchiveTreeTarGzWithProgress(ctx, contentDir, dst, rep)
	if err != nil {
		return nil, fmt.Errorf("archive workshop item %s: %w", workshopID, err)
	}
	_ = os.RemoveAll(filepath.Join(installDir, "steamapps", "workshop", "downloads"))
	_ = os.RemoveAll(filepath.Join(installDir, "steamapps", "workshop", "temp"))
	return &driver.BuiltPayload{
		LocalPath: dst, StagingDir: contentDir, FileName: filepath.Base(dst), MediaType: "application/gzip",
		ArchiveFormat: registryv1.ArchiveFormatTarGz, SizeBytes: size, SHA256Hex: shaHex,
		Build: rel.Build, ModIDs: modIDs, MapFolders: mapFolders, ConfigSchema: configSchema,
		Dependencies: driver.MergeDeps(rel.Dependencies, art.Status.Dependencies),
		PublishedAt:  rel.PublishedAt, Changelog: rel.Changelog, UpstreamURL: rel.UpstreamURL,
	}, nil
}

func applyItemToRelease(rel *addonmirror.ResolvedRelease, item *addonmirror.Item) {
	if rel == nil || item == nil {
		return
	}
	rel.ModIDs = item.ModIDs
	rel.MapFolders = item.MapFolders
	rel.Dependencies = item.Dependencies
	rel.Changelog = item.Changelog
	rel.UpstreamURL = item.UpstreamURL
	if item.Revision > 0 {
		rel.Build = fmt.Sprintf("rev-%d", item.Revision)
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(item.UpdatedAt)); err == nil {
		utc := t.UTC()
		rel.PublishedAt = &utc
	}
}

func readManifestBuildID(staging, appID string) string {
	raw, err := os.ReadFile(filepath.Join(staging, "steamapps", "appmanifest_"+appID+".acf"))
	if err != nil {
		return ""
	}
	if m := steamBuildIDRe.FindStringSubmatch(string(raw)); len(m) == 2 {
		return m[1]
	}
	return ""
}

func readManifestPublishedAt(staging, appID string) *time.Time {
	raw, err := os.ReadFile(filepath.Join(staging, "steamapps", "appmanifest_"+appID+".acf"))
	if err != nil {
		return nil
	}
	m := steamLastUpdatedRe.FindStringSubmatch(string(raw))
	if len(m) != 2 {
		return nil
	}
	sec, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || sec <= 0 {
		return nil
	}
	utc := time.Unix(sec, 0).UTC()
	return &utc
}

func defaultWorkshopFileName(steamAppID int, pkg, raw string) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	if steamAppID == addonmirror.SteamAppLeft4Dead2 {
		return "workshop-" + pkg + ".vpk"
	}
	return "workshop-" + pkg + ".zip"
}

func inferSteamArchiveFormat(fileName, dlURL string) string {
	low := strings.ToLower(fileName + " " + dlURL)
	switch {
	case strings.Contains(low, ".zip"):
		return registryv1.ArchiveFormatZip
	case strings.Contains(low, ".tmod"):
		return registryv1.ArchiveFormatTMod
	default:
		return registryv1.ArchiveFormatRaw
	}
}

func buildCS2WorkshopManifestPayload(
	rep driver.Reporter,
	art *registryv1.Artifact,
	workshopID string,
	item *addonmirror.Item,
	rel *addonmirror.ResolvedRelease,
	tmpDir string,
) (*driver.BuiltPayload, error) {
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageArchiving, 75, "正在生成 CS2 创意工坊地图索引")
		rep.Logf("CS2 启动将通过 +host_workshop_map %s 自动拉取地图，生成轻量元数据索引。", workshopID)
	}
	title := "workshop-" + workshopID
	updatedAt := ""
	if item != nil {
		title = driver.FirstStr(item.Title, title)
		updatedAt = strings.TrimSpace(item.UpdatedAt)
	}
	raw, err := json.Marshal(struct {
		WorkshopID string `json:"workshopId"`
		AppID      int    `json:"appId"`
		Title      string `json:"title,omitempty"`
		Version    string `json:"version"`
		UpdatedAt  string `json:"updatedAt,omitempty"`
	}{
		WorkshopID: workshopID, AppID: addonmirror.SteamAppCS2, Title: title,
		Version: strings.TrimSpace(art.Spec.Version), UpdatedAt: updatedAt,
	})
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(tmpDir, "workshop-"+workshopID+".json")
	if err := os.WriteFile(dst, append(raw, '\n'), 0o644); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(append(raw, '\n'))
	return &driver.BuiltPayload{
		LocalPath: dst, FileName: filepath.Base(dst), MediaType: "application/json",
		ArchiveFormat: registryv1.ArchiveFormatRaw, SizeBytes: int64(len(raw) + 1),
		SHA256Hex: hex.EncodeToString(sum[:]), Build: rel.Build,
		MapFolders:   []string{workshopID},
		Dependencies: driver.MergeDeps(rel.Dependencies, art.Status.Dependencies),
		PublishedAt:  rel.PublishedAt, Changelog: rel.Changelog, UpstreamURL: rel.UpstreamURL,
	}, nil
}
