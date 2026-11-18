package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type memS3Server struct {
	srv          *httptest.Server
	mu           sync.Mutex
	objects      map[string][]byte
	deletedPaths []string
	putPaths     []string
	failDeletes  int
}

func (m *memS3Server) setFailDeletes(n int) {
	m.mu.Lock()
	m.failDeletes = n
	m.mu.Unlock()
}

func newMemS3Server(t *testing.T) *memS3Server {
	t.Helper()
	m := &memS3Server{objects: make(map[string][]byte)}
	m.srv = httptest.NewServer(http.HandlerFunc(m.serveHTTP))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *memS3Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		m.mu.Lock()
		if m.failDeletes > 0 {
			m.failDeletes--
			m.mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		m.deletedPaths = append(m.deletedPaths, r.URL.Path)
		delete(m.objects, r.URL.Path)
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPut:
		if copySrc := r.Header.Get("X-Amz-Copy-Source"); copySrc != "" {
			srcPath, _ := url.PathUnescape(copySrc)
			if !strings.HasPrefix(srcPath, "/") {
				srcPath = "/" + srcPath
			}
			m.mu.Lock()
			srcData, ok := m.objects[srcPath]
			if ok {
				m.objects[r.URL.Path] = append([]byte(nil), srcData...)
			}
			m.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<CopyObjectResult><ETag>"etag-copy"</ETag></CopyObjectResult>`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.objects[r.URL.Path] = body
		m.putPaths = append(m.putPaths, r.URL.Path)
		m.mu.Unlock()
		w.Header().Set("ETag", `"etag-put"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodHead, http.MethodGet:
		m.mu.Lock()
		data, ok := m.objects[r.URL.Path]
		if ok {
			data = append([]byte(nil), data...)
		}
		m.mu.Unlock()
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
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (m *memS3Server) hasObject(urlPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[urlPath]
	return ok
}

func (m *memS3Server) objectCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

func (m *memS3Server) deletedJoined() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.deletedPaths, ",")
}

func putPresignedURL(t *testing.T, cli *http.Client, targetURL string, data []byte) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, targetURL, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new PUT request: %v", err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", targetURL, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestDirectBackupUploadPhaseGatingStagingAndExpiryFence(t *testing.T) {
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	drivers := runtime.NewDefaultRegistry(t.TempDir())
	memS3 := newMemS3Server(t)

	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := tenantdb.NewDB(reg.Pool()).Create(ctx, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "default"},
		Spec: corev1.TenantSpec{
			DisplayName: "Default", Type: corev1.TenantTypeOrganization, Owner: "admin",
			Quota: corev1.ResourceList{
				corev1.ResourceCPU: "4000m", corev1.ResourceMemory: "8Gi", corev1.ResourceStorage: "50Gi",
				corev1.ResourceInstances: "4", corev1.ResourceBackups: "4", corev1.ResourceBackupStorage: "1Gi",
			},
		},
	}); err != nil {
		t.Fatalf("Create default tenant: %v", err)
	}
	inst := fixtures.CreateActive(t, ctx, reg, drivers, "default", "node-01", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "mc-sec-01"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker", Image: "itzg/minecraft-server:java21", State: corev1.InstanceStateRunning,
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceCPU: "1000m", corev1.ResourceMemory: "2Gi", corev1.ResourceStorage: "10Gi",
			}},
		},
	})

	s3Cfg := &config.S3Config{
		Endpoint: memS3.srv.URL, Region: "us-east-1", Bucket: "backups",
		AccessKey: "ak", SecretKey: "sk", UsePathStyle: true, PresignTTL: 10 * time.Minute,
	}
	config.Get().S3 = *s3Cfg
	var nowMu sync.Mutex
	curTime := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
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
	payload := buildFeatureTestTarGz(t, map[string]string{"world/level.dat": "payload-ok"})
	payloadSize := int64(len(payload))
	opID := "op-sec-upload-01"

	// 1. Direct upload URL targets staging key, never canonical ObjectKey.
	initRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: opID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
	})
	if err != nil || initRes.ExpiresAt == nil {
		t.Fatalf("upload_backup init failed: res=%+v err=%v", initRes, err)
	}
	parsedInit, err := url.Parse(initRes.DirectURL)
	if err != nil {
		t.Fatalf("parse DirectURL: %v", err)
	}
	if !strings.HasPrefix(parsedInit.Path, "/backups/staging/backups/"+initRes.OperationID) {
		t.Fatalf("expected staging upload path for %s, got %q", initRes.OperationID, parsedInit.Path)
	}
	if strings.Contains(parsedInit.Path, "sec-save.tar.gz") {
		t.Fatalf("presigned upload URL leaked canonical ObjectKey %q", parsedInit.Path)
	}

	// 2. Retrying upload_backup while Uploading preserves the frozen deadline without extending it.
	advanceClock(5 * time.Second)
	retryRes, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: opID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
	})
	if err != nil || retryRes.ExpiresAt == nil || !retryRes.ExpiresAt.Equal(*initRes.ExpiresAt) {
		t.Fatalf("expected frozen ExpiresAt=%v on Uploading retry, got %+v err=%v", initRes.ExpiresAt, retryRes, err)
	}

	// 3. CompleteUploadBackup -> Unknown; reusing OperationID (client or canonical) while Unknown MUST return Conflict.
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionCompleteUploadBackup, OperationID: initRes.OperationID, Path: initRes.Path,
	}); err != nil {
		t.Fatalf("complete_upload_backup failed: %v", err)
	}
	for _, reuseID := range []string{opID, initRes.OperationID} {
		if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
			Action: corev1.FileActionUploadBackup, OperationID: reuseID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
		}); !errors.Is(err, models.ErrConflict) {
			t.Fatalf("expected ErrConflict reusing OperationID %q in Unknown phase, got %v", reuseID, err)
		}
	}

	// 4. Upload payload via staging DirectURL and reconcile to Completed; reusing OperationID in Completed MUST fail with ErrConflict.
	if st := putPresignedURL(t, memS3.srv.Client(), initRes.DirectURL, payload); st != http.StatusOK {
		t.Fatalf("PUT staging failed: %d", st)
	}
	curInst, err := instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get before ReconcileBackups: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups promote: %v", err)
	}
	if !memS3.hasObject("/backups/"+initRes.Path) || memS3.hasObject("/backups/staging/backups/"+initRes.OperationID) {
		t.Fatalf("expected canonical object promoted and staging object deleted")
	}
	for _, reuseID := range []string{opID, initRes.OperationID} {
		if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
			Action: corev1.FileActionUploadBackup, OperationID: reuseID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
		}); !errors.Is(err, models.ErrConflict) {
			t.Fatalf("expected ErrConflict reusing OperationID %q in Completed phase, got %v", reuseID, err)
		}
	}

	// 5. Deleting before UploadExpiresAt enters Deleting and holds quota until expiry, even if client PUTs late to staging.
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionDeleteBackup, Path: initRes.Path,
	}); err != nil {
		t.Fatalf("delete_backup: %v", err)
	}
	for _, reuseID := range []string{opID, initRes.OperationID} {
		if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
			Action: corev1.FileActionUploadBackup, OperationID: reuseID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
		}); !errors.Is(err, models.ErrConflict) {
			t.Fatalf("expected ErrConflict reusing OperationID %q in Deleting phase, got %v", reuseID, err)
		}
	}
	curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get during Deleting: %v", err)
	}
	err = instOp.ReconcileBackups(ctx, curInst)
	var ctrlErr *corev1.Error
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != corev1.OutcomeUnknown {
		t.Fatalf("expected OutcomeUnknown holding quota before expiry, got %v", err)
	}
	acctMid, err := quotadb.NewDB(reg.Pool()).GetAccount(ctx, curInst.Status.ResourceControl.TenantUID)
	if err != nil || acctMid.Allocated.BackupsCount != 1 {
		t.Fatalf("expected quota retained while upload URL still valid, got %+v err=%v", acctMid.Allocated, err)
	}
	if memS3.hasObject("/backups/" + initRes.Path) {
		t.Fatalf("expected canonical backup deleted immediately on Deleting")
	}

	// Attacker replays still-valid DirectURL just before expiry -> writes to staging, not canonical.
	if st := putPresignedURL(t, memS3.srv.Client(), retryRes.DirectURL, []byte("late-unmetered-blob")); st != http.StatusOK {
		t.Fatalf("late PUT staging failed: %d", st)
	}

	// Advance clock past UploadExpiresAt -> cleanup sweeps late staging upload and refunds quota -> Deleted rejects reuse.
	advanceClock(20 * time.Second)
	curInst, err = instOp.Get(ctx, "default", inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get before final delete reconcile: %v", err)
	}
	if err := instOp.ReconcileBackups(ctx, curInst); err != nil {
		t.Fatalf("ReconcileBackups after expiry: %v", err)
	}
	if memS3.objectCount() != 0 {
		t.Fatalf("expected 0 objects left in S3 after expiry sweep, got %d", memS3.objectCount())
	}
	acctAfter, err := quotadb.NewDB(reg.Pool()).GetAccount(ctx, curInst.Status.ResourceControl.TenantUID)
	if err != nil || acctAfter.Allocated.BackupsCount != 0 || acctAfter.Allocated.BackupStorageBytes != 0 {
		t.Fatalf("expected quota fully refunded after expiry sweep, got %+v err=%v", acctAfter.Allocated, err)
	}
	for _, reuseID := range []string{opID, initRes.OperationID} {
		if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
			Action: corev1.FileActionUploadBackup, OperationID: reuseID, Path: "backups/sec-save.tar.gz", SizeBytes: payloadSize,
		}); !errors.Is(err, models.ErrConflict) {
			t.Fatalf("expected ErrConflict reusing OperationID %q in Deleted phase, got %v", reuseID, err)
		}
	}

	// 6. Expired Uploading window cannot reissue a new URL.
	opExpired := "op-sec-expired-01"
	expInit, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: opExpired, Path: "backups/sec-expired.tar.gz", SizeBytes: 64,
	})
	if err != nil || expInit.ExpiresAt == nil {
		t.Fatalf("upload_backup expired setup failed: %v", err)
	}
	advanceClock(expInit.ExpiresAt.Sub(nowFn()) + time.Second)
	if _, err := instOp.Files(ctx, "default", inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: opExpired, Path: "backups/sec-expired.tar.gz", SizeBytes: 64,
	}); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("expected ErrConflict on expired Uploading window reuse, got %v", err)
	}
}

