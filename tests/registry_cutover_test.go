package tests

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-cloud/pkg/agent"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/controller"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type cutoverTestEnv struct {
	reg         *repo.Registry
	s3Srv       *httptest.Server
	s3Store     *fakeS3Store
	httpSrv     *httptest.Server
	upstreamSrv *httptest.Server
	ctrlMgr     *controller.Manager
	adminCli    client.Interface
	nodeACli    client.Interface
	nodeBCli    client.Interface
	nodeAToken  string
	worker      *registryworker.Worker
}

func setupCutoverTestEnv(t *testing.T) *cutoverTestEnv {
	t.Helper()
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	s3Srv, s3Store := newFakeS3Server(t)
	s3Cfg := &config.S3Config{
		Endpoint: s3Srv.URL, Region: "us-east-1", Bucket: "registry-test",
		AccessKey: "minioadmin", SecretKey: "minioadmin", UsePathStyle: true,
	}
	s3Cfg.Default()
	prevS3 := s3.Default()
	s3.SetDefault(s3.NewClient(s3Cfg, s3Srv.Client()))
	t.Cleanup(func() { s3.SetDefault(prevS3) })

	upstreamSrv := newMockUpstreamHub(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	bus := noopTestMQ{}
	srv := apiserver.NewServer(nil, nil, reg, newMemTestCache(), runtime.NewDefaultRegistry(t.TempDir()), bus)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	cfg := config.New()
	cfg.Client.ServerURL = httpSrv.URL
	cfg.Registry.Endpoint = httpSrv.URL
	cfg.Registry.WorkerToken = config.Get().Registry.WorkerToken
	cfg.Registry.WorkerWorkDir = t.TempDir()
	cfg.Registry.SteamWebAPIKey = "mock-steam-key"

	adminSess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindUser, "admin", "admin")
	nodeASess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindNode, "node-a", "")
	nodeBSess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindNode, "node-b", "")
	adminCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: adminSess.Token})
	nodeACli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: nodeASess.Token})
	nodeBCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: nodeBSess.Token})
	workerCli, err := client.NewRegistryWorkerFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewRegistryWorkerFromConfig: %v", err)
	}
	worker := registryworker.New(cfg, workerCli, registryworker.WithWorkerID("cutover-worker-1"), registryworker.WithSteamExecutor(mockSteamCMD{}))
	return &cutoverTestEnv{
		reg: reg, s3Srv: s3Srv, s3Store: s3Store, httpSrv: httpSrv, upstreamSrv: upstreamSrv,
		ctrlMgr:  controller.NewManager(reg, nil, bus),
		adminCli: adminCli, nodeACli: nodeACli, nodeBCli: nodeBCli, nodeAToken: nodeASess.Token, worker: worker,
	}
}

func TestHarborProxyPullThroughPrewarmAndNodeRestriction(t *testing.T) {
	ctx := context.Background()
	env := setupCutoverTestEnv(t)

	// Point built-in papermc-official upstream to our local mock upstream hub.
	up, err := env.adminCli.RegistryV1().Upstreams().Get(ctx, "papermc-official")
	if err != nil {
		t.Fatalf("Get built-in papermc-official: %v", err)
	}
	up.Endpoint = env.upstreamSrv.URL
	if _, err := env.adminCli.RegistryV1().Upstreams().Update(ctx, up); err != nil {
		t.Fatalf("Update papermc-official endpoint: %v", err)
	}

	// Unknown version on Proxy repo must return 404 NotFound (Fail-Closed).
	if _, err := env.nodeACli.RegistryV1().Artifacts().Get(ctx, "mc-paper.paper.9.99.9"); !client.IsNotFound(err) {
		t.Fatalf("expected 404 NotFound for unknown proxy version, got %v", err)
	}

	// Valid version triggers Harbor pull-through -> creates Pending Artifact on demand.
	pulled, err := env.nodeACli.RegistryV1().Artifacts().Get(ctx, "mc-paper.paper.1.21.4")
	if err != nil || pulled.Status.Phase != registryv1.ArtifactPhasePending {
		t.Fatalf("expected pull-through Pending Artifact, got %+v, err=%v", pulled, err)
	}
	if synced, err := env.worker.SyncOnce(ctx); err != nil || synced != 1 {
		t.Fatalf("expected worker SyncOnce to process 1 pull-through artifact, got %d err=%v", synced, err)
	}
	readyArt, err := env.nodeACli.RegistryV1().Artifacts().Get(ctx, "mc-paper.paper.1.21.4")
	if err != nil || readyArt.Status.Phase != registryv1.ArtifactPhaseReady {
		t.Fatalf("expected Ready Artifact after SyncOnce, got %+v err=%v", readyArt, err)
	}

	verifyPrewarmJobAndNodeRestriction(t, ctx, env)
}

