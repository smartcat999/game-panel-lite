package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func TestMaterializeAllProvidersInstallUpdateCleanup(t *testing.T) {
	workDir, hostDir := t.TempDir(), t.TempDir()
	seedSecurityTestFixtures(t, workDir)
	inst := newSecurityTestInstance("uid-valid-1", "t1", "valid-srv")
	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}

	if err := mat.Materialize(context.Background(), hostDir, inst); err != nil {
		t.Fatalf("Materialize initial: %v", err)
	}
	assertHostFileContent(t, filepath.Join(hostDir, "bin", "server.jar"), "SERVER_BIN_V1")
	assertHostFileContent(t, filepath.Join(hostDir, "plugins", "flat-1.0.jar"), "FLAT_V1")
	assertHostFileContent(t, filepath.Join(hostDir, "Mods", "Calamity.tmod"), "TMOD_V1")
	if _, err := os.Stat(filepath.Join(hostDir, "Mods", "enabled.json")); err != nil {
		t.Fatalf("expected tmod enabled.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hostDir, "mods", "dedicated_server_mods_setup.lua")); err != nil {
		t.Fatalf("expected dst setup lua: %v", err)
	}

	inst.Spec.Addons = inst.Spec.Addons[:1]
	if err := mat.Materialize(context.Background(), hostDir, inst); err != nil {
		t.Fatalf("Materialize after addon removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hostDir, "Mods", "Calamity.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected removed tmod addon cleaned up, stat err=%v", err)
	}
}

func TestMaterializeRejectsParentDirSymlinkAcrossProviders(t *testing.T) {
	cases := []struct {
		name    string
		linkRel string
	}{
		{name: "server-parent", linkRel: "bin"},
		{name: "flatdir-parent", linkRel: "plugins"},
		{name: "tmod-parent", linkRel: "Mods"},
		{name: "dst-parent", linkRel: "mods"},
		{name: "workshoptree-parent", linkRel: "Zomboid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workDir, hostDir, outsideDir := t.TempDir(), t.TempDir(), t.TempDir()
			outsideCanary := filepath.Join(outsideDir, "canary.txt")
			mustWriteTestBytes(t, outsideCanary, []byte("HOST_CANARY"), 0o600)
			seedSecurityTestFixtures(t, workDir)

			if err := os.Symlink(outsideDir, filepath.Join(hostDir, tc.linkRel)); err != nil {
				t.Fatalf("Symlink(%s): %v", tc.linkRel, err)
			}
			mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
			inst := newSecurityTestInstance("uid-"+tc.name, "t1", "srv-"+tc.name)
			if err := mat.Materialize(context.Background(), hostDir, inst); err == nil {
				t.Fatalf("expected Materialize to fail on symlinked directory %s", tc.linkRel)
			}
			assertOutsideUntouched(t, outsideDir, outsideCanary, "HOST_CANARY")
		})
	}
}

