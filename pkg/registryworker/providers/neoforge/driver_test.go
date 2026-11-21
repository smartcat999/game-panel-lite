package neoforge

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func buildMockInstallerZip(t *testing.T, baseURL string, libBytes, embeddedBytes []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	profileJSON := fmt.Sprintf(`{
		"version":"neoforge-21.4.161",
		"minecraft":"1.21.4",
		"serverJarPath":"{LIBRARY_DIR}/net/minecraft/server/{MINECRAFT_VERSION}/server-{MINECRAFT_VERSION}.jar",
		"data":{"MOJMAPS":{"server":"[net.minecraft:server:1.21.4-20241203:mappings@txt]"}},
		"libraries":[
			{"name":"org.example:tool:1.0","downloads":{"artifact":{"path":"org/example/tool/1.0/tool-1.0.jar","url":"%s/maven/org/example/tool/1.0/tool-1.0.jar","sha1":%q}}},
			{"name":"net.neoforged:embedded:1.0","downloads":{"artifact":{"path":"net/neoforged/embedded/1.0/embedded-1.0.jar"}}}
		]
	}`, baseURL, sha1Hex(libBytes))
	versionJSON := fmt.Sprintf(`{
		"id":"neoforge-21.4.161",
		"libraries":[
			{"name":"net.neoforged.fancymodloader:loader:6.0.18","downloads":{"artifact":{"path":"net/neoforged/fancymodloader/loader/6.0.18/loader-6.0.18.jar","url":"%s/maven/net/neoforged/fancymodloader/loader/6.0.18/loader-6.0.18.jar","sha1":%q}}}
		]
	}`, baseURL, sha1Hex(libBytes))
	writeZipBytes(t, zw, "install_profile.json", []byte(profileJSON))
	writeZipBytes(t, zw, "version.json", []byte(versionJSON))
	writeZipBytes(t, zw, "maven/net/neoforged/embedded/1.0/embedded-1.0.jar", embeddedBytes)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func writeZipBytes(t *testing.T, zw *zip.Writer, name string, data []byte) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create %s: %v", name, err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("zip write %s: %v", name, err)
	}
}

func startMockNeoForgeServer(t *testing.T, serverJar, mappingsTxt, libBytes, embeddedBytes []byte) *httptest.Server {
	t.Helper()
	var baseURL string
	var installerZip []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/api/maven/versions/releases/net/neoforged/neoforge", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"versions":["21.4.161"]}`))
	})
	mux.HandleFunc("/releases/net/neoforged/neoforge/21.4.161/neoforge-21.4.161-installer.jar", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(installerZip)
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"latest":{"release":"1.21.4"},"versions":[{"id":"1.21.4","type":"release","url":"%s/mojang/1.21.4.json"}]}`, baseURL)
	})
	mux.HandleFunc("/mojang/1.21.4.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"releaseTime":"2024-12-03T10:12:57Z","downloads":{"server":{"url":"%s/mojang/server.jar","sha1":%q},"server_mappings":{"url":"%s/mojang/mappings.txt","sha1":%q}}}`,
			baseURL, sha1Hex(serverJar), baseURL, sha1Hex(mappingsTxt))
	})
	mux.HandleFunc("/mojang/server.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(serverJar) })
	mux.HandleFunc("/mojang/mappings.txt", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(mappingsTxt) })
	mux.HandleFunc("/maven/org/example/tool/1.0/tool-1.0.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(libBytes) })
	mux.HandleFunc("/maven/net/neoforged/fancymodloader/loader/6.0.18/loader-6.0.18.jar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(libBytes) })
	srv := httptest.NewServer(mux)
	baseURL = srv.URL
	installerZip = buildMockInstallerZip(t, baseURL, libBytes, embeddedBytes)
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
		data, rErr := io.ReadAll(tr)
		if rErr != nil {
			t.Fatalf("read tar entry %s: %v", hdr.Name, rErr)
		}
		out[hdr.Name] = data
	}
	return out
}

