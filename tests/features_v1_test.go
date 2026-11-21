package tests

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authorization"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	appkapis "github.com/smartcat999/game-panel-cloud/pkg/kapis/core/v1/app"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type memTestCache struct {
	mu   sync.Mutex
	data map[string]string
}

func newMemTestCache() *memTestCache {
	return &memTestCache{data: make(map[string]string)}
}

func (m *memTestCache) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}

func (m *memTestCache) Set(_ context.Context, key, val string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return nil
}

func (m *memTestCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memTestCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *memTestCache) Ping(_ context.Context) error { return nil }
func (m *memTestCache) Close() error                 { return nil }

func buildFeatureTestTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(body))}); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write tar body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func TestLocalBackupArchiveImportLifecycle(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	drivers := runtime.NewDefaultRegistry(t.TempDir())
	archiveBytes := buildFeatureTestTarGz(t, map[string]string{"world/level.dat": "mc-world-data"})
	archiveSize := int64(len(archiveBytes))
	memS3 := newMemS3Server(t)

	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	polList, err := policydb.NewDB(reg.Pool()).List(ctx, db.ListOptions{})
	if err != nil || len(polList.Items) != 6 {
		t.Fatalf("expected 6 built-in ReplicationPolicies, got %+v err=%v", polList, err)
	}
	for _, pol := range polList.Items {
		if pol.Enabled {
			t.Fatalf("expected built-in ReplicationPolicy %s to default to Enabled=false", pol.Metadata.Name)
		}
	}
	if _, err := tenantdb.NewDB(reg.Pool()).Create(ctx, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "default"},
		Spec: corev1.TenantSpec{
			DisplayName: "Default", Type: corev1.TenantTypeOrganization, Owner: "admin",
			Quota: corev1.ResourceList{
				corev1.ResourceCPU: "8000m", corev1.ResourceMemory: "16Gi", corev1.ResourceStorage: "100Gi",
				corev1.ResourceInstances: "5", corev1.ResourceBackups: "5", corev1.ResourceBackupStorage: "10Gi",
			},
		},
	}); err != nil {
		t.Fatalf("Create default Tenant: %v", err)
	}

	inst := fixtures.CreateActive(t, ctx, reg, drivers, "default", "node-01", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "mc-import-01"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker", Image: "itzg/minecraft-server:java21", State: corev1.InstanceStateRunning,
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceCPU: "2000m", corev1.ResourceMemory: "4Gi", corev1.ResourceStorage: "20Gi",
			}},
		},
	})

	s3Cfg := &config.S3Config{
		Endpoint: memS3.srv.URL, Region: "us-east-1", Bucket: "backups",
		AccessKey: "ak", SecretKey: "sk", UsePathStyle: true, PresignTTL: 10 * time.Minute,
	}
	config.Get().S3 = *s3Cfg
	var nowMu sync.Mutex
	curTime := time.Now().UTC()
	nowFn := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return curTime
	}
	advanceClock := func(d time.Duration) {
		nowMu.Lock()
		curTime = curTime.Add(d)
		nowMu.Unlock()
	}
	instOp := instancemodel.NewOperator(reg, drivers, nil,
		instancemodel.WithS3Client(s3.NewClient(s3Cfg, memS3.srv.Client())),
		instancemodel.WithClock(nowFn),
	)

	uploadRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, Path: "backups/import-save.tar.gz", SizeBytes: archiveSize,
	})
	if err != nil || !strings.Contains(uploadRes.DirectURL, "X-Amz-Signature=") {
		t.Fatalf("upload_backup failed: res=%+v err=%v", uploadRes, err)
	}
	if st := putPresignedURL(t, memS3.srv.Client(), uploadRes.DirectURL, archiveBytes); st != http.StatusOK {
		t.Fatalf("PUT import-save staging failed: %d", st)
	}

	completeRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionCompleteUploadBackup, Path: "backups/import-save.tar.gz",
	})
	if err != nil || completeRes.Phase != corev1.BackupPhaseUnknown {
		t.Fatalf("complete_upload_backup expected Unknown phase, got res=%+v err=%v", completeRes, err)
	}
	curInst, err := instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get instance before ReconcileBackups: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups: %v", err)
	}

	updatedInst, err := instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil || len(updatedInst.Status.Backups) != 1 {
		t.Fatalf("expected 1 backup record in Status.Backups, got %+v err=%v", updatedInst.Status.Backups, err)
	}
	gotBak := updatedInst.Status.Backups[0]
	if !gotBak.Imported || gotBak.Phase != corev1.BackupPhaseCompleted || gotBak.SizeBytes != archiveSize ||
		gotBak.Inspection == nil || gotBak.Inspection.Phase != corev1.BackupInspectPhaseCompleted {
		t.Fatalf("expected imported Completed backup + Completed inspection, got %+v", gotBak)
	}
	acct, err := quotadb.NewDB(reg.Pool()).GetAccount(ctx, updatedInst.Status.ResourceControl.TenantUID)
	if err != nil || acct.Allocated.BackupsCount != 1 || acct.Allocated.BackupStorageBytes != archiveSize {
		t.Fatalf("expected backup quota allocated (count=1, bytes=%d), got %+v err=%v", archiveSize, acct.Allocated, err)
	}
	idemComplete, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionCompleteUploadBackup, OperationID: uploadRes.OperationID, Path: uploadRes.Path,
	})
	if err != nil || idemComplete.Phase != corev1.BackupPhaseCompleted {
		t.Fatalf("idempotent complete_upload_backup after Completed failed: res=%+v err=%v", idemComplete, err)
	}
	// Controller-first race order: ReconcileBackups settles Uploading -> Completed before client calls complete_upload_backup.
	raceUpload, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, Path: "backups/import-controller-first.tar.gz", SizeBytes: archiveSize,
	})
	if err != nil {
		t.Fatalf("upload_backup controller-first race: %v", err)
	}
	if st := putPresignedURL(t, memS3.srv.Client(), raceUpload.DirectURL, archiveBytes); st != http.StatusOK {
		t.Fatalf("PUT controller-first staging failed: %d", st)
	}
	if curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("Get before controller-first ReconcileBackups: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups controller-first: %v", err)
	}
	raceComplete, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionCompleteUploadBackup, OperationID: raceUpload.OperationID, Path: raceUpload.Path,
	})
	if err != nil || raceComplete.Phase != corev1.BackupPhaseCompleted {
		t.Fatalf("complete_upload_backup after controller-first settlement failed: res=%+v err=%v", raceComplete, err)
	}
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionDeleteBackup, Path: raceUpload.Path,
	}); err != nil {
		t.Fatalf("delete_backup controller-first: %v", err)
	}
	advanceClock(15 * time.Minute)
	if curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("Get after delete controller-first: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups delete controller-first: %v", err)
	}

	// Oversized S3 upload must fail closed, delete the oversized S3 object, and release quota once window expires.
	overRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, Path: "backups/oversized-save.tar.gz", SizeBytes: 64,
	})
	if err != nil {
		t.Fatalf("upload_backup oversized: %v", err)
	}
	if st := putPresignedURL(t, memS3.srv.Client(), overRes.DirectURL, bytes.Repeat([]byte("x"), 2048)); st != http.StatusOK {
		t.Fatalf("PUT oversized staging failed: %d", st)
	}
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionCompleteUploadBackup, Path: overRes.Path,
	}); err != nil {
		t.Fatalf("complete_upload_backup oversized: %v", err)
	}
	advanceClock(15 * time.Minute)
	if curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("Get before oversized ReconcileBackups: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups oversized: %v", err)
	}

	// Interrupted upload (never uploaded to S3) deleted via API must transition to Deleted and release quota after window expires.
	abandRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, Path: "backups/abandoned-save.tar.gz", SizeBytes: 128,
	})
	if err != nil {
		t.Fatalf("upload_backup abandoned: %v", err)
	}
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionDeleteBackup, Path: abandRes.Path,
	}); err != nil {
		t.Fatalf("delete_backup abandoned: %v", err)
	}
	advanceClock(15 * time.Minute)
	if curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name); err != nil {
		t.Fatalf("Get before abandoned ReconcileBackups: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups abandoned: %v", err)
	}

	afterInst, err := instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil || len(afterInst.Status.Backups) != 1 || afterInst.Status.Backups[0].Path != gotBak.Path {
		t.Fatalf("expected only valid backup remaining after oversized/abandoned cleanup, got %+v err=%v", afterInst.Status.Backups, err)
	}
	acctAfter, err := quotadb.NewDB(reg.Pool()).GetAccount(ctx, updatedInst.Status.ResourceControl.TenantUID)
	if err != nil || acctAfter.Allocated.BackupsCount != 1 || acctAfter.Allocated.BackupStorageBytes != archiveSize {
		t.Fatalf("expected backup quota released back to valid import (count=1, bytes=%d), got %+v err=%v", archiveSize, acctAfter.Allocated, err)
	}
	deletedJoined := memS3.deletedJoined()
	if !strings.Contains(deletedJoined, "-oversized-save.tar.gz") || !strings.Contains(deletedJoined, "-abandoned-save.tar.gz") {
		t.Fatalf("expected S3 DELETE for both oversized and abandoned uploads, got %v", deletedJoined)
	}
}

