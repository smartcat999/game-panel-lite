package neoforge

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const (
	maxDownloadAttempts    = 3
	librarySyncConcurrency = 6
)

var errPermanentHTTPStatus = errors.New("non-retriable HTTP status")

type installerMeta struct {
	MCVersion     string
	NFVersion     string
	ServerJarPath string
	MojmapsCoord  string
	Libraries     []mavenLibrary
}

type mavenLibrary struct {
	Name string
	Path string
	URL  string
	SHA1 string
}

type profileLibraryJSON struct {
	Name      string `json:"name"`
	Downloads struct {
		Artifact struct {
			Path string `json:"path"`
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"artifact"`
	} `json:"downloads"`
}

func downloadInstallerJar(
	ctx context.Context,
	req driver.SyncRequest,
	client *http.Client,
	rel *addonmirror.ResolvedRelease,
	preferMirror *atomic.Bool,
) (string, error) {
	if req.Reporter != nil {
		req.Reporter.Stage(registryv1.ArtifactStageDownloading, 6, "正在下载 NeoForge 安装器")
	}
	installerPath := filepath.Join(req.TmpDir, "neoforge-installer.jar")
	urls := driver.CollectCandidateURLs(rel.DownloadURL, rel.FallbackDownloadURLs)
	usedIdx, err := downloadVerifiedWithFallback(ctx, client, urls, installerPath, "", "", req.Reporter, 6, 16)
	if err != nil {
		return "", fmt.Errorf("download NeoForge installer: %w", err)
	}
	if usedIdx > 0 || isMirrorURL(urls[usedIdx]) {
		preferMirror.Store(true)
	}
	return installerPath, nil
}

func isMirrorURL(rawURL string) bool {
	mirrorHost := strings.TrimPrefix(strings.TrimRight(addonmirror.DefaultNeoForgeMirrorBase, "/"), "https://")
	return mirrorHost != "" && strings.Contains(strings.ToLower(rawURL), strings.ToLower(mirrorHost))
}

func readInstallerMeta(installerPath string, rel *addonmirror.ResolvedRelease) (*installerMeta, error) {
	zr, err := zip.OpenReader(installerPath)
	if err != nil {
		return nil, fmt.Errorf("open NeoForge installer jar: %w", err)
	}
	defer func() { _ = zr.Close() }()

	prof, err := decodeInstallerProfile(&zr.Reader)
	if err != nil {
		return nil, err
	}
	verLibs, err := decodeVersionLibraries(&zr.Reader)
	if err != nil {
		return nil, err
	}
	mcVer := driver.FirstStr(prof.Minecraft, rel.Version)
	nfVer := resolveNFVersion(prof.Version, rel.Build)
	jarRel := resolveServerJarPath(prof.ServerJarPath, mcVer)
	return &installerMeta{
		MCVersion:     mcVer,
		NFVersion:     nfVer,
		ServerJarPath: jarRel,
		MojmapsCoord:  extractMojmapsCoord(prof.Data),
		Libraries:     mergeUniqueLibraries(prof.Libraries, verLibs),
	}, nil
}

type rawInstallerProfile struct {
	Version       string `json:"version"`
	Minecraft     string `json:"minecraft"`
	ServerJarPath string `json:"serverJarPath"`
	Data          map[string]struct {
		Server string `json:"server"`
	} `json:"data"`
	Libraries []profileLibraryJSON `json:"libraries"`
}

func decodeInstallerProfile(zr *zip.Reader) (*rawInstallerProfile, error) {
	raw, err := readZipFileBytes(zr, "install_profile.json")
	if err != nil {
		return nil, err
	}
	var prof rawInstallerProfile
	if err := json.Unmarshal(raw, &prof); err != nil {
		return nil, fmt.Errorf("decode install_profile.json: %w", err)
	}
	return &prof, nil
}

func decodeVersionLibraries(zr *zip.Reader) ([]profileLibraryJSON, error) {
	raw, err := readZipFileBytes(zr, "version.json")
	if err != nil {
		return nil, err
	}
	var vj struct {
		Libraries []profileLibraryJSON `json:"libraries"`
	}
	if err := json.Unmarshal(raw, &vj); err != nil {
		return nil, fmt.Errorf("decode version.json: %w", err)
	}
	return vj.Libraries, nil
}

func readZipFileBytes(zr *zip.Reader, target string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != target {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return io.ReadAll(io.LimitReader(rc, 8<<20))
	}
	return nil, fmt.Errorf("%s not found in NeoForge installer", target)
}

func resolveNFVersion(profileVersion, relBuild string) string {
	if _, after, ok := strings.Cut(strings.TrimSpace(relBuild), "+"); ok && after != "" {
		return after
	}
	return strings.TrimPrefix(strings.TrimSpace(profileVersion), "neoforge-")
}

func resolveServerJarPath(rawTemplate, mcVer string) string {
	val := strings.TrimSpace(rawTemplate)
	if val == "" {
		val = "{LIBRARY_DIR}/net/minecraft/server/{MINECRAFT_VERSION}/server-{MINECRAFT_VERSION}.jar"
	}
	repl := strings.NewReplacer(
		"{LIBRARY_DIR}", "libraries",
		"{ROOT}", ".",
		"{MINECRAFT_VERSION}", mcVer,
	)
	cleaned := path.Clean(strings.TrimLeft(filepath.ToSlash(repl.Replace(val)), "/"))
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return path.Join("libraries", "net", "minecraft", "server", mcVer, "server-"+mcVer+".jar")
	}
	return cleaned
}

func extractMojmapsCoord(data map[string]struct {
	Server string `json:"server"`
}) string {
	entry, ok := data["MOJMAPS"]
	if !ok {
		return ""
	}
	raw := strings.TrimSpace(entry.Server)
	return strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
}

func mergeUniqueLibraries(groups ...[]profileLibraryJSON) []mavenLibrary {
	seen := make(map[string]struct{})
	var out []mavenLibrary
	for _, grp := range groups {
		for _, item := range grp {
			lib, ok := toMavenLibrary(item)
			if !ok {
				continue
			}
			if _, exists := seen[lib.Path]; exists {
				continue
			}
			seen[lib.Path] = struct{}{}
			out = append(out, lib)
		}
	}
	return out
}

func toMavenLibrary(item profileLibraryJSON) (mavenLibrary, bool) {
	relPath := strings.TrimLeft(strings.TrimSpace(item.Downloads.Artifact.Path), "/")
	if relPath == "" && strings.TrimSpace(item.Name) != "" {
		if derived, err := mavenCoordinateToPath(item.Name); err == nil {
			relPath = derived
		}
	}
	if relPath == "" {
		return mavenLibrary{}, false
	}
	clean := path.Clean(relPath)
	if strings.HasPrefix(clean, "..") {
		return mavenLibrary{}, false
	}
	return mavenLibrary{
		Name: strings.TrimSpace(item.Name),
		Path: clean,
		URL:  strings.TrimSpace(item.Downloads.Artifact.URL),
		SHA1: strings.TrimSpace(item.Downloads.Artifact.SHA1),
	}, true
}

func stageMojangFiles(
	ctx context.Context,
	req driver.SyncRequest,
	client *http.Client,
	manifestURL string,
	meta *installerMeta,
	stagingDir string,
	preferMirror *atomic.Bool,
) error {
	vanillaRel, err := req.Mirror.ResolveVanillaMC(ctx, manifestURL, meta.MCVersion)
	if err != nil {
		return fmt.Errorf("resolve Mojang vanilla server %q: %w", meta.MCVersion, err)
	}
	if err := downloadMojangAsset(ctx, req.Reporter, client, vanillaRel, filepath.Join(stagingDir, filepath.FromSlash(meta.ServerJarPath)), "正在下载原版服务端 core server.jar", 18, 40, preferMirror); err != nil {
		return err
	}
	if meta.MojmapsCoord == "" {
		return nil
	}
	relPath, err := mavenCoordinateToPath(meta.MojmapsCoord)
	if err != nil {
		return fmt.Errorf("invalid MOJMAPS coordinate %q: %w", meta.MojmapsCoord, err)
	}
	mapsRel, err := req.Mirror.ResolveVanillaMappings(ctx, manifestURL, meta.MCVersion)
	if err != nil {
		if errors.Is(err, addonmirror.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("resolve Mojang server mappings %q: %w", meta.MCVersion, err)
	}
	dst := filepath.Join(stagingDir, "libraries", filepath.FromSlash(relPath))
	return downloadMojangAsset(ctx, req.Reporter, client, mapsRel, dst, "正在下载 Mojang 混淆映射文件", 40, 46, preferMirror)
}

func downloadMojangAsset(
	ctx context.Context,
	rep driver.Reporter,
	client *http.Client,
	rel *addonmirror.ResolvedRelease,
	dstPath, stageMsg string,
	pctFrom, pctTo int32,
	preferMirror *atomic.Bool,
) error {
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageDownloading, pctFrom, stageMsg)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	expectedSHA1 := ""
	if len(strings.TrimSpace(rel.Build)) == 40 {
		expectedSHA1 = strings.TrimSpace(rel.Build)
	}
	urls := driver.CollectCandidateURLs(rel.DownloadURL, rel.FallbackDownloadURLs)
	usedIdx, err := downloadVerifiedWithFallback(ctx, client, urls, dstPath, "sha1", expectedSHA1, rep, pctFrom, pctTo)
	if err != nil {
		return err
	}
	if usedIdx > 0 || isMirrorURL(urls[usedIdx]) {
		preferMirror.Store(true)
	}
	return nil
}

func stageMavenLibraries(
	ctx context.Context,
	rep driver.Reporter,
	client *http.Client,
	installerJar string,
	libs []mavenLibrary,
	stagingDir string,
	preferMirror *atomic.Bool,
) error {
	if len(libs) == 0 {
		return nil
	}
	embedded, err := extractEmbeddedMavenJars(installerJar)
	if err != nil {
		return err
	}
	var done atomic.Int64
	total := len(libs)
	sem := make(chan struct{}, librarySyncConcurrency)
	var firstErr error
	var errMu sync.Mutex
	var wg sync.WaitGroup
	for _, lib := range libs {
		wg.Add(1)
		go func(entry mavenLibrary) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := ensureSingleLibrary(ctx, client, embedded, entry, stagingDir, preferMirror); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			cur := done.Add(1)
			if rep != nil && (cur%8 == 0 || int(cur) == total) {
				pct := driver.ScalePercent(cur, int64(total), 46, 70)
				rep.Stage(registryv1.ArtifactStageDownloading, pct, fmt.Sprintf("正在同步 NeoForge 依赖库 (%d/%d)", cur, total))
			}
		}(lib)
	}
	wg.Wait()
	return firstErr
}

func extractEmbeddedMavenJars(installerJar string) (map[string][]byte, error) {
	zr, err := zip.OpenReader(installerJar)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	out := make(map[string][]byte)
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "maven/") || f.FileInfo().IsDir() {
			continue
		}
		rc, oErr := f.Open()
		if oErr != nil {
			return nil, oErr
		}
		data, rErr := io.ReadAll(io.LimitReader(rc, 32<<20))
		_ = rc.Close()
		if rErr != nil {
			return nil, rErr
		}
		out[strings.TrimPrefix(f.Name, "maven/")] = data
	}
	return out, nil
}

func ensureSingleLibrary(
	ctx context.Context,
	client *http.Client,
	embedded map[string][]byte,
	lib mavenLibrary,
	stagingDir string,
	preferMirror *atomic.Bool,
) error {
	dstPath := filepath.Join(stagingDir, "libraries", filepath.FromSlash(lib.Path))
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	if payload, ok := embedded[lib.Path]; ok {
		return os.WriteFile(dstPath, payload, 0o644)
	}
	urls := libraryCandidateURLs(lib.URL, lib.Path, preferMirror.Load())
	usedIdx, err := downloadVerifiedWithFallback(ctx, client, urls, dstPath, "sha1", lib.SHA1, nil, 0, 0)
	if err != nil {
		return fmt.Errorf("download library %q: %w", lib.Name, err)
	}
	if usedIdx > 0 || isMirrorURL(urls[usedIdx]) {
		preferMirror.Store(true)
	}
	return nil
}

func libraryCandidateURLs(rawPrimary, relPath string, mirrorFirst bool) []string {
	primary := strings.TrimSpace(rawPrimary)
	mirrorURL := strings.TrimRight(addonmirror.DefaultNeoForgeMirrorBase, "/") + "/maven/" + strings.TrimLeft(relPath, "/")
	var ordered []string
	if mirrorFirst {
		ordered = []string{mirrorURL, primary}
	} else {
		ordered = []string{primary, mirrorURL}
	}
	var out []string
	for _, u := range ordered {
		if u != "" && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

func finalizeStagedServerDir(stagingDir, nfVersion string) error {
	unixArgsRel := filepath.Join("libraries", "net", "neoforged", "neoforge", nfVersion, "unix_args.txt")
	unixArgsSrc := filepath.Join(stagingDir, unixArgsRel)
	data, err := os.ReadFile(unixArgsSrc)
	if err != nil {
		return fmt.Errorf("missing generated NeoForge unix_args.txt (%s): %w", unixArgsRel, err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "neoforge-unix-args.txt"), data, 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(stagingDir, "run.bat"))
	entries, _ := os.ReadDir(stagingDir)
	for _, ent := range entries {
		if !ent.IsDir() && strings.HasSuffix(strings.ToLower(ent.Name()), ".log") {
			_ = os.Remove(filepath.Join(stagingDir, ent.Name()))
		}
	}
	return nil
}

func mavenCoordinateToPath(coord string) (string, error) {
	ext := "jar"
	mainPart := strings.TrimSpace(coord)
	if at := strings.IndexByte(mainPart, '@'); at >= 0 {
		ext = strings.TrimSpace(mainPart[at+1:])
		mainPart = strings.TrimSpace(mainPart[:at])
		if ext == "" {
			ext = "jar"
		}
	}
	parts := strings.Split(mainPart, ":")
	if len(parts) < 3 || len(parts) > 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", fmt.Errorf("invalid maven coordinate %q", coord)
	}
	groupPath := strings.ReplaceAll(parts[0], ".", "/")
	artifact, version := parts[1], parts[2]
	fname := artifact + "-" + version
	if len(parts) == 4 && parts[3] != "" {
		fname += "-" + parts[3]
	}
	fname += "." + ext
	return path.Join(groupPath, artifact, version, fname), nil
}

func downloadVerifiedWithFallback(
	ctx context.Context,
	client *http.Client,
	urls []string,
	dstPath, hashAlgo, expectedHex string,
	rep driver.Reporter,
	pctFrom, pctTo int32,
) (int, error) {
	if len(urls) == 0 {
		return -1, errors.New("no download URL provided")
	}
	var lastErr error
	for idx, rawURL := range urls {
		lastErr = downloadVerifiedWithRetries(ctx, client, rawURL, dstPath, hashAlgo, expectedHex, rep, pctFrom, pctTo)
		if lastErr == nil {
			return idx, nil
		}
		if ctx.Err() != nil {
			return -1, lastErr
		}
		if idx+1 < len(urls) && rep != nil {
			rep.Logf("下载源 %s 失败 (%v)，正在切换备用镜像", rawURL, lastErr)
		}
	}
	return -1, lastErr
}

func downloadVerifiedWithRetries(
	ctx context.Context,
	client *http.Client,
	rawURL, dstPath, hashAlgo, expectedHex string,
	rep driver.Reporter,
	pctFrom, pctTo int32,
) error {
	var err error
	for attempt := range maxDownloadAttempts {
		err = downloadVerifiedFile(ctx, client, rawURL, dstPath, hashAlgo, expectedHex, rep, pctFrom, pctTo)
		if err == nil {
			return nil
		}
		_ = os.Remove(dstPath)
		if ctx.Err() != nil || errors.Is(err, errPermanentHTTPStatus) || attempt+1 >= maxDownloadAttempts {
			return err
		}
		client.CloseIdleConnections()
		timer := time.NewTimer(time.Duration(attempt+1) * 150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func downloadVerifiedFile(
	ctx context.Context,
	client *http.Client,
	rawURL, dstPath, hashAlgo, expectedHex string,
	rep driver.Reporter,
	pctFrom, pctTo int32,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errPermanentHTTPStatus, err)
	}
	req.Header.Set("User-Agent", "GamePanel-RegistryWorker/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
			return fmt.Errorf("%w: HTTP %d from %q", errPermanentHTTPStatus, resp.StatusCode, rawURL)
		}
		return fmt.Errorf("HTTP %d from %q", resp.StatusCode, rawURL)
	}
	return writeAndVerifyStream(resp, dstPath, hashAlgo, expectedHex, rep, pctFrom, pctTo)
}

func writeAndVerifyStream(
	resp *http.Response,
	dstPath, hashAlgo, expectedHex string,
	rep driver.Reporter,
	pctFrom, pctTo int32,
) error {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var h hash.Hash
	switch strings.ToLower(hashAlgo) {
	case "sha256":
		h = sha256.New()
	case "sha1":
		h = sha1.New()
	}
	writers := []io.Writer{f}
	if h != nil {
		writers = append(writers, h)
	}
	if rep != nil && resp.ContentLength > 0 {
		writers = append(writers, newPercentTracker(rep, resp.ContentLength, pctFrom, pctTo))
	}
	n, err := io.Copy(io.MultiWriter(writers...), resp.Body)
	if err != nil {
		return err
	}
	if resp.ContentLength > 0 && n != resp.ContentLength {
		return fmt.Errorf("truncated download: got %d of %d bytes", n, resp.ContentLength)
	}
	if h != nil && expectedHex != "" {
		gotHex := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(gotHex, strings.TrimSpace(expectedHex)) {
			return fmt.Errorf("%s mismatch: got %s, want %s", hashAlgo, gotHex, expectedHex)
		}
	}
	return f.Close()
}

type percentTracker struct {
	rep     driver.Reporter
	total   int64
	done    int64
	from    int32
	to      int32
	lastPct int32
	speed   *driver.ByteSpeedTracker
}

func newPercentTracker(rep driver.Reporter, total int64, from, to int32) *percentTracker {
	return &percentTracker{rep: rep, total: total, from: from, to: to, lastPct: from, speed: &driver.ByteSpeedTracker{}}
}

func (p *percentTracker) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	spd := p.speed.Update(p.done)
	pct := driver.ScalePercent(p.done, p.total, p.from, p.to)
	if pct > p.lastPct {
		p.lastPct = pct
		p.rep.Transfer(registryv1.ArtifactStageDownloading, pct, p.done, p.total, spd, fmt.Sprintf("正在下载核心组件 %d%%", pct))
	}
	return len(b), nil
}
