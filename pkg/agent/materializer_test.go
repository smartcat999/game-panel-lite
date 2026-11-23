package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func seedFakeServerArtifactCache(t *testing.T, workDir, ver, hexDigest, dllContent string) {
	t.Helper()
	coord := "terraria-tmodloader.terraria-tmodloader." + ver
	blobPath := blobFilePath(workDir, hexDigest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobPath, []byte(dllContent), 0o444); err != nil {
		t.Fatal(err)
	}
	extracted := blobExtractedDir(workDir, hexDigest)
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "tModLoader.dll"), []byte(dllContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extracted, "serverconfig.txt"), []byte("#example\n#world=ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := CachedArtifactMeta{
		Coordinate:    coord,
		Digest:        "sha256:" + hexDigest,
		HexDigest:     hexDigest,
		SizeBytes:     int64(len(dllContent)),
		ArchiveFormat: registryv1.ArchiveFormatZip,
		TargetPath:    ".",
	}
	if err := writeCachedArtifactMeta(workDir, &meta); err != nil {
		t.Fatal(err)
	}
}

func assertWorkspaceFile(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(raw) != want {
		t.Fatalf("file %s = %q, want %q", path, string(raw), want)
	}
}

func TestMaterializePreservesManagedInstanceFiles(t *testing.T) {
	workDir := t.TempDir()
	hexV1 := strings.Repeat("1", 64)
	hexV2 := strings.Repeat("2", 64)
	seedFakeServerArtifactCache(t, workDir, "v1", hexV1, "dll-v1")
	seedFakeServerArtifactCache(t, workDir, "v2", hexV2, "dll-v2")

	rendered := "world=/data/Worlds/CalamityWorld.wld\nmodpath=/data/Mods\n"
	hostDir := filepath.Join(workDir, "instances", "terraria-99")
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Files: []corev1.InstanceFile{{Path: "serverconfig.txt", Content: rendered}},
			ServerArtifact: &corev1.ArtifactRef{
				Repository: "terraria-tmodloader",
				Package:    "terraria-tmodloader",
				Version:    "v1",
				TargetPath: ".",
			},
		},
	}

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	if err := volume.WriteInstanceFiles(hostDir, in.Spec.Files); err != nil {
		t.Fatalf("WriteInstanceFiles v1: %v", err)
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize v1: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "tModLoader.dll"), "dll-v1")
	assertWorkspaceFile(t, filepath.Join(hostDir, "serverconfig.txt"), rendered)

	srcCfgInfo, err := os.Stat(filepath.Join(blobExtractedDir(workDir, hexV1), "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dstCfgInfo, err := os.Stat(filepath.Join(hostDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(srcCfgInfo, dstCfgInfo) {
		t.Fatalf("serverconfig.txt must not share inode with stock artifact example file")
	}

	in.Spec.ServerArtifact.Version = "v2"
	if err := volume.WriteInstanceFiles(hostDir, in.Spec.Files); err != nil {
		t.Fatalf("WriteInstanceFiles v2: %v", err)
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize v2: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "tModLoader.dll"), "dll-v2")
	assertWorkspaceFile(t, filepath.Join(hostDir, "serverconfig.txt"), rendered)
}

func seedFakeBlobFile(t *testing.T, workDir, coord, hexDigest, fileName, content string) {
	t.Helper()
	blobPath := blobFilePath(workDir, hexDigest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobPath, []byte(content), 0o444); err != nil {
		t.Fatal(err)
	}
	meta := CachedArtifactMeta{
		Coordinate: coord,
		Digest:     "sha256:" + hexDigest,
		HexDigest:  hexDigest,
		SizeBytes:  int64(len(content)),
		FileName:   fileName,
	}
	if err := writeCachedArtifactMeta(workDir, &meta); err != nil {
		t.Fatal(err)
	}
}