func TestSessionDeviceDeduplicationCLIFilterAndRevocation(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	polList, err := policydb.NewDB(reg.Pool()).List(ctx, db.ListOptions{})
	if err != nil || len(polList.Items) != 6 {
		t.Fatalf("expected 6 built-in ReplicationPolicies, got %+v err=%v", polList, err)
	}
	for _, pol := range polList.Items {
		if pol.Enabled {
			t.Fatalf("expected built-in ReplicationPolicy %s to default to Enabled=false", pol.Metadata.Name)
		}
	}
	sessOp := sessionmodel.NewOperator(reg, newMemTestCache(), &config.AuthConfig{
		JWTSecret: "test-jwt-secret",
		TokenTTL:  time.Hour,
	})

	// Login 1 & Login 2 from the exact same browser + IP must deduplicate (evict Login 1).
	s1, err := sessOp.Create(ctx, &iamv1.Session{
		Metadata:   metav1.ObjectMeta{Annotations: map[string]string{"iam.gamepanel.io/user-agent": "Mozilla/5.0 Chrome/131.0", "iam.gamepanel.io/client-ip": "10.0.0.8"}},
		Identifier: "admin", Password: "P@88w0rd",
	})
	if err != nil {
		t.Fatalf("Create s1: %v", err)
	}
	s2, err := sessOp.Create(ctx, &iamv1.Session{
		Metadata:   metav1.ObjectMeta{Annotations: map[string]string{"iam.gamepanel.io/user-agent": "Mozilla/5.0 Chrome/131.0", "iam.gamepanel.io/client-ip": "10.0.0.8"}},
		Identifier: "admin", Password: "P@88w0rd",
	})
	if err != nil {
		t.Fatalf("Create s2: %v", err)
	}
	if _, err := sessOp.Verify(ctx, s1.Token); err == nil {
		t.Fatal("expected s1 token from same device to be evicted after s2 login")
	}
	if _, err := sessOp.Get(ctx, s1.Metadata.Name); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected s1 Get to return ErrNotFound, got %v", err)
	}

	// Login from a second browser device and a CLI curl client.
	s3Browser, err := sessOp.Create(ctx, &iamv1.Session{
		Metadata:   metav1.ObjectMeta{Annotations: map[string]string{"iam.gamepanel.io/user-agent": "Mozilla/5.0 Safari/18.0", "iam.gamepanel.io/client-ip": "10.0.0.9"}},
		Identifier: "admin", Password: "P@88w0rd",
	})
	if err != nil {
		t.Fatalf("Create s3Browser: %v", err)
	}
	if _, err := sessOp.Create(ctx, &iamv1.Session{
		Metadata:   metav1.ObjectMeta{Annotations: map[string]string{"iam.gamepanel.io/user-agent": "curl/8.5.0", "iam.gamepanel.io/client-ip": "10.0.0.8"}},
		Identifier: "admin", Password: "P@88w0rd",
	}); err != nil {
		t.Fatalf("Create curl session: %v", err)
	}

	list, err := sessOp.ListByUser(ctx, "admin")
	if err != nil || len(list.Items) != 2 {
		t.Fatalf("expected ListByUser to return 2 browser sessions and filter curl, got %d err=%v", len(list.Items), err)
	}

	// Revoke other devices from s3Browser, then revoke s3Browser itself.
	if err := sessOp.RevokeOthersByUser(ctx, "admin", s3Browser.Metadata.Name); err != nil {
		t.Fatalf("RevokeOthersByUser: %v", err)
	}
	if _, err := sessOp.Verify(ctx, s2.Token); err == nil {
		t.Fatal("expected s2 token to be invalidated after RevokeOthersByUser")
	}
	if err := sessOp.Delete(ctx, s3Browser.Metadata.Name); err != nil {
		t.Fatalf("Delete s3Browser: %v", err)
	}
	if _, err := sessOp.Verify(ctx, s3Browser.Token); err == nil {
		t.Fatal("expected s3Browser token to be invalidated after Delete")
	}
}

