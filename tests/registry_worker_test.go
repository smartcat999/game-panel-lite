package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/controller"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type mockSteamCMD struct{}

func (mockSteamCMD) Run(_ context.Context, installDir string, args []string, hooks registryworker.SteamHooks) error {
	if hooks.OnAcquired != nil {
		hooks.OnAcquired()
	}
	for i := 0; i+2 < len(args); i++ {
		if args[i] == "+workshop_download_item" {
			appID, wsID := args[i+1], args[i+2]
			modRoot := filepath.Join(installDir, "steamapps", "workshop", "content", appID, wsID, "mods", "Hydrocraft")
			mapDir := filepath.Join(modRoot, "media", "maps", "BedfordFalls")
			if err := os.MkdirAll(mapDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(modRoot, "mod.info"), []byte("name=Hydrocraft\nid=Hydrocraft\n"), 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(mapDir, "objects.lua"), []byte("objects = {}\n"), 0o644)
		}
	}
	return nil
}

type workerTestEnv struct {
	cfg         *config.Config
	upstreamSrv *httptest.Server
	ctrlMgr     *controller.Manager
	adminCli    client.Interface
	workerCli   client.Interface
	worker      *registryworker.Worker
}

func setupWorkerTestEnv(t *testing.T) *workerTestEnv {
	t.Helper()
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	s3Srv, _ := newFakeS3Server(t)
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

	adminSess, _ := srv.Sessions().Issue(ctx, "User", "admin", "admin")
	adminCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: adminSess.Token})
	workerCli, err := client.NewRegistryWorkerFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewRegistryWorkerFromConfig: %v", err)
	}
	worker := registryworker.New(cfg, workerCli, registryworker.WithWorkerID("test-worker-1"), registryworker.WithSteamExecutor(mockSteamCMD{}))
	return &workerTestEnv{
		cfg: cfg, upstreamSrv: upstreamSrv, ctrlMgr: controller.NewManager(reg, nil, bus),
		adminCli: adminCli, workerCli: workerCli, worker: worker,
	}
}

func newMockUpstreamHub(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveMockUpstreamRoute(w, r, srv.URL)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func serveMockUpstreamRoute(w http.ResponseWriter, r *http.Request, baseURL string) {
	switch {
	case r.URL.Path == "/v3/projects/paper":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"versions": map[string][]string{"1.21": {"1.21.4"}}})
	case r.URL.Path == "/v3/projects/paper/versions/1.21.4/builds/latest":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        212,
			"downloads": map[string]any{"server:default": map[string]string{"name": "paper-1.21.4-212.jar", "url": baseURL + "/files/paper-1.21.4-212.jar"}},
		})
	case r.URL.Path == "/files/paper-1.21.4-212.jar":
		_, _ = w.Write([]byte("mock-paper-server-jar-bytes"))
	case r.URL.Path == "/project/essentialsx":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "essx", "slug": "essentialsx", "title": "EssentialsX", "project_type": "plugin"})
	case r.URL.Path == "/project/essentialsx/dependencies":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	case r.URL.Path == "/project/essentialsx/version":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "v-essx-1", "version_number": "2.21.0", "game_versions": []string{"1.21.4"}, "loaders": []string{"paper"},
			"files": []map[string]any{{"url": baseURL + "/files/EssentialsX-2.21.0.jar", "filename": "EssentialsX-2.21.0.jar", "primary": true, "size": 24}},
		}})
	case r.URL.Path == "/files/EssentialsX-2.21.0.jar":
		_, _ = w.Write([]byte("mock-essentialsx-jar-bin"))
	case strings.HasPrefix(r.URL.Path, "/IPublishedFileService/"):
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response": map[string]any{
				"total": 1,
				"publishedfiledetails": []map[string]any{{
					"publishedfileid": "2778991696", "result": 1, "consumer_app_id": 108600,
					"title": "Hydrocraft Pack", "time_updated": 1735689600, "file_url": "",
				}},
			},
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestRegistryWorkerReplicationAndZeroHostBoundary(t *testing.T) {
	ctx := context.Background()
	env := setupWorkerTestEnv(t)

	verifyWorkerRBACZeroHostBoundary(t, ctx, env)
	seedWorkerTestRepositoriesAndPolicies(t, ctx, env)

	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll policies: %v", err)
	}
	synced, err := env.worker.SyncOnce(ctx)
	if err != nil || synced != 3 {
		t.Fatalf("expected SyncOnce to process 3 artifacts, got %d (err=%v)", synced, err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll post-sync: %v", err)
	}
	verifySyncedArtifactsAndPZMetadata(t, ctx, env)
	verifyStaleSyncLeaseRecovery(t, ctx, env)
}