func TestMaterializeLeafSymlinkAndConcurrentSwapNeverTouchOutside(t *testing.T) {
	workDir, hostDir, outsideDir := t.TempDir(), t.TempDir(), t.TempDir()
	outsideCanary := filepath.Join(outsideDir, "canary.txt")
	mustWriteTestBytes(t, outsideCanary, []byte("HOST_CANARY"), 0o600)
	seedSecurityTestFixtures(t, workDir)

	for _, rel := range []string{"bin/server.jar", "plugins/flat-1.0.jar", "Mods/Calamity.tmod", "Mods/enabled.json", "mods/dedicated_server_mods_setup.lua"} {
		full := filepath.Join(hostDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Symlink(outsideCanary, full); err != nil {
			t.Fatalf("Symlink(%s): %v", rel, err)
		}
	}

	var stop atomic.Bool
	done := make(chan struct{})
	go runMaterializeSwapAttacker(hostDir, outsideDir, outsideCanary, &stop, done)

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	inst := newSecurityTestInstance("uid-concurrent", "t1", "srv-conc")
	for i := 0; i < 8; i++ {
		_ = volume.RemoveInstanceState(workDir, inst.Metadata.UID)
		_ = mat.Materialize(context.Background(), hostDir, inst)
	}
	stop.Store(true)
	<-done
	assertOutsideUntouched(t, outsideDir, outsideCanary, "HOST_CANARY")
}

func TestMaterializeCrossInstanceIsolation(t *testing.T) {
	workDir := t.TempDir()
	hostA, hostB := t.TempDir(), t.TempDir()
	seedSecurityTestFixtures(t, workDir)
	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}

	instA := newSecurityTestInstance("uid-inst-a", "t1", "inst-a")
	instB := newSecurityTestInstance("uid-inst-b", "t1", "inst-b")
	if err := mat.Materialize(context.Background(), hostA, instA); err != nil {
		t.Fatalf("Materialize A: %v", err)
	}
	if err := mat.Materialize(context.Background(), hostB, instB); err != nil {
		t.Fatalf("Materialize B: %v", err)
	}

	assertBlobIsolatedFromWorkspaces(t, workDir, hexSHA256("SERVER_BIN_V1"), filepath.Join(hostA, "bin", "server.jar"), filepath.Join(hostB, "bin", "server.jar"))
	overwriteInPlaceBytes(t, filepath.Join(hostA, "bin", "server.jar"), "TAMPERED_BIN!")
	overwriteInPlaceBytes(t, filepath.Join(hostA, "mods", "workshop-378160973", "modmain.lua"), "EVIL_LUA_INJ")

	assertHostFileContent(t, filepath.Join(hostB, "bin", "server.jar"), "SERVER_BIN_V1")
	assertHostFileContent(t, blobFilePath(workDir, hexSHA256("SERVER_BIN_V1")), "SERVER_BIN_V1")
	assertHostFileContent(t, filepath.Join(hostB, "mods", "workshop-378160973", "modmain.lua"), "print('dst')")
}

func TestSameLengthCacheCorruptionRepairedOnPrepare(t *testing.T) {
	workDir := t.TempDir()
	cleanBytes := []byte("CLEAN_SERVER_BINARY_BYTES_0123456789")
	tamperedBytes := []byte("EVIL__SERVER_BINARY_BYTES_0123456789")
	sum := sha256.Sum256(cleanBytes)
	hexDigest := hex.EncodeToString(sum[:])

	var downloads atomic.Int32
	srv := newMockRegistryServer(t, "repo.srv.1.0", hexDigest, cleanBytes, &downloads)
	defer srv.Close()

	blobPath := blobFilePath(workDir, hexDigest)
	mustWriteTestBytes(t, blobPath, tamperedBytes, 0o444)

	mat := &BlobWorkspaceMaterializer{
		WorkDir: workDir,
		ClientFn: func() (client.Interface, error) {
			return client.New(&config.ClientConfig{ServerURL: srv.URL})
		},
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "uid-prepare", Tenant: "t1", Name: "prepare-srv"},
		Spec: corev1.InstanceSpec{
			ServerArtifact: &corev1.ArtifactRef{Repository: "repo", Package: "srv", Version: "1.0", TargetPath: "bin/server.jar"},
		},
	}
	if _, err := mat.Prepare(context.Background(), inst); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if downloads.Load() != 1 {
		t.Fatalf("expected corrupted same-length blob to be re-downloaded once, got %d", downloads.Load())
	}
	assertHostFileContent(t, blobPath, string(cleanBytes))
	info, err := os.Stat(blobPath)
	if err != nil || info.Mode().Perm() != 0o444 {
		t.Fatalf("expected repaired blob mode 0444, got err=%v mode=%v", err, info)
	}
}