func verifyPrewarmJobAndNodeRestriction(t *testing.T, ctx context.Context, env *cutoverTestEnv) {
	t.Helper()
	registerReadyTestNode(t, ctx, env.reg, "node-a", map[string]string{"zone": "cn-east"})
	registerReadyTestNode(t, ctx, env.reg, "node-b", map[string]string{"zone": "cn-south"})

	job, err := env.adminCli.CoreV1().PrewarmJobs().Create(ctx, &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "warm-paper-east"},
		Spec: corev1.PrewarmJobSpec{
			NodeSelector: map[string]string{"zone": "cn-east"},
			Artifacts:    []corev1.ArtifactRef{{Repository: "mc-paper", Package: "paper", Version: "1.21.4"}},
		},
	})
	if err != nil {
		t.Fatalf("Create PrewarmJob: %v", err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll PrewarmJob: %v", err)
	}

	// node-b was NOT targeted -> must be denied by NodeRestriction.
	if _, err := env.nodeBCli.CoreV1().PrewarmJobs().UpdateStatus(ctx, &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: job.Metadata.Name},
		Status:   corev1.PrewarmJobStatus{Nodes: []corev1.PrewarmNodeStatus{{NodeName: "node-b", Phase: corev1.PrewarmJobPhaseRunning}}},
	}); !client.IsForbidden(err) {
		t.Fatalf("expected untargeted node-b UpdateStatus to be forbidden, got %v", err)
	}
	// node-a spoofing node-b entry -> must be denied by NodeRestriction.
	if _, err := env.nodeACli.CoreV1().PrewarmJobs().UpdateStatus(ctx, &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: job.Metadata.Name},
		Status:   corev1.PrewarmJobStatus{Nodes: []corev1.PrewarmNodeStatus{{NodeName: "node-b", Phase: corev1.PrewarmJobPhaseRunning}}},
	}); !client.IsForbidden(err) {
		t.Fatalf("expected node-a spoofing node-b prewarm status to be forbidden, got %v", err)
	}

	// Execute Agent.RunPrewarmOnce on node-a and verify .blob-cache + PrewarmJob Succeeded + Node CachedArtifacts.
	workDir := t.TempDir()
	ag := agent.NewAgent(&agent.Config{
		NodeName:     "node-a",
		APIServerURL: env.httpSrv.URL,
		Token:        env.nodeAToken,
		WorkDir:      workDir,
	}, runtime.NewDefaultRegistry(workDir))
	if err := ag.RunPrewarmOnce(ctx); err != nil {
		t.Fatalf("RunPrewarmOnce: %v", err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll after RunPrewarmOnce: %v", err)
	}
	finalJob, err := env.adminCli.CoreV1().PrewarmJobs().Get(ctx, job.Metadata.Name)
	if err != nil || finalJob.Status.Phase != corev1.PrewarmJobPhaseSucceeded || finalJob.Status.ReadyNodes != 1 {
		t.Fatalf("expected PrewarmJob Succeeded with 1 ready node, got %+v err=%v", finalJob, err)
	}
	nodeObj, err := env.adminCli.CoreV1().Nodes().Get(ctx, "node-a")
	if err != nil || !slices.Contains(nodeObj.Status.CachedArtifacts, "mc-paper.paper.1.21.4") || len(nodeObj.Status.CachedBlobs) == 0 {
		t.Fatalf("expected node-a heartbeat to report CachedArtifacts & CachedBlobs, got %+v err=%v", nodeObj.Status, err)
	}
}

