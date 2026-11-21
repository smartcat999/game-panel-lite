package fabricmc

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const (
	defaultLibrariesRepoURL = "https://libraries.minecraft.net/"
	manifestMaxLineBytes    = 72
)

func stageBundleFiles(
	ctx context.Context,
	req driver.SyncRequest,
	client *http.Client,
	prof *fabricProfile,
	vanillaRel *addonmirror.ResolvedRelease,
) (string, error) {
	stagingDir := filepath.Join(req.TmpDir, "fabric-bundle")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	if err := downloadServerJar(ctx, req.Reporter, client, vanillaRel, stagingDir); err != nil {
		return "", err
	}
	classPathEntries, err := downloadLibraries(ctx, req.Reporter, client, prof.Libraries, stagingDir)
	if err != nil {
		return "", err
	}
	if err := writeLauncherJar(filepath.Join(stagingDir, "fabric-server-launch.jar"), prof.MainClass, classPathEntries); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "fabric-server-launcher.properties"), []byte("serverJar=server.jar\n"), 0o644); err != nil {
		return "", err
	}
	if err := writeJVMArgsFile(filepath.Join(stagingDir, "fabric-jvm.args"), prof.Arguments.JVM); err != nil {
		return "", err
	}
	return stagingDir, nil
}

func downloadServerJar(
	ctx context.Context,
	rep driver.Reporter,
	client *http.Client,
	vanillaRel *addonmirror.ResolvedRelease,
	stagingDir string,
) error {
	if rep != nil {
		rep.Stage(registryv1.ArtifactStageDownloading, 8, "正在下载 Mojang 原版服务端核心 server.jar")
	}
	dstPath := filepath.Join(stagingDir, "server.jar")
	expectedSHA1 := ""
	if len(strings.TrimSpace(vanillaRel.Build)) == 40 {
		expectedSHA1 = strings.TrimSpace(vanillaRel.Build)
	}
	return downloadVerifiedFile(ctx, client, vanillaRel.DownloadURL, dstPath, "sha1", expectedSHA1, rep, 8, 50)
}

func downloadLibraries(
	ctx context.Context,
	rep driver.Reporter,
	client *http.Client,
	libs []fabricLibrary,
	stagingDir string,
) ([]string, error) {
	classPath := make([]string, 0, len(libs))
	total := len(libs)
	for idx, lib := range libs {
		relMaven, err := mavenCoordinateToPath(lib.Name)
		if err != nil {
			return nil, err
		}
		repoBase := strings.TrimRight(strings.TrimSpace(lib.URL), "/")
		if repoBase == "" {
			repoBase = strings.TrimRight(defaultLibrariesRepoURL, "/")
		}
		libURL := repoBase + "/" + relMaven
		relInBundle := "libraries/" + relMaven
		dstPath := filepath.Join(stagingDir, filepath.FromSlash(relInBundle))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return nil, err
		}
		algo, expected := pickLibraryChecksum(lib)
		pct := driver.ScalePercent(int64(idx), int64(max(total, 1)), 50, 70)
		if rep != nil {
			rep.Stage(registryv1.ArtifactStageDownloading, pct, fmt.Sprintf("正在下载依赖库 (%d/%d): %s", idx+1, total, path.Base(relMaven)))
		}
		if err := downloadVerifiedFile(ctx, client, libURL, dstPath, algo, expected, nil, 0, 0); err != nil {
			return nil, fmt.Errorf("download library %q: %w", lib.Name, err)
		}
		classPath = append(classPath, relInBundle)
	}
	return classPath, nil
}

func pickLibraryChecksum(lib fabricLibrary) (string, string) {
	if s := strings.TrimSpace(lib.SHA256); s != "" {
		return "sha256", s
	}
	if s := strings.TrimSpace(lib.SHA1); s != "" {
		return "sha1", s
	}
	return "", ""
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

func downloadVerifiedFile(
	ctx context.Context,
	client *http.Client,
	rawURL, dstPath, hashAlgo, expectedHex string,
	rep driver.Reporter,
	pctFrom, pctTo int32,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GamePanel-RegistryWorker/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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

func writeLauncherJar(jarPath, launchMainClass string, classPath []string) error {
	f, err := os.Create(jarPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	manifestText := formatJarManifest(addonmirror.DefaultFabricServerMainClass, classPath)
	if err := writeZipEntry(zw, "META-INF/MANIFEST.MF", []byte(manifestText)); err != nil {
		_ = zw.Close()
		return err
	}
	props := fmt.Sprintf("launch.mainClass=%s\n", strings.TrimSpace(launchMainClass))
	if err := writeZipEntry(zw, "fabric-server-launch.properties", []byte(props)); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func formatJarManifest(mainClass string, classPath []string) string {
	var b strings.Builder
	writeWrappedManifestAttr(&b, "Manifest-Version", "1.0")
	writeWrappedManifestAttr(&b, "Main-Class", mainClass)
	if len(classPath) > 0 {
		writeWrappedManifestAttr(&b, "Class-Path", strings.Join(classPath, " "))
	}
	return b.String()
}

func writeWrappedManifestAttr(b *strings.Builder, key, val string) {
	rest := key + ": " + val
	limit := manifestMaxLineBytes
	for len(rest) > limit {
		b.WriteString(rest[:limit])
		b.WriteString("\r\n ")
		rest = rest[limit:]
		limit = manifestMaxLineBytes - 1
	}
	b.WriteString(rest)
	b.WriteString("\r\n")
}

func writeJVMArgsFile(dstPath string, args []string) error {
	var clean []string
	for _, a := range args {
		if trimmed := strings.TrimSpace(a); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	content := strings.Join(clean, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(dstPath, []byte(content), 0o644)
}