func TestExternalManifestsAndPostRestoreStateReset(t *testing.T) {
	workDir, hostDir := t.TempDir(), t.TempDir()
	seedSecurityTestFixtures(t, workDir)
	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	inst := newSecurityTestInstance("uid-restore-1", "t1", "restore-srv")

	if err := mat.Materialize(context.Background(), hostDir, inst); err != nil {
		t.Fatalf("Materialize initial: %v", err)
	}
	assertNoPlatformManifestsInHostDir(t, hostDir)
	stateDir := filepath.Join(workDir, "state", "instances", inst.Metadata.UID)
	for _, f := range []string{"server-artifact.json", "addons-manifest.json"} {
		if _, err := os.Stat(filepath.Join(stateDir, f)); err != nil {
			t.Fatalf("expected external manifest %s: %v", f, err)
		}
	}

	if err := os.Remove(filepath.Join(hostDir, "bin", "server.jar")); err != nil {
		t.Fatalf("Remove server.jar: %v", err)
	}
	if err := volume.RemoveInstanceState(workDir, inst.Metadata.UID); err != nil {
		t.Fatalf("RemoveInstanceState: %v", err)
	}
	if err := mat.Materialize(context.Background(), hostDir, inst); err != nil {
		t.Fatalf("Materialize after RemoveInstanceState: %v", err)
	}
	assertHostFileContent(t, filepath.Join(hostDir, "bin", "server.jar"), "SERVER_BIN_V1")
}

func TestBlobCacheRejectsWritableAndHardlinkedCacheFile(t *testing.T) {
	dir := t.TempDir()
	blob := filepath.Join(dir, "sha256-test")
	if err := os.WriteFile(blob, []byte("DATA"), 0o444); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	if !isValidLocalBlob(blob, 4) {
		t.Fatal("expected 0o444 single-link blob to be valid")
	}
	if err := os.Chmod(blob, 0o644); err != nil {
		t.Fatalf("chmod 0644: %v", err)
	}
	if isValidLocalBlob(blob, 4) {
		t.Fatal("expected writable 0o644 blob to be rejected")
	}
	if err := os.Chmod(blob, 0o444); err != nil {
		t.Fatalf("chmod 0444: %v", err)
	}
	if err := os.Link(blob, filepath.Join(dir, "sha256-link")); err != nil {
		t.Fatalf("hardlink blob: %v", err)
	}
	if isValidLocalBlob(blob, 4) {
		t.Fatal("expected hard-linked blob (nlink > 1) to be rejected")
	}

	dstDir := filepath.Join(dir, "extracted")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir extracted: %v", err)
	}
	marker := filepath.Join(dstDir, blobExtractedOkMarker)
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write writable marker: %v", err)
	}
	if err := ensureArchiveExtracted(filepath.Join(dir, "missing.zip"), dstDir, "zip"); err == nil {
		t.Fatal("expected ensureArchiveExtracted not to trust writable 0o644 marker")
	}
	if err := os.Chmod(marker, 0o444); err != nil {
		t.Fatalf("chmod marker 0444: %v", err)
	}
	if err := os.Link(marker, filepath.Join(dir, "marker-link")); err != nil {
		t.Fatalf("hardlink marker: %v", err)
	}
	if err := ensureArchiveExtracted(filepath.Join(dir, "missing.zip"), dstDir, "zip"); err == nil {
		t.Fatal("expected ensureArchiveExtracted not to trust hard-linked marker")
	}
}

func newSecurityTestInstance(uid, tenant, name string) *corev1.Instance {
	return &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: uid, Tenant: tenant, Name: name},
		Spec: corev1.InstanceSpec{
			State:          corev1.InstanceStateRunning,
			ServerArtifact: &corev1.ArtifactRef{Repository: "sec", Package: "srv", Version: "1.0", TargetPath: "bin/server.jar"},
			Addons: []corev1.InstanceAddon{
				{Repository: "sec", Package: "flat", Version: "1.0", InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: "plugins"},
				{Repository: "sec", Package: "calamity", Version: "1.0", InstallMode: corev1.AddonInstallModeTModLatest, TargetDir: "Mods"},
				{Repository: "sec", Package: "378160973", Version: "1.0", InstallMode: corev1.AddonInstallModeDSTLocalMods, TargetDir: "mods"},
				{Repository: "sec", Package: "zmod", Version: "1.0", InstallMode: corev1.AddonInstallModeWorkshopTree, TargetDir: "Zomboid", Config: map[string]string{"ServerName": "servertest"}},
			},
		},
	}
}