func TestPublicReadOnlyAppsAuthorizationAndSanitization(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := appmodel.EnsureBuiltInApps(ctx, reg); err != nil {
		t.Fatalf("EnsureBuiltInApps: %v", err)
	}
	appOp := appmodel.NewOperator(reg, nil)
	if _, err := appOp.Create(ctx, &corev1.App{
		Metadata:    metav1.ObjectMeta{Name: "draft-hidden-game"},
		DisplayName: "Hidden Game", Category: "sandbox", Image: "alpine:3.20",
		Ports:     []corev1.InstancePort{{Name: "game", Protocol: "TCP", TargetPort: 8080}},
		Published: false,
	}); err != nil {
		t.Fatalf("Create unpublished app: %v", err)
	}

	anonUser := request.UserInfo{Name: request.AnonymousUser, Groups: []string{request.GroupUnauthenticated}}
	pathAuthz := authorization.NewPathAuthorizer()
	dec, _, err := pathAuthz.Authorize(ctx, authorization.AttributesRecord{
		User: anonUser, IsResourceRequest: true,
		APIGroup: corev1.GroupName, Resource: "apps", Verb: iamv1.VerbList,
	})
	if err != nil || dec != authorization.DecisionAllow {
		t.Fatalf("expected anonymous list apps to be allowed by PathAuthorizer, got dec=%v err=%v", dec, err)
	}
	decCreate, _, _ := pathAuthz.Authorize(ctx, authorization.AttributesRecord{
		User: anonUser, IsResourceRequest: true,
		APIGroup: corev1.GroupName, Resource: "apps", Verb: iamv1.VerbCreate,
	})
	if decCreate == authorization.DecisionAllow {
		t.Fatal("anonymous create apps must not be allowed by PathAuthorizer")
	}

	mux := http.NewServeMux()
	container := apiruntime.NewContainer(mux)
	appkapis.AddToContainer(apiruntime.Dependencies{DB: reg}, container)

	// Anonymous GET /apis/core.gamepanel.io/v1/apps
	reqList := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/apps", nil)
	reqList = reqList.WithContext(request.WithUser(reqList.Context(), anonUser))
	recList := httptest.NewRecorder()
	mux.ServeHTTP(recList, reqList)
	if recList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for anonymous list apps, got %d: %s", recList.Code, recList.Body.String())
	}
	var pubList metav1.List[*corev1.App]
	if err := json.Unmarshal(recList.Body.Bytes(), &pubList); err != nil {
		t.Fatalf("unmarshal public AppList: %v", err)
	}
	for _, item := range pubList.Items {
		if !item.Published || item.Metadata.Name == "draft-hidden-game" {
			t.Fatalf("unpublished app leaked to anonymous caller: %s", item.Metadata.Name)
		}
		if len(item.FileTemplates) != 0 || len(item.ConsoleActions) != 0 || len(item.ConfigSchema) != 0 || len(item.Metrics) != 0 {
			t.Fatalf("expected internal templates/schema/actions stripped for anonymous caller on %s", item.Metadata.Name)
		}
	}

	// Anonymous GET /apis/core.gamepanel.io/v1/apps/draft-hidden-game -> 404
	reqDraft := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/apps/draft-hidden-game", nil)
	reqDraft = reqDraft.WithContext(request.WithUser(reqDraft.Context(), anonUser))
	recDraft := httptest.NewRecorder()
	mux.ServeHTTP(recDraft, reqDraft)
	if recDraft.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for anonymous GET on unpublished app, got %d", recDraft.Code)
	}

	// Authenticated admin GET /apis/core.gamepanel.io/v1/apps -> includes full ConfigSchema & unpublished apps
	adminUser := request.UserInfo{Name: "admin", Kind: request.KindUser, Groups: []string{request.GroupAuthenticated}}
	reqAdmin := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/apps", nil)
	reqAdmin = reqAdmin.WithContext(request.WithUser(reqAdmin.Context(), adminUser))
	recAdmin := httptest.NewRecorder()
	mux.ServeHTTP(recAdmin, reqAdmin)
	var adminList metav1.List[*corev1.App]
	if err := json.Unmarshal(recAdmin.Body.Bytes(), &adminList); err != nil {
		t.Fatalf("unmarshal admin AppList: %v", err)
	}
	foundDraft := false
	foundFullMC := false
	for _, item := range adminList.Items {
		if item.Metadata.Name == "draft-hidden-game" {
			foundDraft = true
		}
		if item.Metadata.Name == "minecraft-paper" && len(item.ConfigSchema) > 0 && len(item.ConsoleActions) > 0 {
			foundFullMC = true
		}
	}
	if !foundDraft || !foundFullMC {
		t.Fatalf("expected authenticated caller to receive unpublished apps and full schema: foundDraft=%v foundFullMC=%v", foundDraft, foundFullMC)
	}
}