func verifyWorkerRBACZeroHostBoundary(t *testing.T, ctx context.Context, env *workerTestEnv) {
	t.Helper()
	if _, err := env.workerCli.CoreV1().Nodes().List(ctx, client.ListOptions{}); !client.IsForbidden(err) {
		t.Fatalf("expected registry-worker listing core Nodes to be forbidden, got %v", err)
	}
	if _, err := env.workerCli.IAMV1().Users().List(ctx, client.ListOptions{}); !client.IsForbidden(err) {
		t.Fatalf("expected registry-worker listing IAM Users to be forbidden, got %v", err)
	}
}

func seedWorkerTestRepositoriesAndPolicies(t *testing.T, ctx context.Context, env *workerTestEnv) {
	t.Helper()
	upstreams := []*registryv1.Upstream{
		{Metadata: metav1.ObjectMeta{Name: "up-paper"}, Provider: registryv1.UpstreamProviderPaperMC, Endpoint: env.upstreamSrv.URL},
		{Metadata: metav1.ObjectMeta{Name: "up-modrinth"}, Provider: registryv1.UpstreamProviderModrinth, Endpoint: env.upstreamSrv.URL, ModrinthLoader: "paper", ModrinthProjectType: "plugin"},
		{Metadata: metav1.ObjectMeta{Name: "up-pz-steam"}, Provider: registryv1.UpstreamProviderSteam, Endpoint: env.upstreamSrv.URL, SteamAppID: 108600, APIKey: "mock-key"},
	}
	for _, u := range upstreams {
		if _, err := env.adminCli.RegistryV1().Upstreams().Create(ctx, u); err != nil {
			t.Fatalf("Create upstream %s: %v", u.Metadata.Name, err)
		}
	}
	repos := []*registryv1.Repository{
		{Metadata: metav1.ObjectMeta{Name: "test-paper"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatServerPackage, UpstreamName: "up-paper"}},
		{Metadata: metav1.ObjectMeta{Name: "test-plugins"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatModrinthAddon, UpstreamName: "up-modrinth"}},
		{Metadata: metav1.ObjectMeta{Name: "test-pz-workshop"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatWorkshopMod, UpstreamName: "up-pz-steam"}},
	}
	for _, r := range repos {
		if _, err := env.adminCli.RegistryV1().Repositories().Create(ctx, r); err != nil {
			t.Fatalf("Create repo %s: %v", r.Metadata.Name, err)
		}
	}
	policies := []*registryv1.ReplicationPolicy{
		{Metadata: metav1.ObjectMeta{Name: "pol-paper"}, Repository: "test-paper", Enabled: true, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"paper"}},
		{Metadata: metav1.ObjectMeta{Name: "pol-essx"}, Repository: "test-plugins", Enabled: true, Mode: registryv1.ReplicationModeExplicitPackages, Packages: []string{"essentialsx"}, GameVersion: "1.21.4"},
		{Metadata: metav1.ObjectMeta{Name: "pol-pz"}, Repository: "test-pz-workshop", Enabled: true, Mode: registryv1.ReplicationModeTopPopular, TopN: 5},
	}
	for _, p := range policies {
		if _, err := env.adminCli.RegistryV1().ReplicationPolicies().Create(ctx, p); err != nil {
			t.Fatalf("Create policy %s: %v", p.Metadata.Name, err)
		}
	}
}