type directUploadRecoveryEnv struct {
	ctx          context.Context
	reg          *repo.Registry
	memS3        *memS3Server
	inst         *corev1.Instance
	instOp       instancemodel.Operator
	nowFn        func() time.Time
	advanceClock func(time.Duration)
}

func setupDirectUploadRecoveryTest(t *testing.T, backupQuotaStorage string) *directUploadRecoveryEnv {
	t.Helper()
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	drivers := runtime.NewDefaultRegistry(t.TempDir())
	memS3 := newMemS3Server(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := tenantdb.NewDB(reg.Pool()).Create(ctx, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "default"},
		Spec: corev1.TenantSpec{
			DisplayName: "Default", Type: corev1.TenantTypeOrganization, Owner: "admin",
			Quota: corev1.ResourceList{
				corev1.ResourceCPU: "4000m", corev1.ResourceMemory: "8Gi", corev1.ResourceStorage: "50Gi",
				corev1.ResourceInstances: "4", corev1.ResourceBackups: "4", corev1.ResourceBackupStorage: corev1.Quantity(backupQuotaStorage),
			},
		},
	}); err != nil {
		t.Fatalf("Create default tenant: %v", err)
	}
	inst := fixtures.CreateActive(t, ctx, reg, drivers, "default", "node-01", &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "mc-rec-01"},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker", Image: "itzg/minecraft-server:java21", State: corev1.InstanceStateRunning,
			Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
				corev1.ResourceCPU: "1000m", corev1.ResourceMemory: "2Gi", corev1.ResourceStorage: "10Gi",
			}},
		},
	})
	s3Cfg := &config.S3Config{
		Endpoint: memS3.srv.URL, Region: "us-east-1", Bucket: "backups",
		AccessKey: "ak", SecretKey: "sk", UsePathStyle: true, PresignTTL: 10 * time.Minute,
	}
	config.Get().S3 = *s3Cfg
	var mu sync.Mutex
	cur := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	adv := func(d time.Duration) { mu.Lock(); cur = cur.Add(d); mu.Unlock() }
	instOp := instancemodel.NewOperator(reg, drivers, nil,
		instancemodel.WithS3Client(s3.NewClient(s3Cfg, memS3.srv.Client())),
		instancemodel.WithClock(nowFn),
	)
	return &directUploadRecoveryEnv{ctx: ctx, reg: reg, memS3: memS3, inst: inst, instOp: instOp, nowFn: nowFn, advanceClock: adv}
}