func TestNeoForgeBundleDriverSyncOfflineTarGz(t *testing.T) {
	serverJar := []byte("mock-vanilla-1.21.4")
	mappingsTxt := []byte("mock-mojmaps-1.21.4")
	libBytes := []byte("mock-maven-lib")
	embeddedBytes := []byte("mock-embedded-jar")
	srv := startMockNeoForgeServer(t, serverJar, mappingsTxt, libBytes, embeddedBytes)

	svc, src := addonmirror.NewForRegistry(nil, config.RegistryConfig{}, addonmirror.RegistryParams{
		Provider: registryv1.UpstreamProviderNeoForge, Endpoint: srv.URL, Format: registryv1.FormatServerPackage,
	})
	art := &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: "mc-neoforge.neoforge.1.21.4"},
		Spec:     registryv1.ArtifactSpec{Repository: "mc-neoforge", Package: "neoforge", Version: "1.21.4"},
	}
	drv := newBundleDriver(srv.URL+"/manifest.json", mockNeoForgeInstallerRunner(t, serverJar, mappingsTxt))
	payload, err := drv.Sync(context.Background(), driver.SyncRequest{
		Runtime: stubRuntime{}, Mirror: svc, Source: src, Artifact: art, TmpDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("bundleDriver.Sync failed: %v", err)
	}
	if payload.ArchiveFormat != registryv1.ArchiveFormatTarGz || payload.TargetPath != "." || payload.Build != "1.21.4+21.4.161" {
		t.Fatalf("unexpected NeoForge bundle payload: %+v", payload)
	}
	entries := readTarGzEntries(t, payload.LocalPath)
	assertNeoForgeBundleEntries(t, entries, serverJar, mappingsTxt, libBytes, embeddedBytes)
}

func mockNeoForgeInstallerRunner(t *testing.T, wantServer, wantMaps []byte) installerRunner {
	t.Helper()
	return func(_ context.Context, _ string, stagingDir string, _ driver.Reporter) error {
		gotServer, err := os.ReadFile(filepath.Join(stagingDir, "libraries/net/minecraft/server/1.21.4/server-1.21.4.jar"))
		if err != nil || string(gotServer) != string(wantServer) {
			t.Fatalf("missing or invalid server.jar before offline install: %v", err)
		}
		gotMaps, err := os.ReadFile(filepath.Join(stagingDir, "libraries/net/minecraft/server/1.21.4-20241203/server-1.21.4-20241203-mappings.txt"))
		if err != nil || string(gotMaps) != string(wantMaps) {
			t.Fatalf("missing or invalid mojmaps before offline install: %v", err)
		}
		argsDst := filepath.Join(stagingDir, "libraries/net/neoforged/neoforge/21.4.161/unix_args.txt")
		if err := os.MkdirAll(filepath.Dir(argsDst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(argsDst, []byte("-Dmock=neoforge\ncpw.mods.bootstraplauncher.BootstrapLauncher\n"), 0o644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stagingDir, "run.bat"), []byte("echo win"), 0o644)
	}
}

func assertNeoForgeBundleEntries(t *testing.T, entries map[string][]byte, serverJar, mappingsTxt, libBytes, embeddedBytes []byte) {
	t.Helper()
	if string(entries["libraries/net/minecraft/server/1.21.4/server-1.21.4.jar"]) != string(serverJar) {
		t.Fatalf("expected vanilla server.jar inside bundle")
	}
	if string(entries["libraries/net/minecraft/server/1.21.4-20241203/server-1.21.4-20241203-mappings.txt"]) != string(mappingsTxt) {
		t.Fatalf("expected server_mappings.txt inside bundle")
	}
	if string(entries["libraries/org/example/tool/1.0/tool-1.0.jar"]) != string(libBytes) ||
		string(entries["libraries/net/neoforged/embedded/1.0/embedded-1.0.jar"]) != string(embeddedBytes) {
		t.Fatalf("expected maven & embedded jars in bundle")
	}
	if string(entries["neoforge-unix-args.txt"]) != "-Dmock=neoforge\ncpw.mods.bootstraplauncher.BootstrapLauncher\n" {
		t.Fatalf("expected promoted neoforge-unix-args.txt, got %q", entries["neoforge-unix-args.txt"])
	}
	if _, hasBat := entries["run.bat"]; hasBat {
		t.Fatalf("expected run.bat stripped from Linux server bundle")
	}
}