func seedSecurityTestFixtures(t *testing.T, workDir string) {
	t.Helper()
	seedRawCachedBlob(t, workDir, "sec.srv.1.0", "server.jar", registryv1.ArchiveFormatJar, "bin/server.jar", "SERVER_BIN_V1")
	seedRawCachedBlob(t, workDir, "sec.flat.1.0", "Essentials.jar", registryv1.ArchiveFormatJar, "", "FLAT_V1")
	seedRawCachedBlob(t, workDir, "sec.calamity.1.0", "Calamity.tmod", registryv1.ArchiveFormatTMod, "", "TMOD_V1")
	seedZipCachedBlob(t, workDir, "sec.378160973.1.0", "dst.zip", "", []string{"dstmod"}, nil, map[string]string{"modmain.lua": "print('dst')"})
	seedZipCachedBlob(t, workDir, "sec.zmod.1.0", "zmod.zip", "", []string{"TestZMod"}, []string{"TestMap"}, map[string]string{"mods/TestZMod/mod.info": "id=TestZMod\n"})
}

func seedRawCachedBlob(t *testing.T, workDir, coord, fileName, archFmt, targetPath, content string) {
	t.Helper()
	hexDigest := hexSHA256(content)
	mustWriteTestBytes(t, blobFilePath(workDir, hexDigest), []byte(content), 0o444)
	meta := &CachedArtifactMeta{
		Coordinate: coord, Digest: "sha256:" + hexDigest, HexDigest: hexDigest,
		SizeBytes: int64(len(content)), ArchiveFormat: archFmt, FileName: fileName, TargetPath: targetPath,
	}
	if err := writeCachedArtifactMeta(workDir, meta); err != nil {
		t.Fatalf("writeCachedArtifactMeta(%s): %v", coord, err)
	}
}

func seedZipCachedBlob(t *testing.T, workDir, coord, fileName, targetPath string, modIDs, mapFolders []string, files map[string]string) {
	t.Helper()
	zipBytes := buildTestZipBytes(t, files)
	hexDigest := hexSHA256(string(zipBytes))
	mustWriteTestBytes(t, blobFilePath(workDir, hexDigest), zipBytes, 0o444)
	extDir := blobExtractedDir(workDir, hexDigest)
	for rel, body := range files {
		mustWriteTestBytes(t, filepath.Join(extDir, filepath.FromSlash(rel)), []byte(body), 0o444)
	}
	mustWriteTestBytes(t, filepath.Join(extDir, blobExtractedOkMarker), []byte("ok\n"), 0o444)
	meta := &CachedArtifactMeta{
		Coordinate: coord, Digest: "sha256:" + hexDigest, HexDigest: hexDigest,
		SizeBytes: int64(len(zipBytes)), ArchiveFormat: registryv1.ArchiveFormatZip,
		FileName: fileName, TargetPath: targetPath, ModIDs: modIDs, MapFolders: mapFolders,
	}
	if err := writeCachedArtifactMeta(workDir, meta); err != nil {
		t.Fatalf("writeCachedArtifactMeta(%s): %v", coord, err)
	}
}

func buildTestZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%s): %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip.Write(%s): %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

func newMockRegistryServer(t *testing.T, coord, hexDigest string, blob []byte, downloads *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/" + registryv1.GroupVersion + "/artifacts/" + coord:
			art := &registryv1.Artifact{
				TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: registryv1.KindArtifact},
				Metadata: metav1.ObjectMeta{Name: coord},
				Status: registryv1.ArtifactStatus{
					Phase: registryv1.ArtifactPhaseReady,
					Descriptor: &registryv1.BlobDescriptor{
						Digest: "sha256:" + hexDigest, SizeBytes: int64(len(blob)),
						FileName: "server.jar", ArchiveFormat: registryv1.ArchiveFormatJar,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(art)
		case "/apis/" + registryv1.GroupVersion + "/blobs/sha256:" + hexDigest:
			downloads.Add(1)
			_, _ = w.Write(blob)
		default:
			http.NotFound(w, r)
		}
	}))
}