func deterministicUncompressibleBytes(n int) string {
	out := make([]byte, 0, n)
	h := sha256.Sum256([]byte("seed"))
	for len(out) < n {
		out = append(out, h[:]...)
		h = sha256.Sum256(h[:])
	}
	return string(out[:n])
}

func TestDirectBackupUploadAbortSweepAndExpiryMargin(t *testing.T) {
	fillPayload := buildFeatureTestTarGz(t, map[string]string{"world/fill.bin": deterministicUncompressibleBytes(300)})
	payload := buildFeatureTestTarGz(t, map[string]string{"world/a.dat": "ok"})
	quotaLimit := len(fillPayload) + len(payload) + 120
	env := setupDirectUploadRecoveryTest(t, strconv.Itoa(quotaLimit))

	fillRes, err := env.instOp.Files(env.ctx, "default", env.inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: "op-fill-01", Path: "backups/fill.tar.gz", SizeBytes: int64(len(fillPayload)),
	})
	if err != nil {
		t.Fatalf("fill upload_backup: %v", err)
	}
	if st := putPresignedURL(t, env.memS3.srv.Client(), fillRes.DirectURL, fillPayload); st != http.StatusOK {
		t.Fatalf("PUT fill staging=%d", st)
	}
	cur, _ := env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	if err := env.instOp.ReconcileBackups(env.ctx, cur); err != nil {
		t.Fatalf("reconcile fill backup: %v", err)
	}

	overSize := int64(len(payload) + 150)
	if _, err := env.instOp.Files(env.ctx, "default", env.inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: "op-over-quota", Path: "backups/over.tar.gz", SizeBytes: overSize,
	}); err == nil {
		t.Fatal("expected over-quota upload_backup to fail")
	}
	afterAbort, err := env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	if err != nil || len(afterAbort.Status.Backups) != 1 {
		t.Fatalf("expected only 1 completed backup in Status.Backups after setup abort, got %+v err=%v", afterAbort.Status.Backups, err)
	}
	okRes, err := env.instOp.Files(env.ctx, "default", env.inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: "op-valid-01", Path: "backups/ok.tar.gz", SizeBytes: int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("expected follow-up upload_backup to succeed after abort, got %v", err)
	}
	verifyStagingSweepRecovery(t, env, okRes, payload)
	verifyDirectUploadExpiryMargin(t, env)
}

