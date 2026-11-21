package fabricmc

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

type stubRuntime struct{}

func (stubRuntime) WorkDir() string                 { return "" }
func (stubRuntime) SteamExec() driver.SteamExecutor { return nil }
func (stubRuntime) ReusableArtifactPayload(context.Context, driver.Reporter, *registryv1.Artifact, string, *time.Time, string) *driver.BuiltPayload {
	return nil
}
func (stubRuntime) DownloadHTTPPayload(context.Context, driver.Reporter, *addonmirror.Service, *registryv1.Artifact, *addonmirror.ResolvedRelease, string) (*driver.BuiltPayload, error) {
	return nil, nil
}
func (stubRuntime) InspectCachedWorkshopPayload(context.Context, driver.Reporter, *registryv1.Artifact, int, *addonmirror.ResolvedRelease, string) *driver.BuiltPayload {
	return nil
}

func sha1Hex(b []byte) string {
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:])
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func startMockFabricServer(t *testing.T, serverJar, asmJar, mixinJar []byte) *httptest.Server {
	t.Helper()
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/versions/game", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version":"26.3","stable":true}]`))
	})
	mux.HandleFunc("/v2/versions/loader", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version":"0.20.1","stable":true}]`))
	})
	mux.HandleFunc("/v2/versions/loader/26.3/0.20.1/server/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{
			"id":"fabric-loader-0.20.1-26.3",
			"releaseTime":"2026-11-20T19:08:25+0000",
			"mainClass":"net.fabricmc.loader.impl.launch.knot.KnotServer",
			"arguments":{"jvm":["-XX:+UnlockDiagnosticVMOptions","-XX:-UseBASE64Intrinsics"]},
			"libraries":[
				{"name":"org.ow2.asm:asm:9.10.1","url":"%s/maven/","sha256":%q},
				{"name":"net.fabricmc:sponge-mixin:0.17.5+mixin.0.8.7","url":"%s/maven/"}
			]
		}`, baseURL, sha256Hex(asmJar), baseURL)
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"latest":{"release":"26.3"},"versions":[{"id":"26.3","type":"release","url":"%s/mojang/26.3.json"}]}`, baseURL)
	})
	mux.HandleFunc("/mojang/26.3.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"releaseTime":"2026-11-18T12:00:00Z","downloads":{"server":{"url":"%s/mojang/server.jar","sha1":%q}}}`, baseURL, sha1Hex(serverJar))
	})
	mux.HandleFunc("/mojang/server.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(serverJar) })
	mux.HandleFunc("/maven/org/ow2/asm/asm/9.10.1/asm-9.10.1.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(asmJar) })
	mux.HandleFunc("/maven/net/fabricmc/sponge-mixin/0.17.5+mixin.0.8.7/sponge-mixin-0.17.5+mixin.0.8.7.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(mixinJar) })
	srv := httptest.NewServer(mux)
	baseURL = srv.URL
	t.Cleanup(srv.Close)
	return srv
}

func readTarGzEntries(t *testing.T, archivePath string) map[string][]byte {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()
	out := make(map[string][]byte)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry %s: %v", hdr.Name, err)
		}
		out[hdr.Name] = data
	}
	return out
}

func TestBundleDriverSyncOfflineSelfContainedTarGz(t *testing.T) {
	serverJar := []byte("mock-vanilla-server-jar")
	asmJar := []byte("mock-asm-jar")
	mixinJar := []byte("mock-sponge-mixin-jar")
	srv := startMockFabricServer(t, serverJar, asmJar, mixinJar)

	tmpDir := t.TempDir()
	svc, src := addonmirror.NewForRegistry(nil, config.RegistryConfig{}, addonmirror.RegistryParams{
		Provider: registryv1.UpstreamProviderFabricMC,
		Endpoint: srv.URL,
		Format:   registryv1.FormatServerPackage,
	})
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "mc-fabric.fabric.26.3"},
		Spec:     registryv1.ArtifactSpec{Repository: "mc-fabric", Package: "fabric", Version: "26.3"},
	}
	drv := newBundleDriver(srv.URL + "/manifest.json")
	payload, err := drv.Sync(context.Background(), driver.SyncRequest{
		Runtime: stubRuntime{}, Mirror: svc, Source: src, Artifact: art, TmpDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("bundleDriver.Sync failed: %v", err)
	}
	if payload.ArchiveFormat != registryv1.ArchiveFormatTarGz || payload.TargetPath != "." || payload.Build != "26.3+0.20.1" {
		t.Fatalf("unexpected bundle payload metadata: %+v", payload)
	}
	entries := readTarGzEntries(t, payload.LocalPath)
	assertBundleContents(t, tmpDir, entries, serverJar, asmJar, mixinJar)
}

