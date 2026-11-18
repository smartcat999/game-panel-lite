package volume

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeXFSQuotaPlatform struct {
	mu              sync.Mutex
	probeCalls      int
	probeMountCalls int
	syncCalls       int
}

func (f *fakeXFSQuotaPlatform) Probe(context.Context, string, string) error {
	f.mu.Lock()
	f.probeCalls++
	f.mu.Unlock()
	return nil
}

func (f *fakeXFSQuotaPlatform) ProbeMount(context.Context, string) error {
	f.mu.Lock()
	f.probeMountCalls++
	f.mu.Unlock()
	return nil
}

func (f *fakeXFSQuotaPlatform) SyncMount(string) error {
	f.mu.Lock()
	f.syncCalls++
	f.mu.Unlock()
	return nil
}

func (f *fakeXFSQuotaPlatform) Lock(context.Context, *os.File) (func(), error) {
	return func() {}, nil
}

type fakeXFSQuotaBackend struct {
	mu       sync.Mutex
	mount    string
	projects map[string]uint32
	limits   map[uint32]int64
	usedKiB  map[uint32]int64
}

func newFakeXFSQuotaBackend(mount string) *fakeXFSQuotaBackend {
	return &fakeXFSQuotaBackend{
		mount:    mount,
		projects: make(map[string]uint32),
		limits:   make(map[uint32]int64),
		usedKiB:  make(map[uint32]int64),
	}
}

func (b *fakeXFSQuotaBackend) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cmd := extractXFSCommandArg(args)
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return nil, fmt.Errorf("missing -c xfs_quota command: %v", args)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch fields[0] {
	case "state":
		text := fmt.Sprintf("Project quota state on %s (/dev/loop0)\n  Accounting: ON\n  Enforcement: ON\n", b.mount)
		return []byte(text), nil
	case "project":
		return b.execProject(fields)
	case "dump":
		return b.execDump(fields)
	case "report":
		return b.execReport(fields)
	case "limit":
		return b.execLimit(fields)
	default:
		return nil, fmt.Errorf("unsupported fake xfs_quota command %q", cmd)
	}
}

func extractXFSCommandArg(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" {
			return args[i+1]
		}
	}
	return ""
}

func (b *fakeXFSQuotaBackend) execProject(fields []string) ([]byte, error) {
	pathVal, idVal := "", ""
	isSet := false
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "-s":
			isSet = true
		case "-p":
			if i+1 < len(fields) {
				pathVal = fields[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(fields[i], "-") && fields[i-1] != "-d" {
				idVal = fields[i]
			}
		}
	}
	id64, _ := strconv.ParseUint(idVal, 10, 32)
	id := uint32(id64)
	if isSet {
		b.projects[pathVal] = id
	} else if b.projects[pathVal] != id {
		return []byte("project mismatch\n"), nil
	}
	out := fmt.Sprintf("Checking project %d (path %s)...\nProcessed 1 (/dev/null and cmdline) paths for project %d with recursion depth infinite (-1).\n", id, pathVal, id)
	return []byte(out), nil
}

func (b *fakeXFSQuotaBackend) execDump(fields []string) ([]byte, error) {
	id := parseLastUint32(fields)
	limit, ok := b.limits[id]
	if !ok {
		return nil, nil
	}
	out := fmt.Sprintf("fs = %s\n%d 0 %d 0 0\n", b.mount, id, limit/512)
	return []byte(out), nil
}

func (b *fakeXFSQuotaBackend) execReport(fields []string) ([]byte, error) {
	id := parseLastUint32(fields)
	limit := b.limits[id]
	used := b.usedKiB[id]
	if limit == 0 && used == 0 {
		return nil, nil
	}
	out := fmt.Sprintf("#%d %d 0 %d 00 [--------]\n", id, used, limit/1024)
	return []byte(out), nil
}

func (b *fakeXFSQuotaBackend) execLimit(fields []string) ([]byte, error) {
	var bhard int64
	for _, f := range fields {
		if raw, ok := strings.CutPrefix(f, "bhard="); ok {
			bhard, _ = strconv.ParseInt(raw, 10, 64)
		}
	}
	id := parseLastUint32(fields)
	b.limits[id] = bhard
	return nil, nil
}