func runMaterializeSwapAttacker(hostDir, outsideDir, outsideCanary string, stop *atomic.Bool, done chan struct{}) {
	defer close(done)
	swapDir := filepath.Join(hostDir, "plugins")
	swapLeaf := filepath.Join(swapDir, "flat-1.0.jar")
	for !stop.Load() {
		_ = os.RemoveAll(swapDir)
		_ = os.Symlink(outsideDir, swapDir)
		_ = os.Remove(swapDir)
		_ = os.MkdirAll(swapDir, 0o755)
		_ = os.Symlink(outsideCanary, swapLeaf)
	}
}

func assertBlobIsolatedFromWorkspaces(t *testing.T, workDir, hexDigest, pathA, pathB string) {
	t.Helper()
	infoBlob, err := os.Lstat(blobFilePath(workDir, hexDigest))
	if err != nil {
		t.Fatalf("Lstat blob: %v", err)
	}
	if infoBlob.Mode().Perm() != 0o444 {
		t.Fatalf("blob perm = %o, want 0444", infoBlob.Mode().Perm())
	}
	infoA, err := os.Lstat(pathA)
	if err != nil {
		t.Fatalf("Lstat A: %v", err)
	}
	infoB, err := os.Lstat(pathB)
	if err != nil {
		t.Fatalf("Lstat B: %v", err)
	}
	if os.SameFile(infoBlob, infoA) || os.SameFile(infoA, infoB) {
		t.Fatalf("expected isolated inodes between blob, instA, and instB")
	}
}

func assertNoPlatformManifestsInHostDir(t *testing.T, hostDir string) {
	t.Helper()
	err := filepath.WalkDir(hostDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".gp-") {
			t.Fatalf("unexpected platform manifest/metadata file inside workspace: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", hostDir, err)
	}
}

func assertOutsideUntouched(t *testing.T, outsideDir, canaryPath, want string) {
	t.Helper()
	entries, err := os.ReadDir(outsideDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", outsideDir, err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(canaryPath) {
		t.Fatalf("outsideDir mutated: %v", entries)
	}
	assertHostFileContent(t, canaryPath, want)
}

func assertHostFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		entries, _ := os.ReadDir(filepath.Dir(path))
		t.Fatalf("ReadFile(%s): %v (dir entries: %v)", path, err, entryNames(entries))
	}
	if string(got) != want {
		t.Fatalf("file %s = %q, want %q", path, string(got), want)
	}
}

func entryNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func overwriteInPlaceBytes(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0)
	if err != nil {
		t.Fatalf("OpenFile(%s): %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString(%s): %v", path, err)
	}
}

func mustWriteTestBytes(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestApplyRuntimeObservationMultiStopTransitionID(t *testing.T) {
	a := &Agent{}
	inst := &corev1.Instance{Metadata: metav1.ObjectMeta{Generation: 1}}
	t1 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)
	obsAt1 := t1.Add(time.Minute)
	obsAt2 := t2.Add(time.Minute)

	newLatest := func() *corev1.Instance {
		return &corev1.Instance{
			Status: corev1.InstanceStatus{
				ResourceControl: &corev1.InstanceResourceControl{
					ApprovedRevision: 1,
					CurrentAttempt:   &corev1.InstancePlacementAttempt{ID: "att-1", Epoch: 1},
				},
			},
		}
	}
	l1, l2 := newLatest(), newLatest()
	a.applyRuntimeObservationToStatus(inst, l1, &drvruntime.RuntimeObservation{
		Phase: corev1.InstancePhaseStopped, StartedAt: &t1, ObservedAt: &obsAt1,
	}, nil, "127.0.0.1")
	a.applyRuntimeObservationToStatus(inst, l2, &drvruntime.RuntimeObservation{
		Phase: corev1.InstancePhaseStopped, StartedAt: &t2, ObservedAt: &obsAt2,
	}, nil, "127.0.0.1")
	if l1.Status.TransitionID == "" || l2.Status.TransitionID == "" || l1.Status.TransitionID == l2.Status.TransitionID {
		t.Fatalf("expected distinct non-empty TransitionIDs across two stops in gen 1, got %q and %q", l1.Status.TransitionID, l2.Status.TransitionID)
	}
}