func registerReadyTestNode(t *testing.T, ctx context.Context, reg *repo.Registry, name string, labels map[string]string) {
	t.Helper()
	ndb := nodedb.NewDB(reg.Pool())
	created, err := ndb.Create(ctx, &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:     corev1.NodeSpec{Unschedulable: false},
	})
	if err != nil {
		t.Fatalf("Create Node %s: %v", name, err)
	}
	created.Status.Conditions = []metav1.Condition{{Type: metav1.ConditionTypeReady, Status: metav1.ConditionTrue}}
	if _, err := ndb.UpdateStatus(ctx, created); err != nil {
		t.Fatalf("UpdateStatus Node %s: %v", name, err)
	}
}

func TestHardlinkInjectorsUpgradeRollbackAndFailClosed(t *testing.T) {
	ctx := context.Background()
	env := setupCutoverTestEnv(t)

	if _, err := env.adminCli.RegistryV1().Repositories().Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "hosted-addons"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeHosted, Format: registryv1.FormatGeneric},
	}); err != nil {
		t.Fatalf("Create hosted-addons repo: %v", err)
	}

	seedInjectorTestArtifacts(t, ctx, env)

	workDir := t.TempDir()
	mat := &agent.BlobWorkspaceMaterializer{
		WorkDir:  workDir,
		ClientFn: func() (client.Interface, error) { return env.nodeACli, nil },
	}

	verifyFailClosedMaterializerGate(t, ctx, env, mat)
	verifyFlatDirUpgradeAndRollback(t, ctx, mat)
	verifyTModLatestAndDSTInjectors(t, ctx, mat)
	verifyPZWorkshopTreeInjector(t, ctx, mat)
}

func seedInjectorTestArtifacts(t *testing.T, ctx context.Context, env *cutoverTestEnv) {
	t.Helper()
	publishHostedTestArtifact(t, ctx, env, "essx", "2.20.0", "EssentialsX-2.20.0.jar", registryv1.ArchiveFormatJar, []byte("essx-v2200"), nil, nil)
	publishHostedTestArtifact(t, ctx, env, "essx", "2.21.0", "EssentialsX-2.21.0.jar", registryv1.ArchiveFormatJar, []byte("essx-v2210"), nil, nil)

	tmodTar := buildTestTarGz(t, map[string]string{
		"2026.8/CalamityMod.tmod": "calamity-old-2026-8",
		"2026.9/CalamityMod.tmod": "calamity-new-2026-9",
	})
	publishHostedTestArtifact(t, ctx, env, "2824688072", "100", "calamity.tar.gz", registryv1.ArchiveFormatTarGz, tmodTar, nil, nil)

	dstTar := buildTestTarGz(t, map[string]string{
		"modinfo.lua": "name = 'Global Positions'\n",
		"modmain.lua": "GLOBAL.setmetatable(env,{})\n",
	})
	publishHostedTestArtifact(t, ctx, env, "378160973", "200", "dst-mod.tar.gz", registryv1.ArchiveFormatTarGz, dstTar, nil, nil)

	pzTar := buildTestTarGz(t, map[string]string{
		"mods/Hydrocraft/mod.info":                   "name=Hydrocraft\nid=Hydrocraft\n",
		"mods/Hydrocraft/media/maps/Bedford/map.bin": "bedford-map-data",
	})
	publishHostedTestArtifact(t, ctx, env, "2778991696", "300", "pz-mod.tar.gz", registryv1.ArchiveFormatTarGz, pzTar, []string{"Hydrocraft"}, []string{"Bedford"})
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll seeded artifacts: %v", err)
	}
}