func verifyStagingSweepRecovery(t *testing.T, env *directUploadRecoveryEnv, res *corev1.InstanceFiles, payload []byte) {
	t.Helper()
	if st := putPresignedURL(t, env.memS3.srv.Client(), res.DirectURL, payload); st != http.StatusOK {
		t.Fatalf("PUT staging=%d", st)
	}
	env.memS3.setFailDeletes(10)
	cur, err := env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	if err != nil {
		t.Fatalf("Get before first reconcile: %v", err)
	}
	if err := env.instOp.ReconcileBackups(env.ctx, cur); !errors.Is(err, models.ErrBadGateway) {
		t.Fatalf("expected ErrBadGateway on transient staging delete failure, got %v", err)
	}
	env.memS3.setFailDeletes(0)
	bdb := backupdb.NewDB(env.reg.Pool())
	rec, err := bdb.Get(env.ctx, res.OperationID)
	if err != nil || rec.Phase != backupdb.Completed || rec.Result == nil || rec.Result.StagingCleaned {
		t.Fatalf("expected Completed record with StagingCleaned=false, got %+v err=%v", rec, err)
	}
	if !env.memS3.hasObject("/backups/staging/backups/" + res.OperationID) {
		t.Fatal("expected staging object preserved after delete failure")
	}
	cur, _ = env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	if err := env.instOp.ReconcileBackups(env.ctx, cur); err != nil {
		t.Fatalf("second ReconcileBackups sweep failed: %v", err)
	}
	if env.memS3.hasObject("/backups/staging/backups/" + res.OperationID) {
		t.Fatal("expected staging object deleted on sweep")
	}
	if rec, err = bdb.Get(env.ctx, res.OperationID); err != nil || rec.Result == nil || !rec.Result.StagingCleaned {
		t.Fatalf("expected StagingCleaned=true after sweep, got %+v err=%v", rec, err)
	}
}