func parseLastUint32(fields []string) uint32 {
	if len(fields) == 0 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[len(fields)-1], 10, 32)
	return uint32(v)
}

func newTestXFSProvider(t *testing.T, minID, maxID uint32) (*XFSProjectQuotaProvider, *fakeXFSQuotaBackend, *fakeXFSQuotaPlatform, string) {
	t.Helper()
	rawBase := t.TempDir()
	base, err := filepath.EvalSymlinks(rawBase)
	if err != nil {
		t.Fatalf("eval temp dir: %v", err)
	}
	workDir := filepath.Join(base, "work")
	if err := os.MkdirAll(filepath.Join(workDir, "instances"), 0o755); err != nil {
		t.Fatalf("mkdir instances: %v", err)
	}
	p, err := NewXFSProjectQuotaProvider(workDir, base, minID, maxID, "/usr/sbin/xfs_quota")
	if err != nil {
		t.Fatalf("NewXFSProjectQuotaProvider: %v", err)
	}
	backend := newFakeXFSQuotaBackend(base)
	platform := &fakeXFSQuotaPlatform{}
	p.run = backend.run
	p.platform = platform
	return p, backend, platform, workDir
}

func makeTestInstanceVol(t *testing.T, workDir, uid string) Identity {
	t.Helper()
	dir := filepath.Join(workDir, "instances", uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir instance %s: %v", uid, err)
	}
	return Identity{
		InstanceUID:      uid,
		NodeUID:          "node-1",
		AttemptID:        "attempt-1",
		Epoch:            1,
		ResourceRevision: 1,
		Path:             dir,
	}
}

func TestVerifyXFSMountInfoAndRoot(t *testing.T) {
	good := []byte("31 1 8:1 / /var/lib/gp rw,relatime shared:1 - xfs /dev/loop0 rw,attr2,inode64,logbufs=8,prjquota\n")
	vol := "/var/lib/gp/instances/u1"
	if err := verifyXFSMountInfo(good, "/var/lib/gp", vol); err != nil {
		t.Fatalf("verifyXFSMountInfo valid mount: %v", err)
	}
	if err := verifyXFSMountRoot(good, "/var/lib/gp"); err != nil {
		t.Fatalf("verifyXFSMountRoot valid mount: %v", err)
	}
	badNoQuota := []byte("31 1 8:1 / /var/lib/gp rw - xfs /dev/loop0 rw,noquota\n")
	if err := verifyXFSMountRoot(badNoQuota, "/var/lib/gp"); err == nil {
		t.Fatal("expected missing prjquota mount to be rejected")
	}
	badNoEnforce := []byte("31 1 8:1 / /var/lib/gp rw - xfs /dev/loop0 rw,prjquota,pqnoenforce\n")
	if err := verifyXFSMountRoot(badNoEnforce, "/var/lib/gp"); err == nil {
		t.Fatal("expected pqnoenforce mount to be rejected")
	}
	badRO := []byte("31 1 8:1 / /var/lib/gp ro - xfs /dev/loop0 rw,prjquota\n")
	if err := verifyXFSMountInfo(badRO, "/var/lib/gp", vol); err == nil {
		t.Fatal("expected read-only XFS mount to be rejected")
	}
}

func TestParseXFSReportsAndExactFullQuota(t *testing.T) {
	used, err := parseXFSBlockReport([]byte("#20001 32768 0 32768 00 [--------]\n"), 20001)
	if err != nil || used != 32<<20 {
		t.Fatalf("parseXFSBlockReport 32Mi = (%d, %v), want %d", used, err, 32<<20)
	}
	empty, err := parseXFSBlockReport(nil, 20001)
	if err != nil || empty != 0 {
		t.Fatalf("parseXFSBlockReport empty = (%d, %v), want 0", empty, err)
	}
	hard5, err := parseXFSLimitDump([]byte("fs = /mnt/xfs\n20001 0 65536 0 0\n"), 20001)
	if err != nil || hard5 != 32<<20 {
		t.Fatalf("parseXFSLimitDump 5-field = (%d, %v), want %d", hard5, err, 32<<20)
	}
	if _, err := parseXFSLimitDump([]byte("fs = /mnt/xfs\n20001 0 65536 0 0 1 0\n"), 20001); err == nil {
		t.Fatal("expected realtime XFS quota dump to be rejected")
	}
}