func TestEnsureBuiltInAppsManifestEvolutionSync(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := appmodel.EnsureBuiltInApps(ctx, reg); err != nil {
		t.Fatalf("first EnsureBuiltInApps: %v", err)
	}

	appDB := appdb.NewDB(reg.Pool())
	mc, err := appDB.Get(ctx, "minecraft-paper")
	if err != nil {
		t.Fatalf("Get minecraft-paper: %v", err)
	}
	latestHash := mc.Metadata.Annotations[appmodel.AnnotationBuiltInHash]
	if latestHash == "" {
		t.Fatal("expected built-in app to carry core.gamepanel.io/builtin-hash annotation")
	}

	// Simulate an older DB record where admin uploaded a custom data: cover and unpublished the app.
	mc.CoverImage = "data:image/jpeg;base64,CUSTOM_ADMIN_COVER"
	mc.Published = false
	mc.Versions = []string{"1.19.4"}
	mc.ConfigSchema = nil
	mc.Metadata.Annotations[appmodel.AnnotationBuiltInHash] = "stale-manifest-hash"
	if _, err := appDB.Update(ctx, mc); err != nil {
		t.Fatalf("Update stale minecraft-paper: %v", err)
	}

	if err := appmodel.EnsureBuiltInApps(ctx, reg); err != nil {
		t.Fatalf("second EnsureBuiltInApps: %v", err)
	}
	synced, err := appDB.Get(ctx, "minecraft-paper")
	if err != nil {
		t.Fatalf("Get synced minecraft-paper: %v", err)
	}
	if synced.Metadata.Annotations[appmodel.AnnotationBuiltInHash] != latestHash {
		t.Fatalf("expected builtin-hash updated to %q, got %q", latestHash, synced.Metadata.Annotations[appmodel.AnnotationBuiltInHash])
	}
	if len(synced.Versions) < 2 || len(synced.ConfigSchema) == 0 {
		t.Fatalf("expected Versions and ConfigSchema synced from embedded YAML, got versions=%v schema=%d", synced.Versions, len(synced.ConfigSchema))
	}
	if synced.CoverImage != "data:image/jpeg;base64,CUSTOM_ADMIN_COVER" || synced.Published != false {
		t.Fatalf("expected custom CoverImage and Published=false preserved, got cover=%q published=%v", synced.CoverImage, synced.Published)
	}
}

func TestBuiltInRegistryBootstrap(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	polList, err := policydb.NewDB(reg.Pool()).List(ctx, db.ListOptions{})
	if err != nil || len(polList.Items) != 6 {
		t.Fatalf("expected 6 built-in ReplicationPolicies, got %+v err=%v", polList, err)
	}
	for _, pol := range polList.Items {
		if pol.Enabled {
			t.Fatalf("expected built-in ReplicationPolicy %s to default to Enabled=false", pol.Metadata.Name)
		}
	}
}