func seedFakeArchiveArtifact(t *testing.T, workDir, coord, hexDigest string, files map[string]string) {
	t.Helper()
	blobPath := blobFilePath(workDir, hexDigest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobPath, []byte("archive-blob"), 0o444); err != nil {
		t.Fatal(err)
	}
	extracted := blobExtractedDir(workDir, hexDigest)
	for rel, body := range files {
		full := filepath.Join(extracted, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	meta := CachedArtifactMeta{
		Coordinate:    coord,
		Digest:        "sha256:" + hexDigest,
		HexDigest:     hexDigest,
		SizeBytes:     int64(len("archive-blob")),
		ArchiveFormat: registryv1.ArchiveFormatZip,
	}
	if err := writeCachedArtifactMeta(workDir, &meta); err != nil {
		t.Fatal(err)
	}
}

func TestFlatDirAndCustomAddonInjector(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "mc-1")
	seedFakeBlobFile(t, workDir, "plugins.geyser.1.0", strings.Repeat("a", 64), "Geyser.jar", "jar-bytes")

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{
				{Repository: "plugins", Package: "geyser", Version: "1.0"},
			},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize flat_dir: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "plugins", "geyser-1.0.jar"), "jar-bytes")

	in.Spec.Addons = []corev1.InstanceAddon{{Repository: "plugins", Package: "geyser", Version: "1.0", InstallMode: "missing-mode"}}
	if err := mat.Materialize(context.Background(), hostDir, in); err == nil {
		t.Fatalf("expected error for unsupported installMode")
	}

	RegisterAddonInjector(&testCustomInjector{})
	in.Spec.Addons[0].InstallMode = "custom-test"
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize custom injector: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "custom.marker"), "geyser")
}

type testCustomInjector struct{ BaseInjector }

func (*testCustomInjector) Mode() string { return "custom-test" }
func (*testCustomInjector) Inject(ws *WorkspaceContext, addon corev1.InstanceAddon, _ *CachedArtifactMeta) error {
	ws.Next.Files = append(ws.Next.Files, "custom.marker")
	return volume.WriteFileAtomicInRoot(ws.Root, "custom.marker", []byte(addon.Package), 0o644)
}

func TestTModLatestAddonLifecycle(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "tmod-1")
	seedFakeArchiveArtifact(t, workDir, "mods.calamity.1.0", strings.Repeat("b", 64), map[string]string{
		"2024.1/CalamityMod.tmod": "old-tmod",
		"2024.2/CalamityMod.tmod": "new-tmod",
	})

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{
				{Repository: "mods", Package: "calamity", Version: "1.0", InstallMode: corev1.AddonInstallModeTModLatest},
			},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize tmod_latest: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "Mods", "CalamityMod.tmod"), "new-tmod")
	assertWorkspaceFile(t, filepath.Join(hostDir, "Mods", "enabled.json"), "[\n  \"CalamityMod\"\n]\n")

	in.Spec.Addons = nil
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Cleanup tmod_latest: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "Mods", "enabled.json"), "[]\n")
}

func TestDSTAndWorkshopTreeAddonLifecycles(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "combo-1")
	seedFakeArchiveArtifact(t, workDir, "ws.111.v1", strings.Repeat("c", 64), map[string]string{"modmain.lua": "return {}"})
	seedFakeArchiveArtifact(t, workDir, "ws.222.v1", strings.Repeat("d", 64), map[string]string{
		"mods/TownMod/mod.info":                  "name=Town\nid=TownMod\n",
		"mods/TownMod/media/maps/TownMap/map.ok": "1",
	})

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{
				{Repository: "ws", Package: "111", Version: "v1", InstallMode: corev1.AddonInstallModeDSTLocalMods},
				{Repository: "ws", Package: "222", Version: "v1", InstallMode: corev1.AddonInstallModeWorkshopTree},
			},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize dst+pz: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "mods", "workshop-111", "modmain.lua"), "return {}")
	assertWorkspaceFile(t, filepath.Join(hostDir, "DoNotStarveTogether", "Cluster_1", "Master", "modoverrides.lua"),
		"return {\n  [\"workshop-111\"] = { enabled = true },\n}\n")
	rawINI, err := os.ReadFile(filepath.Join(hostDir, "Zomboid", "Server", "servertest.ini"))
	if err != nil {
		t.Fatalf("read servertest.ini: %v", err)
	}
	if !strings.Contains(string(rawINI), "WorkshopItems=222") || !strings.Contains(string(rawINI), "Mods=TownMod") ||
		!strings.Contains(string(rawINI), "Map=TownMap;Muldraugh, KY") {
		t.Fatalf("unexpected servertest.ini:\n%s", string(rawINI))
	}

	in.Spec.Addons = nil
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Cleanup dst+pz: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "DoNotStarveTogether", "Cluster_1", "Caves", "modoverrides.lua"), "return {\n}\n")
	cleanedINI, _ := os.ReadFile(filepath.Join(hostDir, "Zomboid", "Server", "servertest.ini"))
	if !strings.Contains(string(cleanedINI), "WorkshopItems=\n") || strings.Contains(string(cleanedINI), "TownMap") {
		t.Fatalf("expected PZ cleanup in servertest.ini:\n%s", string(cleanedINI))
	}
}