func TestXFSProjectQuotaApplyObserveAndExactFullUsage(t *testing.T) {
	ctx := context.Background()
	p, backend, _, workDir := newTestXFSProvider(t, 20001, 20004)
	vol := makeTestInstanceVol(t, workDir, "inst-exact-full")
	const limit = int64(32 << 20)
	if err := ApplyQuota(ctx, p, vol, limit); err != nil {
		t.Fatalf("ApplyQuota: %v", err)
	}
	backend.mu.Lock()
	id := backend.projects[vol.Path]
	backend.usedKiB[id] = 32768 // exact 32MiB usage
	backend.mu.Unlock()
	obs, err := p.Observe(ctx, vol)
	if err != nil {
		t.Fatalf("Observe at exact full quota: %v", err)
	}
	if err := VerifyObservation(obs, vol, limit); err != nil {
		t.Fatalf("VerifyObservation at exact 100%% quota failed: %v", err)
	}
	vol.ResourceRevision = 2
	if err := ApplyQuota(ctx, p, vol, 16<<20); err == nil {
		t.Fatal("expected shrink below exact 32Mi usage to fail")
	}
}

func TestXFSProjectQuotaCollisionProbeAndExhaustion(t *testing.T) {
	ctx := context.Background()
	p, _, _, workDir := newTestXFSProvider(t, 20001, 20002)
	v1 := makeTestInstanceVol(t, workDir, "uid-alpha")
	v2 := makeTestInstanceVol(t, workDir, "uid-beta")
	v3 := makeTestInstanceVol(t, workDir, "uid-gamma")
	if err := ApplyQuota(ctx, p, v1, 32<<20); err != nil {
		t.Fatalf("ApplyQuota v1: %v", err)
	}
	if err := ApplyQuota(ctx, p, v2, 32<<20); err != nil {
		t.Fatalf("ApplyQuota v2 on 2-slot pool: %v", err)
	}
	if err := ApplyQuota(ctx, p, v3, 32<<20); err == nil {
		t.Fatal("expected 3rd active UID on 2-slot project pool to fail")
	}
}

func TestXFSProjectQuotaReleaseReclaimAndReuse(t *testing.T) {
	ctx := context.Background()
	p, backend, platform, workDir := newTestXFSProvider(t, 20001, 20002)
	v1 := makeTestInstanceVol(t, workDir, "uid-reclaim-1")
	v2 := makeTestInstanceVol(t, workDir, "uid-reclaim-2")
	if err := ApplyQuota(ctx, p, v1, 32<<20); err != nil {
		t.Fatalf("ApplyQuota v1: %v", err)
	}
	if err := ApplyQuota(ctx, p, v2, 32<<20); err != nil {
		t.Fatalf("ApplyQuota v2: %v", err)
	}
	if err := p.Release(ctx, v1.InstanceUID); err == nil {
		t.Fatal("expected Release while instance directory exists to fail")
	}
	if err := RemoveInstanceWorkDir(workDir, v1.InstanceUID, "t", "n"); err != nil {
		t.Fatalf("RemoveInstanceWorkDir: %v", err)
	}
	if err := p.Release(ctx, v1.InstanceUID); err != nil {
		t.Fatalf("Release v1: %v", err)
	}
	if err := p.Release(ctx, v1.InstanceUID); err != nil {
		t.Fatalf("idempotent Release v1: %v", err)
	}
	id1 := backend.projects[v1.Path]
	if backend.limits[id1] != 0 || platform.syncCalls == 0 {
		t.Fatalf("expected released project %d bhard=0 and syncCalls>0, got bhard=%d sync=%d", id1, backend.limits[id1], platform.syncCalls)
	}
	v3 := makeTestInstanceVol(t, workDir, "uid-reclaim-3")
	if err := ApplyQuota(ctx, p, v3, 64<<20); err != nil {
		t.Fatalf("expected reclaimed project ID to be reusable for v3: %v", err)
	}
}