func publishHostedTestArtifact(
	t *testing.T,
	ctx context.Context,
	env *cutoverTestEnv,
	pkg, ver, fileName, archFmt string,
	payload []byte,
	modIDs, mapFolders []string,
) {
	t.Helper()
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	upload, err := env.adminCli.RegistryV1().BlobUploads().Create(ctx, &registryv1.BlobUpload{
		Digest: digest, SizeBytes: int64(len(payload)), MediaType: "application/octet-stream",
	})
	if err != nil {
		t.Fatalf("Create BlobUpload %s:%s: %v", pkg, ver, err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, upload.PresignedURL, bytes.NewReader(payload))
	resp, err := env.s3Srv.Client().Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT test blob %s:%s: resp=%v err=%v", pkg, ver, resp, err)
	}
	_ = resp.Body.Close()
	if _, err := env.adminCli.RegistryV1().BlobUploads().Complete(ctx, upload); err != nil {
		t.Fatalf("Complete BlobUpload %s:%s: %v", pkg, ver, err)
	}
	created, err := env.adminCli.RegistryV1().Artifacts().Create(ctx, &registryv1.Artifact{
		Spec: registryv1.ArtifactSpec{
			Repository: "hosted-addons", Package: pkg, Version: ver, ArchiveFormat: archFmt,
			Descriptor: &registryv1.BlobDescriptor{Digest: digest, SizeBytes: int64(len(payload)), FileName: fileName, ArchiveFormat: archFmt},
		},
	})
	if err != nil {
		t.Fatalf("Create Artifact %s:%s: %v", pkg, ver, err)
	}
	if len(modIDs) > 0 || len(mapFolders) > 0 {
		created.Status.Phase = registryv1.ArtifactPhaseReady
		created.Status.Descriptor = created.Spec.Descriptor
		created.Status.ModIDs = modIDs
		created.Status.MapFolders = mapFolders
		if _, err := env.adminCli.RegistryV1().Artifacts().UpdateStatus(ctx, created); err != nil {
			t.Fatalf("UpdateStatus Artifact %s:%s: %v", pkg, ver, err)
		}
	}
}

func buildTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		raw := []byte(body)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(raw))}); err != nil {
			t.Fatalf("tar header %s: %v", name, err)
		}
		if _, err := tw.Write(raw); err != nil {
			t.Fatalf("tar write %s: %v", name, err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func verifyFailClosedMaterializerGate(t *testing.T, ctx context.Context, env *cutoverTestEnv, mat *agent.BlobWorkspaceMaterializer) {
	t.Helper()
	// 1) Missing 404 version must strictly return error (never boot or fallback).
	missingInst := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{{Repository: "hosted-addons", Package: "essx", Version: "9.9.9"}},
	}}
	if _, err := mat.Prepare(ctx, missingInst); err == nil {
		t.Fatal("expected Prepare to fail-closed on missing 404 artifact")
	}

	// 2) Pending artifact returns syncMsg != "" and nil error (non-failing Starting wait).
	pendingArt, err := env.adminCli.RegistryV1().Artifacts().Create(ctx, &registryv1.Artifact{
		Spec: registryv1.ArtifactSpec{Repository: "hosted-addons", Package: "pending-mod", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Create pending-mod: %v", err)
	}
	pendingInst := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{{Repository: "hosted-addons", Package: "pending-mod", Version: "1.0.0"}},
	}}
	syncMsg, err := mat.Prepare(ctx, pendingInst)
	if err != nil || syncMsg == "" {
		t.Fatalf("expected non-empty syncMsg and nil err for Pending artifact, got msg=%q err=%v", syncMsg, err)
	}

	// 3) Failed artifact strictly returns error (Fail-Closed).
	pendingArt.Status.Phase = registryv1.ArtifactPhaseFailed
	pendingArt.Status.Message = "upstream checksum mismatch"
	if _, err := env.adminCli.RegistryV1().Artifacts().UpdateStatus(ctx, pendingArt); err != nil {
		t.Fatalf("UpdateStatus Failed: %v", err)
	}
	if _, err := mat.Prepare(ctx, pendingInst); err == nil || !strings.Contains(err.Error(), "upstream checksum mismatch") {
		t.Fatalf("expected Fail-Closed error on Failed artifact, got %v", err)
	}
}