func seedArchiveWithSchema(t *testing.T, workDir, coord, hexDigest string, files map[string]string, schema []registryv1.AddonConfigField) {
	t.Helper()
	seedFakeArchiveArtifact(t, workDir, coord, hexDigest, files)
	meta := CachedArtifactMeta{
		Coordinate:    coord,
		Digest:        "sha256:" + hexDigest,
		HexDigest:     hexDigest,
		SizeBytes:     int64(len("archive-blob")),
		ArchiveFormat: registryv1.ArchiveFormatZip,
		ConfigSchema:  schema,
	}
	if err := writeCachedArtifactMeta(workDir, &meta); err != nil {
		t.Fatal(err)
	}
}

func TestDSTModOverridesTypedAndPreserved(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "dst-1")
	schema := []registryv1.AddonConfigField{
		{Name: "scale", Type: registryv1.AddonConfigTypeNumber, Default: "1"},
		{Name: "show_hud", Type: registryv1.AddonConfigTypeBoolean, Default: "true"},
		{Name: "tag", Type: registryv1.AddonConfigTypeText, Default: "100"},
	}
	seedArchiveWithSchema(t, workDir, "ws.333.v1", strings.Repeat("e", 64), map[string]string{"modmain.lua": "return {}"}, schema)

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{
				Repository: "ws", Package: "333", Version: "v1", InstallMode: corev1.AddonInstallModeDSTLocalMods,
				Config: map[string]string{"scale": "1.5", "show_hud": "false", "tag": "100"},
			}},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize typed dst: %v", err)
	}
	masterPath := filepath.Join(hostDir, "DoNotStarveTogether", "Cluster_1", "Master", "modoverrides.lua")
	raw, err := os.ReadFile(masterPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `["scale"] = 1.5`) || !strings.Contains(body, `["show_hud"] = false`) || !strings.Contains(body, `["tag"] = "100"`) {
		t.Fatalf("unexpected typed modoverrides.lua:\n%s", body)
	}

	manual := "return {\n  [\"workshop-333\"] = { enabled = true, configuration_options = { manual_opt = 42 } },\n}\n"
	if err := os.WriteFile(masterPath, []byte(manual), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Spec.Addons[0].Config = nil
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize preserve dst: %v", err)
	}
	preserved, _ := os.ReadFile(masterPath)
	if !strings.Contains(string(preserved), "manual_opt = 42") {
		t.Fatalf("expected existing disk configuration_options preserved, got:\n%s", string(preserved))
	}
}

func TestPZSandboxVarsMerge(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "pz-1")
	schema := []registryv1.AddonConfigField{
		{Name: "MyMod.SpawnRate", Type: registryv1.AddonConfigTypeNumber},
		{Name: "MyMod.HardMode", Type: registryv1.AddonConfigTypeBoolean},
		{Name: "TopFlag", Type: registryv1.AddonConfigTypeText},
	}
	seedArchiveWithSchema(t, workDir, "ws.444.v1", strings.Repeat("f", 64), map[string]string{"mods/MyMod/mod.info": "id=MyMod\n"}, schema)
	varsPath := filepath.Join(hostDir, "Zomboid", "Server", "servertest_SandboxVars.lua")
	if err := os.MkdirAll(filepath.Dir(varsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := "SandboxVars = {\n  VERSION = 5,\n  MyMod = {\n    SpawnRate = 1.0,\n  },\n}\n"
	if err := os.WriteFile(varsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{
				Repository: "ws", Package: "444", Version: "v1", InstallMode: corev1.AddonInstallModeWorkshopTree,
				Config: map[string]string{"MyMod.SpawnRate": "0.75", "MyMod.HardMode": "true", "TopFlag": "fast"},
			}},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize pz sandbox vars: %v", err)
	}
	got, err := os.ReadFile(varsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "SpawnRate = 0.75") || !strings.Contains(text, "HardMode = true") || !strings.Contains(text, `TopFlag = "fast"`) {
		t.Fatalf("unexpected servertest_SandboxVars.lua:\n%s", text)
	}
}

