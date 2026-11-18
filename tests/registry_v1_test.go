package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/controller"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	usermodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/user"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type fakeS3Store struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeS3Server(t *testing.T) (*httptest.Server, *fakeS3Store) {
	t.Helper()
	store := &fakeS3Store{objects: make(map[string][]byte)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("X-Amz-Signature") != "" && r.Header.Get("Authorization") != "" {
			http.Error(w, "dual auth mechanism rejected", http.StatusBadRequest)
			return
		}
		serveFakeS3Object(w, r, store, strings.TrimPrefix(r.URL.Path, "/registry-test/"))
	}))
	t.Cleanup(srv.Close)
	return srv, store
}

func serveFakeS3Object(w http.ResponseWriter, r *http.Request, store *fakeS3Store, key string) {
	switch r.Method {
	case http.MethodPut:
		if copySrc := r.Header.Get("X-Amz-Copy-Source"); copySrc != "" {
			if decoded, err := url.PathUnescape(copySrc); err == nil {
				copySrc = decoded
			}
			srcKey := strings.TrimPrefix(copySrc, "/registry-test/")
			store.mu.Lock()
			srcData, ok := store.objects[srcKey]
			if ok {
				store.objects[key] = append([]byte(nil), srcData...)
			}
			store.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<CopyObjectResult><ETag>"etag-1"</ETag></CopyObjectResult>`))
			return
		}
		data, _ := io.ReadAll(r.Body)
		store.mu.Lock()
		store.objects[key] = data
		store.mu.Unlock()
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodHead, http.MethodGet:
		store.mu.Lock()
		data, ok := store.objects[key]
		store.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", `"etag-1"`)
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	case http.MethodDelete:
		store.mu.Lock()
		delete(store.objects, key)
		store.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type noopTestMQ struct{}

func (noopTestMQ) Publish(context.Context, string, mq.Event) error { return nil }
func (noopTestMQ) Subscribe(context.Context, string, string, mq.HandlerFunc) (mq.Subscription, error) {
	return nil, nil
}
func (noopTestMQ) Ping(context.Context) error { return nil }
func (noopTestMQ) Close() error               { return nil }

type registryTestEnv struct {
	s3Srv       *httptest.Server
	s3Store     *fakeS3Store
	modrinthSrv *httptest.Server
	ctrlMgr     *controller.Manager
	adminCli    client.Interface
	aliceCli    client.Interface
	nodeCli     client.Interface
}

func setupRegistryTestEnv(t *testing.T) *registryTestEnv {
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

	modrinthSrv := newMockModrinthServer(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	userHash, _ := usermodel.HashPassword("UserPass123!")
	_, _ = userdb.NewDB(reg.Pool()).Create(ctx, &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: "alice"},
		Spec:     iamv1.UserSpec{Email: "alice@example.com", DisplayName: "Alice", PasswordHash: userHash, State: iamv1.UserStateActive},
		Status:   iamv1.UserStatus{Phase: iamv1.UserStateActive, DefaultTenant: "alice"},
	})
	bus := noopTestMQ{}
	srv := apiserver.NewServer(nil, nil, reg, newMemTestCache(), runtime.NewDefaultRegistry(t.TempDir()), bus)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)

	adminSess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindUser, "admin", "admin")
	aliceSess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindUser, "alice", "alice")
	nodeSess, _ := srv.Sessions().Issue(ctx, iamv1.SubjectKindNode, "node-01", "")
	adminCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: adminSess.Token})
	aliceCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: aliceSess.Token})
	nodeCli, _ := client.New(&config.ClientConfig{ServerURL: httpSrv.URL, Token: nodeSess.Token})
	return &registryTestEnv{
		s3Srv: s3Srv, s3Store: s3Store, modrinthSrv: modrinthSrv,
		ctrlMgr:  controller.NewManager(reg, nil, bus),
		adminCli: adminCli, aliceCli: aliceCli, nodeCli: nodeCli,
	}
}

func newMockModrinthServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"total_hits": 1,
				"hits": []map[string]any{{
					"project_id": "AANobbMI", "slug": "lithium", "title": "Lithium",
					"description": "General-purpose optimization mod", "author": "jellysquid3",
					"downloads": 12500000, "date_modified": "2026-01-01T00:00:00Z",
				}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRegistryV1LifecycleAndRBAC(t *testing.T) {
	ctx := context.Background()
	env := setupRegistryTestEnv(t)
	verifyRegistryCatalogAndRBAC(t, ctx, env)
	verifyBlobUploadArtifactAndRefCount(t, ctx, env)
}

func verifyRegistryCatalogAndRBAC(t *testing.T, ctx context.Context, env *registryTestEnv) {
	t.Helper()
	up, err := env.adminCli.RegistryV1().Upstreams().Create(ctx, &registryv1.Upstream{
		Metadata:            metav1.ObjectMeta{Name: "modrinth-fabric"},
		Provider:            registryv1.UpstreamProviderModrinth,
		Endpoint:            env.modrinthSrv.URL,
		ModrinthLoader:      "fabric",
		ModrinthProjectType: "mod",
	})
	if err != nil {
		t.Fatalf("Create Upstream: %v", err)
	}
	proxyRepo, err := env.adminCli.RegistryV1().Repositories().Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "fabric-mods"},
		Spec: registryv1.RepositorySpec{
			Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatModrinthAddon, UpstreamName: up.Metadata.Name,
		},
	})
	if err != nil {
		t.Fatalf("Create Proxy Repository: %v", err)
	}
	if err := env.adminCli.RegistryV1().Upstreams().Delete(ctx, up.Metadata.Name); err == nil {
		t.Fatal("expected deleting referenced Upstream to fail")
	}
	if _, err := env.adminCli.RegistryV1().ReplicationPolicies().Create(ctx, &registryv1.ReplicationPolicy{
		Metadata: metav1.ObjectMeta{Name: "sync-lithium"}, Repository: proxyRepo.Metadata.Name,
		Mode: registryv1.ReplicationModeExplicitPackages, Packages: []string{"lithium"}, Enabled: true,
	}); err != nil {
		t.Fatalf("Create ReplicationPolicy: %v", err)
	}
	pkgList, err := env.aliceCli.RegistryV1().Packages().List(ctx, client.ListOptions{
		Query: map[string]string{"repository": "fabric-mods", "q": "lithium"},
	})
	if err != nil || len(pkgList.Items) != 1 || pkgList.Items[0].Metadata.Name != "fabric-mods.lithium" {
		t.Fatalf("unexpected proxy catalog items: %+v, err=%v", pkgList, err)
	}
	if _, err := env.aliceCli.RegistryV1().Repositories().Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "unauth"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeHosted, Format: registryv1.FormatGeneric},
	}); err == nil {
		t.Fatal("expected alice creating Repository to be forbidden")
	}
	if _, err := env.nodeCli.RegistryV1().Upstreams().List(ctx, client.ListOptions{}); err == nil {
		t.Fatal("expected node-01 listing Upstreams to be denied")
	}
}

func verifyBlobUploadArtifactAndRefCount(t *testing.T, ctx context.Context, env *registryTestEnv) {
	t.Helper()
	if _, err := env.adminCli.RegistryV1().Repositories().Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "hosted-paper"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeHosted, Format: registryv1.FormatServerPackage},
	}); err != nil {
		t.Fatalf("Create Hosted Repository: %v", err)
	}
	payload := []byte("fake-paper-1.21.4-server-bundle-payload")
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	uploadAndCommitTestBlob(t, ctx, env, digest, payload)

	if _, err := env.adminCli.RegistryV1().Artifacts().Create(ctx, &registryv1.Artifact{
		Spec: registryv1.ArtifactSpec{
			Repository: "hosted-paper", Package: "paper", Version: "1.21.4", ArchiveFormat: registryv1.ArchiveFormatTarGz,
			Descriptor: &registryv1.BlobDescriptor{Digest: digest, SizeBytes: int64(len(payload)), FileName: "paper.tar.gz"},
		},
	}); err != nil {
		t.Fatalf("Create Artifact: %v", err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}
	verifyArtifactStatusAndDownload(t, ctx, env, digest, payload)
	verifyArtifactReleaseAndBlobDelete(t, ctx, env, digest)
}

func uploadAndCommitTestBlob(t *testing.T, ctx context.Context, env *registryTestEnv, digest string, payload []byte) {
	t.Helper()
	upload, err := env.adminCli.RegistryV1().BlobUploads().Create(ctx, &registryv1.BlobUpload{
		Digest: digest, SizeBytes: int64(len(payload)), MediaType: "application/gzip",
	})
	if err != nil || upload.PresignedURL == "" {
		t.Fatalf("Create BlobUpload failed: %+v, err=%v", upload, err)
	}
	putReq, _ := http.NewRequestWithContext(ctx, http.MethodPut, upload.PresignedURL, bytes.NewReader(payload))
	putResp, err := env.s3Srv.Client().Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("direct S3 PUT failed: resp=%v err=%v", putResp, err)
	}
	_ = putResp.Body.Close()
	completed, err := env.adminCli.RegistryV1().BlobUploads().Complete(ctx, upload)
	if err != nil || completed.UploadState != registryv1.BlobUploadStateCompleted {
		t.Fatalf("Complete BlobUpload failed: %+v, err=%v", completed, err)
	}
}

func verifyArtifactStatusAndDownload(t *testing.T, ctx context.Context, env *registryTestEnv, digest string, payload []byte) {
	t.Helper()
	artAfter, err := env.nodeCli.RegistryV1().Artifacts().Get(ctx, "hosted-paper.paper.1.21.4")
	if err != nil || artAfter.Status.Phase != registryv1.ArtifactPhaseReady {
		t.Fatalf("unexpected Artifact after reconcile: %+v, err=%v", artAfter, err)
	}
	rvBeforeStatus := artAfter.Metadata.ResourceVersion
	now := time.Now().UTC()
	artAfter.Status.SyncedAt = &now
	artStatusUpdated, err := env.adminCli.RegistryV1().Artifacts().UpdateStatus(ctx, artAfter)
	if err != nil || artStatusUpdated.Metadata.ResourceVersion != rvBeforeStatus {
		t.Fatalf("UpdateStatus mutated RV (%v, err=%v)", artStatusUpdated, err)
	}
	rc, err := env.nodeCli.RegistryV1().Blobs().Download(ctx, digest)
	if err != nil {
		t.Fatalf("node Download Blob: %v", err)
	}
	downloaded, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(downloaded, payload) {
		t.Fatalf("downloaded blob mismatch: got %q", string(downloaded))
	}
	if err := env.adminCli.RegistryV1().Blobs().Delete(ctx, digest); err == nil {
		t.Fatal("expected deleting referenced Blob to fail")
	}
}

func verifyArtifactReleaseAndBlobDelete(t *testing.T, ctx context.Context, env *registryTestEnv, digest string) {
	t.Helper()
	if err := env.adminCli.RegistryV1().Artifacts().Delete(ctx, "hosted-paper.paper.1.21.4"); err != nil {
		t.Fatalf("Delete Artifact: %v", err)
	}
	if err := env.ctrlMgr.ReconcileAll(ctx); err != nil {
		t.Fatalf("ReconcileAll after delete: %v", err)
	}
	blobMeta, err := env.adminCli.RegistryV1().Blobs().GetMetadata(ctx, digest)
	if err != nil || blobMeta.RefCount != 0 {
		t.Fatalf("expected Blob RefCount=0, got %+v, err=%v", blobMeta, err)
	}
	if err := env.adminCli.RegistryV1().Blobs().Delete(ctx, digest); err != nil {
		t.Fatalf("Delete unreferenced Blob: %v", err)
	}
	env.s3Store.mu.Lock()
	_, stillInS3 := env.s3Store.objects[blobMeta.ObjectKey]
	env.s3Store.mu.Unlock()
	if stillInS3 {
		t.Fatalf("expected S3 object %q to be deleted", blobMeta.ObjectKey)
	}
}