func verifyDirectUploadExpiryMargin(t *testing.T, env *directUploadRecoveryEnv) {
	t.Helper()
	marginRes, err := env.instOp.Files(env.ctx, "default", env.inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionUploadBackup, OperationID: "op-margin-01", Path: "backups/margin.tar.gz", SizeBytes: 64,
	})
	if err != nil || marginRes.ExpiresAt == nil {
		t.Fatalf("upload_backup margin setup failed: %v", err)
	}
	if _, err := env.instOp.Files(env.ctx, "default", env.inst.Metadata.Name, &corev1.InstanceFiles{
		Action: corev1.FileActionDeleteBackup, Path: marginRes.Path,
	}); err != nil {
		t.Fatalf("delete_backup margin: %v", err)
	}
	env.advanceClock(marginRes.ExpiresAt.Sub(env.nowFn()) + 3*time.Second)
	cur, _ := env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	err = env.instOp.ReconcileBackups(env.ctx, cur)
	var ctrlErr *corev1.Error
	if !errors.As(err, &ctrlErr) || ctrlErr.Code != corev1.OutcomeUnknown {
		t.Fatalf("expected OutcomeUnknown at UploadExpiresAt+3s margin, got %v", err)
	}
	acctMid, err := quotadb.NewDB(env.reg.Pool()).GetAccount(env.ctx, env.inst.Status.ResourceControl.TenantUID)
	if err != nil || acctMid.Allocated.BackupsCount != 3 {
		t.Fatalf("expected 3 backups held within 5s fence margin, got %+v err=%v", acctMid.Allocated, err)
	}
	env.advanceClock(3 * time.Second)
	cur, _ = env.instOp.Get(env.ctx, "default", env.inst.Metadata.Name)
	if err := env.instOp.ReconcileBackups(env.ctx, cur); err != nil {
		t.Fatalf("ReconcileBackups past 5s margin failed: %v", err)
	}
	rec, err := backupdb.NewDB(env.reg.Pool()).Get(env.ctx, marginRes.OperationID)
	if err != nil || rec.Phase != backupdb.Deleted {
		t.Fatalf("expected Deleted past 5s margin, got %+v err=%v", rec, err)
	}
	acctAfter, _ := quotadb.NewDB(env.reg.Pool()).GetAccount(env.ctx, env.inst.Status.ResourceControl.TenantUID)
	if acctAfter.Allocated.BackupsCount != 2 {
		t.Fatalf("expected 2 backups remaining after expiry margin refund, got %+v", acctAfter.Allocated)
	}
}