func TestSafeExtractTargetNormalizesWindowsSeparatorsAndRejectsTraversal(t *testing.T) {
	rootDir := t.TempDir()
	got, ok := safeExtractTarget(rootDir, `images\back.tex`)
	want := filepath.Join(rootDir, "images", "back.tex")
	if !ok || got != want {
		t.Fatalf("safeExtractTarget backslash = (%q, %v), want (%q, true)", got, ok, want)
	}
	for _, bad := range []string{`..\evil.txt`, `C:\evil.txt`, `/etc/passwd`, ``, `.`} {
		if target, ok := safeExtractTarget(rootDir, bad); ok {
			t.Fatalf("expected rejection for %q, got %q", bad, target)
		}
	}
}

func TestPendingStatusTransitionAndRuntimeOperationContext(t *testing.T) {
	ag := &Agent{}
	runCtx, cancel := ag.runtimeOperationContext(context.Background())
	defer cancel()
	deadline, hasDeadline := runCtx.Deadline()
	if !hasDeadline || time.Until(deadline) < 10*time.Minute {
		t.Fatalf("expected >= 10m runtime reconcile deadline, got remaining=%v", time.Until(deadline))
	}

	agCustom := &Agent{operationTimeout: 2 * time.Second}
	customCtx, customCancel := agCustom.runtimeOperationContext(context.Background())
	defer customCancel()
	customDeadline, _ := customCtx.Deadline()
	if rem := time.Until(customDeadline); rem <= 0 || rem > 3*time.Second {
		t.Fatalf("expected ~2s custom deadline, got remaining=%v", rem)
	}

	t0 := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	pendingInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "uid-1", Generation: 1},
		Spec:     corev1.InstanceSpec{State: corev1.InstanceStateRunning},
		Status:   corev1.InstanceStatus{Phase: corev1.InstancePhasePending},
	}
	if reason, _, _, ok := ag.pendingStatusTransition(pendingInst); !ok || reason != corev1.ReasonStarting {
		t.Fatalf("pending transition = (%q, %v), want (%q, true)", reason, ok, corev1.ReasonStarting)
	}

	runningInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			UID:         "uid-2",
			Annotations: map[string]string{corev1.AnnotationRestartRequestedAt: t0.Add(time.Minute).Format(time.RFC3339Nano)},
		},
		Spec:   corev1.InstanceSpec{State: corev1.InstanceStateRunning},
		Status: corev1.InstanceStatus{Phase: corev1.InstancePhaseRunning, StartedAt: &t0},
	}
	if reason, _, _, ok := ag.pendingStatusTransition(runningInst); !ok || reason != corev1.ReasonRestarting {
		t.Fatalf("running restart transition = (%q, %v), want (%q, true)", reason, ok, corev1.ReasonRestarting)
	}

	failedInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			UID:         "uid-3",
			Annotations: map[string]string{corev1.AnnotationRestartRequestedAt: t0.Add(time.Minute).Format(time.RFC3339Nano)},
		},
		Spec: corev1.InstanceSpec{State: corev1.InstanceStateRunning},
		Status: corev1.InstanceStatus{
			Phase:      corev1.InstancePhaseFailed,
			Conditions: []metav1.Condition{{Type: metav1.ConditionTypeReady, Status: metav1.ConditionFalse, LastTransitionTime: t0}},
		},
	}
	if reason, _, _, ok := ag.pendingStatusTransition(failedInst); !ok || reason != corev1.ReasonRestarting {
		t.Fatalf("failed retry transition = (%q, %v), want (%q, true)", reason, ok, corev1.ReasonRestarting)
	}
	if _, _, _, ok := ag.pendingStatusTransition(failedInst); ok {
		t.Fatal("expected duplicate failed retry with same restartAt timestamp to be claimed once")
	}
}