func assertBundleContents(t *testing.T, tmpDir string, entries map[string][]byte, serverJar, asmJar, mixinJar []byte) {
	t.Helper()
	if string(entries["server.jar"]) != string(serverJar) {
		t.Fatalf("server.jar mismatch")
	}
	if string(entries["libraries/org/ow2/asm/asm/9.10.1/asm-9.10.1.jar"]) != string(asmJar) {
		t.Fatalf("asm library mismatch")
	}
	if string(entries["libraries/net/fabricmc/sponge-mixin/0.17.5+mixin.0.8.7/sponge-mixin-0.17.5+mixin.0.8.7.jar"]) != string(mixinJar) {
		t.Fatalf("mixin library mismatch")
	}
	if string(entries["fabric-server-launcher.properties"]) != "serverJar=server.jar\n" {
		t.Fatalf("unexpected fabric-server-launcher.properties: %q", string(entries["fabric-server-launcher.properties"]))
	}
	if string(entries["fabric-jvm.args"]) != "-XX:+UnlockDiagnosticVMOptions\n-XX:-UseBASE64Intrinsics\n" {
		t.Fatalf("unexpected fabric-jvm.args: %q", string(entries["fabric-jvm.args"]))
	}
	verifyLauncherJarManifest(t, tmpDir, entries["fabric-server-launch.jar"])
}

func verifyLauncherJarManifest(t *testing.T, tmpDir string, rawJar []byte) {
	t.Helper()
	if len(rawJar) == 0 {
		t.Fatal("missing fabric-server-launch.jar in bundle")
	}
	jarPath := filepath.Join(tmpDir, "launch.jar")
	if err := os.WriteFile(jarPath, rawJar, 0o644); err != nil {
		t.Fatalf("write launch.jar: %v", err)
	}
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open launch.jar zip: %v", err)
	}
	defer func() { _ = zr.Close() }()
	jarFiles := make(map[string]string)
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", zf.Name, err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		jarFiles[zf.Name] = string(b)
	}
	if jarFiles["fabric-server-launch.properties"] != "launch.mainClass=net.fabricmc.loader.impl.launch.knot.KnotServer\n" {
		t.Fatalf("unexpected launch properties: %q", jarFiles["fabric-server-launch.properties"])
	}
	mf := jarFiles["META-INF/MANIFEST.MF"]
	for _, line := range strings.Split(strings.TrimSuffix(mf, "\r\n"), "\r\n") {
		if len(line) > manifestMaxLineBytes {
			t.Fatalf("manifest line exceeds %d bytes (%d): %q", manifestMaxLineBytes, len(line), line)
		}
	}
	unfolded := strings.ReplaceAll(mf, "\r\n ", "")
	if !strings.Contains(unfolded, "Main-Class: "+addonmirror.DefaultFabricServerMainClass) ||
		!strings.Contains(unfolded, "libraries/org/ow2/asm/asm/9.10.1/asm-9.10.1.jar") ||
		!strings.Contains(unfolded, "libraries/net/fabricmc/sponge-mixin/0.17.5+mixin.0.8.7/sponge-mixin-0.17.5+mixin.0.8.7.jar") {
		t.Fatalf("unexpected unfolded MANIFEST.MF:\n%s", unfolded)
	}
}

func TestMavenCoordinateToPathVariants(t *testing.T) {
	cases := []struct {
		coord string
		want  string
	}{
		{"org.ow2.asm:asm:9.10.1", "org/ow2/asm/asm/9.10.1/asm-9.10.1.jar"},
		{"net.fabricmc:sponge-mixin:0.17.5+mixin.0.8.7", "net/fabricmc/sponge-mixin/0.17.5+mixin.0.8.7/sponge-mixin-0.17.5+mixin.0.8.7.jar"},
		{"com.mojang:datafixerupper:8.0.16:natives-linux@zip", "com/mojang/datafixerupper/8.0.16/datafixerupper-8.0.16-natives-linux.zip"},
	}
	for _, tc := range cases {
		got, err := mavenCoordinateToPath(tc.coord)
		if err != nil || got != tc.want {
			t.Fatalf("mavenCoordinateToPath(%q) = (%q, %v), want %q", tc.coord, got, err, tc.want)
		}
	}
	if _, err := mavenCoordinateToPath("invalid-coord"); err == nil {
		t.Fatal("expected error for invalid maven coordinate")
	}
}

func TestDownloadServerJarFallsBackAndVerifiesSHA1(t *testing.T) {
	wantJar := []byte("verified-server-jar-bytes")
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream eof", http.StatusBadGateway)
	}))
	defer primarySrv.Close()

	fallbackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good.jar" {
			_, _ = w.Write(wantJar)
			return
		}
		_, _ = w.Write([]byte("corrupted-bytes"))
	}))
	defer fallbackSrv.Close()

	tmpDir := t.TempDir()
	rel := &addonmirror.ResolvedRelease{
		Version:              "26.3",
		Build:                sha1Hex(wantJar),
		DownloadURL:          primarySrv.URL + "/bad.jar",
		FallbackDownloadURLs: []string{fallbackSrv.URL + "/good.jar"},
	}
	if err := downloadServerJar(context.Background(), nil, http.DefaultClient, rel, tmpDir); err != nil {
		t.Fatalf("expected fallback server.jar download to succeed, got %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tmpDir, "server.jar"))
	if err != nil || string(got) != string(wantJar) {
		t.Fatalf("server.jar mismatch after fallback: err=%v len=%d", err, len(got))
	}

	badRel := &addonmirror.ResolvedRelease{
		Version:              "26.3",
		Build:                sha1Hex(wantJar),
		DownloadURL:          primarySrv.URL + "/bad.jar",
		FallbackDownloadURLs: []string{fallbackSrv.URL + "/corrupted.jar"},
	}
	if err := downloadServerJar(context.Background(), nil, http.DefaultClient, badRel, t.TempDir()); err == nil {
		t.Fatal("expected SHA-1 mismatch error on corrupted fallback")
	}
}