func verifyFlatDirUpgradeAndRollback(t *testing.T, ctx context.Context, mat *agent.BlobWorkspaceMaterializer) {
	t.Helper()
	hostDir := t.TempDir()
	userSave := filepath.Join(hostDir, "world", "level.dat")
	_ = os.MkdirAll(filepath.Dir(userSave), 0o755)
	_ = os.WriteFile(userSave, []byte("precious-player-world"), 0o644)

	instV1 := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{{Repository: "hosted-addons", Package: "essx", Version: "2.20.0", InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: "plugins"}},
	}}
	if msg, err := mat.Prepare(ctx, instV1); err != nil || msg != "" {
		t.Fatalf("Prepare v1: msg=%q err=%v", msg, err)
	}
	if err := mat.Materialize(ctx, hostDir, instV1); err != nil {
		t.Fatalf("Materialize v1: %v", err)
	}
	v1Jar := filepath.Join(hostDir, "plugins", "essx-2.20.0.jar")
	_, blobs := agent.CollectNodeBlobCacheInventory(mat.WorkDir)
	blobPath := filepath.Join(mat.WorkDir, ".blob-cache", "sha256", strings.TrimPrefix(blobs[0], "sha256:"))
	infoJar, err1 := os.Stat(v1Jar)
	infoBlob, err2 := os.Stat(blobPath)
	if err1 != nil || err2 != nil || os.SameFile(infoJar, infoBlob) {
		t.Fatalf("expected plugins/essx-2.20.0.jar to have isolated inode from .blob-cache (sameFile=%v err1=%v err2=%v)", os.SameFile(infoJar, infoBlob), err1, err2)
	}
	if infoBlob.Mode().Perm() != 0o444 {
		t.Fatalf("expected .blob-cache file permission 0444, got %04o", infoBlob.Mode().Perm())
	}
	if err := os.WriteFile(v1Jar, []byte("tampered!!"), 0o644); err != nil {
		t.Fatalf("mutate instance workspace jar: %v", err)
	}
	if blobRaw, err := os.ReadFile(blobPath); err != nil || string(blobRaw) != "essx-v2200" {
		t.Fatalf("expected .blob-cache intact after workspace write, got %q err=%v", string(blobRaw), err)
	}

	// Upgrade to 2.21.0 -> 2.20.0 unlinked, 2.21.0 linked, world/level.dat untouched!
	instV2 := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{{Repository: "hosted-addons", Package: "essx", Version: "2.21.0", InstallMode: corev1.AddonInstallModeFlatDir, TargetDir: "plugins"}},
	}}
	if msg, err := mat.Prepare(ctx, instV2); err != nil || msg != "" {
		t.Fatalf("Prepare v2: msg=%q err=%v", msg, err)
	}
	if err := mat.Materialize(ctx, hostDir, instV2); err != nil {
		t.Fatalf("Materialize v2: %v", err)
	}
	if _, err := os.Stat(v1Jar); !os.IsNotExist(err) {
		t.Fatalf("expected old v1 jar unlinked on upgrade, got err=%v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(hostDir, "plugins", "essx-2.21.0.jar")); err != nil || string(raw) != "essx-v2210" {
		t.Fatalf("expected upgraded v2 jar, got %q err=%v", string(raw), err)
	}
	if saveRaw, err := os.ReadFile(userSave); err != nil || string(saveRaw) != "precious-player-world" {
		t.Fatalf("expected user save untouched across addon upgrade, got %q err=%v", string(saveRaw), err)
	}
}