func TestPendingStatusTransitionObservedGenerationGap(t *testing.T) {
	ag := &Agent{}
	reconfigInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "uid-4", Generation: 5},
		Spec:     corev1.InstanceSpec{State: corev1.InstanceStateRunning},
		Status:   corev1.InstanceStatus{Phase: corev1.InstancePhaseRunning, ObservedGeneration: 4},
	}
	if reason, _, _, ok := ag.pendingStatusTransition(reconfigInst); !ok || reason != corev1.ReasonRestarting {
		t.Fatalf("generation gap transition = (%q, %v), want (%q, true)", reason, ok, corev1.ReasonRestarting)
	}
}

func TestMaterializeIncrementalReusesUnchangedFilesAndRemovesStaleAddons(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "dst-inc")
	seedFakeArchiveArtifact(t, workDir, "dst.server.v1", strings.Repeat("1", 64), map[string]string{
		"bin/server.txt": "server-v1",
	})
	seedFakeArchiveArtifact(t, workDir, "ws.111.v1", strings.Repeat("2", 64), map[string]string{
		"modmain.lua": "mod-111",
	})
	seedFakeArchiveArtifact(t, workDir, "ws.222.v1", strings.Repeat("3", 64), map[string]string{
		"modmain.lua": "mod-222",
	})

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "uid-inc"},
		Spec: corev1.InstanceSpec{
			ServerArtifact: &corev1.ArtifactRef{Repository: "dst", Package: "server", Version: "v1", TargetPath: "."},
			Addons: []corev1.InstanceAddon{
				{Repository: "ws", Package: "111", Version: "v1", InstallMode: corev1.AddonInstallModeDSTLocalMods},
				{Repository: "ws", Package: "222", Version: "v1", InstallMode: corev1.AddonInstallModeDSTLocalMods},
			},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize initial: %v", err)
	}
	srvPath := filepath.Join(hostDir, "bin", "server.txt")
	mod111Path := filepath.Join(hostDir, "mods", "workshop-111", "modmain.lua")
	mod222Path := filepath.Join(hostDir, "mods", "workshop-222", "modmain.lua")
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, p := range []string{srvPath, mod111Path} {
		if err := os.Chtimes(p, t0, t0); err != nil {
			t.Fatal(err)
		}
	}

	in.Spec.Addons = in.Spec.Addons[:1]
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("Materialize incremental: %v", err)
	}
	storagePath := blobStorageInfoPath(workDir)
	if _, err := os.Stat(storagePath); err != nil {
		t.Fatalf("expected .blob-cache storage capability file %s to exist: %v", storagePath, err)
	}
	for _, p := range []string{srvPath, mod111Path} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(t0) {
			t.Fatalf("expected unchanged file %s to be reused in-place without rewrite (modTime=%v, want=%v)", p, info.ModTime(), t0)
		}
	}
	if _, err := os.Stat(mod222Path); !os.IsNotExist(err) {
		t.Fatalf("expected removed addon file %s to be deleted, stat err=%v", mod222Path, err)
	}
}

func TestNodeNeedsPrewarmExecutionDetectsMissingArtifacts(t *testing.T) {
	job := &corev1.PrewarmJob{
		Spec: corev1.PrewarmJobSpec{
			Artifacts: []corev1.ArtifactRef{
				{Repository: "dst-server", Package: "dst-server", Version: "latest"},
			},
		},
		Status: corev1.PrewarmJobStatus{
			Nodes: []corev1.PrewarmNodeStatus{
				{NodeName: "node-1", Phase: corev1.PrewarmJobPhaseSucceeded},
			},
		},
	}
	if !nodeNeedsPrewarmExecution("node-1", job, map[string]struct{}{}) {
		t.Fatal("expected Succeeded node with missing cached artifact to require re-execution")
	}
	cached := map[string]struct{}{"dst-server.dst-server.latest": {}}
	if nodeNeedsPrewarmExecution("node-1", job, cached) {
		t.Fatal("expected Succeeded node with all artifacts cached to skip execution")
	}
}