func verifySyncedArtifactsAndPZMetadata(t *testing.T, ctx context.Context, env *workerTestEnv) {
	t.Helper()
	paperArt, err := env.adminCli.RegistryV1().Artifacts().Get(ctx, "test-paper.paper.1.21.4")
	if err != nil || paperArt.Status.Phase != registryv1.ArtifactPhaseReady || paperArt.Status.Build != "212" {
		t.Fatalf("unexpected paper artifact: %+v, err=%v", paperArt, err)
	}
	essArt, err := env.adminCli.RegistryV1().Artifacts().Get(ctx, "test-plugins.essentialsx.2.21.0")
	if err != nil || essArt.Status.Phase != registryv1.ArtifactPhaseReady {
		t.Fatalf("unexpected essentialsx artifact: %+v, err=%v", essArt, err)
	}
	pzArt, err := env.adminCli.RegistryV1().Artifacts().Get(ctx, "test-pz-workshop.2778991696.1735689600")
	if err != nil || pzArt.Status.Phase != registryv1.ArtifactPhaseReady {
		t.Fatalf("unexpected pz workshop artifact: %+v, err=%v", pzArt, err)
	}
	if !slices.Contains(pzArt.Status.ModIDs, "Hydrocraft") || !slices.Contains(pzArt.Status.MapFolders, "BedfordFalls") {
		t.Fatalf("expected authoritative PZ ModIDs/MapFolders, got mods=%v maps=%v", pzArt.Status.ModIDs, pzArt.Status.MapFolders)
	}
	pzPkg, err := env.adminCli.RegistryV1().Packages().Get(ctx, "test-pz-workshop.2778991696")
	if err != nil || !slices.Contains(pzPkg.ModIDs, "Hydrocraft") || !slices.Contains(pzPkg.MapFolders, "BedfordFalls") {
		t.Fatalf("expected Package metadata propagation, got %+v, err=%v", pzPkg, err)
	}
	blobMeta, err := env.adminCli.RegistryV1().Blobs().GetMetadata(ctx, pzArt.Status.Descriptor.Digest)
	if err != nil || blobMeta.RefCount != 1 {
		t.Fatalf("expected Blob RefCount=1, got %+v, err=%v", blobMeta, err)
	}
}

func verifyStaleSyncLeaseRecovery(t *testing.T, ctx context.Context, env *workerTestEnv) {
	t.Helper()
	stuck, err := env.adminCli.RegistryV1().Artifacts().Create(ctx, &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{
			Name: "test-paper.paper.1.20.4",
			Annotations: map[string]string{
				registryv1.AnnotationSyncClaimedBy:  "crashed-worker",
				registryv1.AnnotationSyncLeaseUntil: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
			},
		},
		Spec: registryv1.ArtifactSpec{Repository: "test-paper", Package: "paper", Version: "1.20.4"},
	})
	if err != nil {
		t.Fatalf("Create stuck artifact: %v", err)
	}
	stuck.Status.Phase = registryv1.ArtifactPhaseSyncing
	if _, err := env.adminCli.RegistryV1().Artifacts().UpdateStatus(ctx, stuck); err != nil {
		t.Fatalf("UpdateStatus Syncing: %v", err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll stale lease: %v", err)
	}
	recovered, err := env.adminCli.RegistryV1().Artifacts().Get(ctx, stuck.Metadata.Name)
	if err != nil || recovered.Status.Phase != registryv1.ArtifactPhasePending {
		t.Fatalf("expected stuck artifact reset to Pending, got %+v, err=%v", recovered, err)
	}
	if recovered.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] != "" {
		t.Fatalf("expected sync claim annotation cleared, got %+v", recovered.Metadata.Annotations)
	}
}