func verifyTModLatestAndDSTInjectors(t *testing.T, ctx context.Context, mat *agent.BlobWorkspaceMaterializer) {
	t.Helper()
	hostDir := t.TempDir()
	inst := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{
			{Repository: "hosted-addons", Package: "2824688072", Version: "100", InstallMode: corev1.AddonInstallModeTModLatest, TargetDir: "Mods"},
			{Repository: "hosted-addons", Package: "378160973", Version: "200", InstallMode: corev1.AddonInstallModeDSTLocalMods, TargetDir: "mods"},
		},
	}}
	if msg, err := mat.Prepare(ctx, inst); err != nil || msg != "" {
		t.Fatalf("Prepare tmod+dst: msg=%q err=%v", msg, err)
	}
	if err := mat.Materialize(ctx, hostDir, inst); err != nil {
		t.Fatalf("Materialize tmod+dst: %v", err)
	}
	tmodRaw, err := os.ReadFile(filepath.Join(hostDir, "Mods", "CalamityMod.tmod"))
	if err != nil || string(tmodRaw) != "calamity-new-2026-9" {
		t.Fatalf("expected latest 2026.9 CalamityMod.tmod selected, got %q err=%v", string(tmodRaw), err)
	}
	enabledRaw, _ := os.ReadFile(filepath.Join(hostDir, "Mods", "enabled.json"))
	if !strings.Contains(string(enabledRaw), `"CalamityMod"`) {
		t.Fatalf("expected CalamityMod in enabled.json, got %s", string(enabledRaw))
	}
	masterLua, _ := os.ReadFile(filepath.Join(hostDir, "DoNotStarveTogether", "Cluster_1", "Master", "modoverrides.lua"))
	if !strings.Contains(string(masterLua), `["workshop-378160973"] = { enabled = true }`) {
		t.Fatalf("unexpected Master modoverrides.lua:\n%s", string(masterLua))
	}
	if _, err := os.Stat(filepath.Join(hostDir, "mods", "workshop-378160973", "modinfo.lua")); err != nil {
		t.Fatalf("expected DST workshop dir hard-linked: %v", err)
	}

	// Remove all addons -> clean reset of enabled.json and modoverrides.lua.
	if err := mat.Materialize(ctx, hostDir, &corev1.Instance{}); err != nil {
		t.Fatalf("Materialize empty cleanup: %v", err)
	}
	enabledAfter, _ := os.ReadFile(filepath.Join(hostDir, "Mods", "enabled.json"))
	masterAfter, _ := os.ReadFile(filepath.Join(hostDir, "DoNotStarveTogether", "Cluster_1", "Master", "modoverrides.lua"))
	if strings.TrimSpace(string(enabledAfter)) != "[]" || strings.Contains(string(masterAfter), "workshop-378160973") {
		t.Fatalf("expected clean removal reset, got enabled=%s master=%s", string(enabledAfter), string(masterAfter))
	}
}

func verifyPZWorkshopTreeInjector(t *testing.T, ctx context.Context, mat *agent.BlobWorkspaceMaterializer) {
	t.Helper()
	hostDir := t.TempDir()
	iniPath := filepath.Join(hostDir, "Zomboid", "Server", "servertest.ini")
	_ = os.MkdirAll(filepath.Dir(iniPath), 0o755)
	_ = os.WriteFile(iniPath, []byte("PVP=false\nMap=Muldraugh, KY\nWorkshopItems=\nMods=\n"), 0o644)

	inst := &corev1.Instance{Spec: corev1.InstanceSpec{
		Addons: []corev1.InstanceAddon{{
			Repository: "hosted-addons", Package: "2778991696", Version: "300",
			InstallMode: corev1.AddonInstallModeWorkshopTree, TargetDir: "steamapps/workshop/content/108600",
		}},
	}}
	if msg, err := mat.Prepare(ctx, inst); err != nil || msg != "" {
		t.Fatalf("Prepare pz: msg=%q err=%v", msg, err)
	}
	if err := mat.Materialize(ctx, hostDir, inst); err != nil {
		t.Fatalf("Materialize pz: %v", err)
	}
	iniRaw, _ := os.ReadFile(iniPath)
	iniStr := string(iniRaw)
	if !strings.Contains(iniStr, "WorkshopItems=2778991696") || !strings.Contains(iniStr, "Mods=Hydrocraft") || !strings.Contains(iniStr, "Map=Bedford;Muldraugh, KY") {
		t.Fatalf("unexpected patched PZ servertest.ini:\n%s", iniStr)
	}
	if _, err := os.Stat(filepath.Join(hostDir, "steamapps", "workshop", "appworkshop_108600.acf")); err != nil {
		t.Fatalf("expected appworkshop_108600.acf written: %v", err)
	}

	// Remove PZ mod -> restores Map=Muldraugh, KY and clears WorkshopItems=/Mods=.
	if err := mat.Materialize(ctx, hostDir, &corev1.Instance{}); err != nil {
		t.Fatalf("Materialize empty pz cleanup: %v", err)
	}
	iniCleaned, _ := os.ReadFile(iniPath)
	if !strings.Contains(string(iniCleaned), "Map=Muldraugh, KY\n") || !strings.Contains(string(iniCleaned), "WorkshopItems=\n") {
		t.Fatalf("expected base Map restored on PZ mod removal, got:\n%s", string(iniCleaned))
	}
}