func TestEnsureBlobCacheStorageInfoPersistsAndReuses(t *testing.T) {
	workDir := t.TempDir()
	info := EnsureBlobCacheStorageInfo(workDir)
	if info.ProbedAt.IsZero() {
		t.Fatal("expected non-zero ProbedAt from EnsureBlobCacheStorageInfo")
	}
	raw, err := os.ReadFile(blobStorageInfoPath(workDir))
	if err != nil {
		t.Fatalf("read storage.json: %v", err)
	}
	var disk BlobCacheStorageInfo
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("unmarshal storage.json: %v", err)
	}
	disk.ReflinkSupported = false
	updated, err := json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobStorageInfoPath(workDir), updated, 0o644); err != nil {
		t.Fatal(err)
	}
	blobStorageCache.Delete(filepath.Clean(workDir))
	reused := EnsureBlobCacheStorageInfo(workDir)
	if reused.ReflinkSupported || !reused.ProbedAt.Equal(info.ProbedAt) {
		t.Fatalf("expected persisted storage.json to be reused, got %+v (orig=%+v)", reused, info)
	}
}

func seedTestCacheItem(t *testing.T, workDir, coord, hexDigest string) {
	t.Helper()
	if err := writeCachedArtifactMeta(workDir, &CachedArtifactMeta{
		Coordinate: coord,
		Digest:     "sha256:" + hexDigest,
		HexDigest:  hexDigest,
		SizeBytes:  3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(blobFilePath(workDir, hexDigest)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blobFilePath(workDir, hexDigest), []byte("abc"), 0o444); err != nil {
		t.Fatal(err)
	}
	exDir := blobExtractedDir(workDir, hexDigest)
	if err := os.MkdirAll(filepath.Dir(exDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(exDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exDir, blobExtractedOkMarker), []byte("ok\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(exDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(exDir, 0o755) })
}

func TestPruneDisallowedNodeBlobCache(t *testing.T) {
	workDir := t.TempDir()
	hexAllowed := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hexActive := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hexDisallowed := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	seedTestCacheItem(t, workDir, "mc-paper.paper.26.3", hexAllowed)
	seedTestCacheItem(t, workDir, "rust-server.rust.latest", hexActive)
	seedTestCacheItem(t, workDir, "seven-days-server.seven-days-server.latest", hexDisallowed)

	allowedRepos := map[string]struct{}{"mc-paper": {}}
	activeCoords := map[string]struct{}{"rust-server.rust.latest": {}}
	if !PruneDisallowedNodeBlobCache(workDir, allowedRepos, activeCoords) {
		t.Fatal("expected PruneDisallowedNodeBlobCache to prune disallowed entry")
	}
	if PruneDisallowedNodeBlobCache(workDir, allowedRepos, activeCoords) {
		t.Fatal("expected second prune call to be a no-op")
	}
	if _, err := os.Stat(blobFilePath(workDir, hexDisallowed)); !os.IsNotExist(err) {
		t.Fatalf("expected disallowed blob to be removed, err=%v", err)
	}
	if _, err := os.Stat(blobExtractedDir(workDir, hexDisallowed)); !os.IsNotExist(err) {
		t.Fatalf("expected disallowed extracted dir to be removed, err=%v", err)
	}
	if _, err := os.Stat(blobFilePath(workDir, hexAllowed)); err != nil {
		t.Fatalf("expected allowed blob to be kept: %v", err)
	}
	if _, err := os.Stat(blobFilePath(workDir, hexActive)); err != nil {
		t.Fatalf("expected active-instance blob to be kept: %v", err)
	}
}

func TestNodeShouldRunPrewarmJobRespectsAllowedApps(t *testing.T) {
	node := &corev1.Node{Spec: corev1.NodeSpec{AllowedApps: []string{"minecraft-paper"}}}
	jobRust := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "auto-default-rust", Labels: map[string]string{corev1.LabelInstanceApp: "rust-dedicated"}},
		Spec:     corev1.PrewarmJobSpec{Artifacts: []corev1.ArtifactRef{{Repository: "rust-server", Package: "rust", Version: "latest"}}},
		Status:   corev1.PrewarmJobStatus{Nodes: []corev1.PrewarmNodeStatus{{NodeName: "n1", Phase: corev1.PrewarmJobPhasePending}}},
	}
	if nodeShouldRunPrewarmJob("n1", node, jobRust, map[string]string{"rust-server": "rust-dedicated"}, map[string]struct{}{}) {
		t.Fatal("expected disallowed PrewarmJob to be skipped by node agent")
	}
}

func seedServerArchiveWithFiles(t *testing.T, workDir, coord, hexDigest string, files map[string]string) {
	t.Helper()
	seedFakeArchiveArtifact(t, workDir, coord, hexDigest, files)
	meta := CachedArtifactMeta{
		Coordinate:    coord,
		Digest:        "sha256:" + hexDigest,
		HexDigest:     hexDigest,
		SizeBytes:     int64(len("archive-blob")),
		ArchiveFormat: registryv1.ArchiveFormatZip,
		TargetPath:    ".",
	}
	if err := writeCachedArtifactMeta(workDir, &meta); err != nil {
		t.Fatal(err)
	}
}

func TestOxideOverlayPreservesOnRestartReappliesOnUpgradeAndRestoresOnUninstall(t *testing.T) {
	workDir := t.TempDir()
	hostDir := filepath.Join(workDir, "instances", "rust-1")

	seedServerArchiveWithFiles(t, workDir, "rust-server.rust.v1", strings.Repeat("1", 64), map[string]string{
		"RustDedicated": "bin-v1",
		"RustDedicated_Data/Managed/Assembly-CSharp.dll": "vanilla-csharp-v1",
	})
	seedServerArchiveWithFiles(t, workDir, "rust-server.rust.v2", strings.Repeat("2", 64), map[string]string{
		"RustDedicated": "bin-v2",
		"RustDedicated_Data/Managed/Assembly-CSharp.dll": "vanilla-csharp-v2",
	})
	seedFakeArchiveArtifact(t, workDir, "rust-umod.oxide-rust.2.0.7912", strings.Repeat("a", 64), map[string]string{
		"RustDedicated_Data/Managed/Assembly-CSharp.dll": "oxide-patched-csharp",
		"RustDedicated_Data/Managed/Oxide.Core.dll":      "oxide-core",
	})
	seedFakeBlobFile(t, workDir, "rust-umod.gather-manager.2.2.78", strings.Repeat("b", 64), "GatherManager.cs", "// cs plugin")

	mat := &BlobWorkspaceMaterializer{WorkDir: workDir}
	in := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			ServerArtifact: &corev1.ArtifactRef{Repository: "rust-server", Package: "rust", Version: "v1", TargetPath: "."},
			Addons: []corev1.InstanceAddon{
				{Repository: "rust-umod", Package: "oxide-rust", Version: "2.0.7912", InstallMode: corev1.AddonInstallModeOxide, TargetDir: "oxide"},
				{Repository: "rust-umod", Package: "gather-manager", Version: "2.2.78", InstallMode: corev1.AddonInstallModeOxide, TargetDir: "oxide"},
			},
		},
	}
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("initial Materialize failed: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated_Data/Managed/Assembly-CSharp.dll"), "oxide-patched-csharp")
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated_Data/Managed/Oxide.Core.dll"), "oxide-core")
	assertWorkspaceFile(t, filepath.Join(hostDir, "oxide/plugins/GatherManager.cs"), "// cs plugin")

	// Restart without changes must keep Oxide's Assembly-CSharp.dll untouched.
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("restart Materialize failed: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated_Data/Managed/Assembly-CSharp.dll"), "oxide-patched-csharp")

	// Server upgrade must refresh RustDedicated while re-applying Oxide overlay.
	in.Spec.ServerArtifact.Version = "v2"
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("server upgrade Materialize failed: %v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated"), "bin-v2")
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated_Data/Managed/Assembly-CSharp.dll"), "oxide-patched-csharp")

	// Removing all Oxide addons must remove Oxide.Core.dll and restore vanilla v2 Assembly-CSharp.dll.
	in.Spec.Addons = nil
	if err := mat.Materialize(context.Background(), hostDir, in); err != nil {
		t.Fatalf("uninstall Oxide Materialize failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hostDir, "RustDedicated_Data/Managed/Oxide.Core.dll")); !os.IsNotExist(err) {
		t.Fatalf("expected Oxide.Core.dll removed on uninstall, err=%v", err)
	}
	assertWorkspaceFile(t, filepath.Join(hostDir, "RustDedicated_Data/Managed/Assembly-CSharp.dll"), "vanilla-csharp-v2")
}
