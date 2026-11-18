package volume

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// DefaultXFSQuotaProviderName is the quota provider identifier for XFS directory project quotas.
const DefaultXFSQuotaProviderName = "xfs-project-quota"

func init() {
	RegisterQuotaProvider(DefaultXFSQuotaProviderName, func(workDir string, params map[string]string) (QuotaProvider, error) {
		minimum, err := strconv.ParseUint(params["storageQuotaProjectIDMin"], 10, 31)
		if err != nil || minimum == 0 {
			return nil, errors.New("storageQuotaProjectIDMin must specify a positive reserved project ID")
		}
		maximum, err := strconv.ParseUint(params["storageQuotaProjectIDMax"], 10, 31)
		if err != nil || maximum < minimum {
			return nil, errors.New("storageQuotaProjectIDMax must bound the reserved project ID range")
		}
		return NewXFSProjectQuotaProvider(workDir, params["storageQuotaMount"], uint32(minimum), uint32(maximum), params["storageQuotaBinary"])
	})
}

var (
	quotaGates        sync.Map
	quotaTempSequence atomic.Uint64
)

func lockQuotaGate(ctx context.Context, key string) (func(), error) {
	candidate := make(chan struct{}, 1)
	gate, _ := quotaGates.LoadOrStore(key, candidate)
	ch := gate.(chan struct{})
	select {
	case ch <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-ch
			return nil, err
		}
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func syncRootDirectory(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

type xfsQuotaRunner func(context.Context, string, ...string) ([]byte, error)

type xfsQuotaPlatform interface {
	Probe(context.Context, string, string) error
	ProbeMount(context.Context, string) error
	SyncMount(string) error
	Lock(context.Context, *os.File) (func(), error)
}

// XFSProjectQuotaProvider enforces the UID data directory on an already mounted Linux XFS filesystem.
type XFSProjectQuotaProvider struct {
	workDir, mount, binary string
	minimum, maximum       uint32
	run                    xfsQuotaRunner
	platform               xfsQuotaPlatform
}

var xfsSafePath = regexp.MustCompile(`^/[-a-zA-Z0-9_./]+$`)

func NewXFSProjectQuotaProvider(workDir, mount string, minimum, maximum uint32, executable string) (*XFSProjectQuotaProvider, error) {
	base, err := filepath.Abs(workDir)
	if err != nil || strings.TrimSpace(workDir) == "" || filepath.Clean(workDir) == "." {
		return nil, errors.New("XFS quota requires a private node workDir")
	}
	if !filepath.IsAbs(mount) || filepath.Clean(mount) != mount || !xfsSafePath.MatchString(mount) || minimum == 0 || maximum < minimum || maximum > math.MaxInt32 {
		return nil, errors.New("XFS quota requires a clean absolute mount and a reserved positive 31-bit project ID range")
	}
	if !xfsSafePath.MatchString(base) || !pathWithin(mount, filepath.Join(base, "instances")) {
		return nil, errors.New("instance workDir must be a safe path within the configured XFS mount")
	}
	if executable == "" {
		executable = "/usr/sbin/xfs_quota"
	}
	if !filepath.IsAbs(executable) || strings.ContainsRune(executable, 0) {
		return nil, errors.New("storageQuotaBinary must be an absolute executable path")
	}
	return &XFSProjectQuotaProvider{workDir: base, mount: mount, binary: executable, minimum: minimum, maximum: maximum, run: runXFSQuota, platform: nativeXFSQuotaPlatform{}}, nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func (p *XFSProjectQuotaProvider) command(ctx context.Context, command string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.run(ctx, p.binary, "-x", "-D", "/dev/null", "-P", "/dev/null", "-c", command, p.mount)
}

type quotaCommandOutput struct{ bytes.Buffer }

func (output *quotaCommandOutput) Write(data []byte) (int, error) {
	if output.Len()+len(data) > 4<<20 {
		return 0, errors.New("xfs_quota output exceeded 4MiB")
	}
	return output.Buffer.Write(data)
}

func runXFSQuota(ctx context.Context, executable string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	command.WaitDelay = time.Second
	var output, diagnostic quotaCommandOutput
	command.Stdout, command.Stderr = &output, &diagnostic
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("xfs_quota failed: %w (%s)", err, strings.TrimSpace(diagnostic.String()))
	}
	if diagnostic.Len() != 0 {
		return nil, fmt.Errorf("xfs_quota reported unverifiable diagnostics: %s", strings.TrimSpace(diagnostic.String()))
	}
	return output.Bytes(), nil
}

func (p *XFSProjectQuotaProvider) Capabilities(ctx context.Context, volume Identity) (QuotaCapabilities, error) {
	if err := p.validateVolume(volume); err != nil {
		return QuotaCapabilities{}, err
	}
	if err := p.platform.Probe(ctx, p.mount, volume.Path); err != nil {
		return QuotaCapabilities{}, err
	}
	if err := p.verifyQuotaState(ctx); err != nil {
		return QuotaCapabilities{}, err
	}
	return QuotaCapabilities{HardLimit: true, CanShrink: true}, nil
}

func (p *XFSProjectQuotaProvider) verifyQuotaState(ctx context.Context) error {
	state, err := p.command(ctx, "state -p")
	if err != nil {
		return err
	}
	if !strings.Contains(string(state), "Project quota state on "+p.mount+" (") {
		return fmt.Errorf("XFS project quota state is missing: %w", ErrStorageQuotaUnsupported)
	}
	accounting, enforcement := 0, 0
	for _, line := range strings.Split(string(state), "\n") {
		switch strings.Join(strings.Fields(line), " ") {
		case "Accounting: ON":
			accounting++
		case "Enforcement: ON":
			enforcement++
		}
	}
	if accounting != 1 || enforcement != 1 || strings.Contains(string(state), "OFF") {
		return fmt.Errorf("XFS project accounting and enforcement must already be ON: %w", ErrStorageQuotaUnsupported)
	}
	return nil
}

func (p *XFSProjectQuotaProvider) validateVolume(volume Identity) error {
	if err := metav1.ValidateSingleSegmentName("volume instance UID", volume.InstanceUID); err != nil {
		return err
	}
	if volume.InstanceUID == "" || volume.NodeUID == "" || volume.AttemptID == "" || volume.Epoch <= 0 || volume.ResourceRevision <= 0 || !xfsSafePath.MatchString(volume.Path) {
		return errors.New("invalid XFS quota volume identity")
	}
	if filepath.Join(p.workDir, "instances", volume.InstanceUID) != volume.Path || filepath.Base(volume.InstanceUID) != volume.InstanceUID {
		return errors.New("XFS quota path must equal the immutable UID directory")
	}
	resolved, err := filepath.EvalSymlinks(volume.Path)
	if err != nil || resolved != volume.Path {
		return errors.New("XFS quota volume must exist without symbolic-link path components")
	}
	return nil
}

type xfsProjectRecord struct {
	Intent      Identity
	IntentLimit int64
	Applied     Identity
}

type xfsProjectRegistry struct {
	Mount    string
	Minimum  uint32
	Maximum  uint32
	Projects map[uint32]xfsProjectRecord
}

func (p *XFSProjectQuotaProvider) projectID(uid string) uint32 {
	hash := sha256.Sum256([]byte(uid))
	return p.minimum + uint32(binary.BigEndian.Uint64(hash[:8])%uint64(p.maximum-p.minimum+1))
}

func (p *XFSProjectQuotaProvider) registry(ctx context.Context) (*os.Root, *xfsProjectRegistry, func(), error) {
	hash := sha256.Sum256([]byte(p.mount))
	key := hex.EncodeToString(hash[:])
	root, cleanup, err := p.openLockedQuotaRoot(ctx, key)
	if err != nil {
		return nil, nil, nil, err
	}
	registry, err := p.loadAndValidateRegistry(root, key)
	if err != nil {
		cleanup()
		return nil, nil, nil, err
	}
	return root, registry, cleanup, nil
}

func (p *XFSProjectQuotaProvider) openLockedQuotaRoot(ctx context.Context, key string) (*os.Root, func(), error) {
	private := filepath.Join(p.workDir, ".volume-quotas")
	release, err := lockQuotaGate(ctx, filepath.Join(private, key))
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.Root, func(), error) { release(); return nil, nil, err }
	base, err := os.OpenRoot(p.workDir)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = base.Close() }()
	if err := base.MkdirAll(".volume-quotas", 0o700); err != nil {
		return fail(err)
	}
	if err := syncRootDirectory(base); err != nil {
		return fail(err)
	}
	info, err := base.Lstat(".volume-quotas")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fail(errors.New("XFS quota registry must be a private directory"))
	}
	root, err := base.OpenRoot(".volume-quotas")
	if err != nil {
		return fail(err)
	}
	for _, name := range []string{key + ".lock", key + ".json"} {
		if info, err := root.Lstat(name); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
			_ = root.Close()
			return fail(errors.New("XFS quota state must use private regular files"))
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = root.Close()
			return fail(err)
		}
	}
	lock, err := root.OpenFile(key+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		_ = root.Close()
		return fail(err)
	}
	unlock, err := p.platform.Lock(ctx, lock)
	if err != nil {
		_ = lock.Close()
		_ = root.Close()
		return fail(err)
	}
	return root, func() { unlock(); _ = lock.Close(); _ = root.Close(); release() }, nil
}

func findProjectByUID(registry *xfsProjectRegistry, uid string) (uint32, xfsProjectRecord, bool) {
	if registry == nil {
		return 0, xfsProjectRecord{}, false
	}
	for id, record := range registry.Projects {
		if record.Intent.InstanceUID == uid {
			return id, record, true
		}
	}
	return 0, xfsProjectRecord{}, false
}

func (p *XFSProjectQuotaProvider) loadAndValidateRegistry(root *os.Root, key string) (*xfsProjectRegistry, error) {
	registry := &xfsProjectRegistry{Mount: p.mount, Minimum: p.minimum, Maximum: p.maximum, Projects: make(map[uint32]xfsProjectRecord)}
	data, err := root.ReadFile(key + ".json")
	if err == nil {
		if err := json.Unmarshal(data, registry); err != nil {
			return nil, fmt.Errorf("damaged XFS project registry: %w", err)
		}
		if registry.Mount != p.mount || registry.Minimum != p.minimum || registry.Maximum != p.maximum || registry.Projects == nil {
			return nil, errors.New("XFS project registry configuration differs; existing IDs cannot be reassigned")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	seenUIDs := make(map[string]bool, len(registry.Projects))
	for id, record := range registry.Projects {
		if err := metav1.ValidateSingleSegmentName("registered volume UID", record.Intent.InstanceUID); err != nil {
			return nil, err
		}
		if seenUIDs[record.Intent.InstanceUID] {
			return nil, errors.New("XFS project registry contains duplicate instance UID mappings")
		}
		seenUIDs[record.Intent.InstanceUID] = true
		if id < p.minimum || id > p.maximum || record.Intent.NodeUID == "" || record.Intent.AttemptID == "" || record.Intent.Epoch <= 0 || record.Intent.ResourceRevision <= 0 || record.IntentLimit <= 0 || record.IntentLimit%1024 != 0 || filepath.Join(p.workDir, "instances", record.Intent.InstanceUID) != record.Intent.Path {
			return nil, errors.New("XFS project registry contains an invalid immutable mapping")
		}
		if record.Applied.InstanceUID != "" {
			if err := IdentityAccepts(record.Applied, record.Intent); err != nil {
				return nil, errors.New("XFS project registry contains an invalid applied identity")
			}
		}
	}
	return registry, nil
}

func (p *XFSProjectQuotaProvider) saveRegistry(root *os.Root, registry *xfsProjectRegistry) error {
	hash := sha256.Sum256([]byte(p.mount))
	name := hex.EncodeToString(hash[:]) + ".json"
	data, err := json.Marshal(registry)
	if err != nil {
		return err
	}
	temporary := name + ".tmp-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(quotaTempSequence.Add(1), 10)
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temporary) }()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := root.Rename(temporary, name); err != nil {
		return err
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (p *XFSProjectQuotaProvider) checkProject(ctx context.Context, volume Identity, id uint32) error {
	text, err := p.command(ctx, fmt.Sprintf("project -c -p %s %d", volume.Path, id))
	if err != nil {
		return err
	}
	want := fmt.Sprintf("Checking project %d (path %s)...\nProcessed 1 (/dev/null and cmdline) paths for project %d with recursion depth infinite (-1).", id, volume.Path, id)
	if strings.TrimSpace(string(text)) != want {
		return errors.New("XFS project tree/inheritance verification failed")
	}
	return nil
}

func parseXFSBlockReport(data []byte, id uint32) (int64, error) {
	lines := strings.FieldsFunc(strings.TrimSpace(string(data)), func(r rune) bool { return r == '\n' || r == '\r' })
	if len(lines) == 0 {
		return 0, nil
	}
	if len(lines) != 1 {
		return 0, errors.New("unexpected XFS block report rows")
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 4 || fields[0] != "#"+strconv.FormatUint(uint64(id), 10) {
		return 0, errors.New("unexpected XFS block report identity")
	}
	used, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || used < 0 || used > math.MaxInt64/1024 {
		return 0, errors.New("invalid XFS block usage")
	}
	return used * 1024, nil
}

func parseXFSLimitDump(data []byte, id uint32) (int64, error) {
	lines := strings.FieldsFunc(strings.TrimSpace(string(data)), func(r rune) bool { return r == '\n' || r == '\r' })
	if len(lines) == 0 {
		return 0, nil
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "fs = ") {
		return 0, errors.New("unexpected XFS raw quota dump")
	}
	fields := strings.Fields(lines[1])
	if len(fields) != 5 && len(fields) != 7 || fields[0] != strconv.FormatUint(uint64(id), 10) {
		return 0, errors.New("unexpected XFS raw quota identity")
	}
	values := make([]int64, len(fields)-1)
	for index, field := range fields[1:] {
		value, err := strconv.ParseInt(field, 10, 64)
		if err != nil || value < 0 {
			return 0, errors.New("invalid XFS raw quota limit")
		}
		values[index] = value
	}
	if len(values) == 6 && (values[4] != 0 || values[5] != 0) {
		return 0, errors.New("XFS realtime quota volumes are unsupported")
	}
	if values[1] > math.MaxInt64/512 {
		return 0, errors.New("XFS hard quota overflows bytes")
	}
	return values[1] * 512, nil
}

func (p *XFSProjectQuotaProvider) readQuota(ctx context.Context, id uint32) (int64, int64, error) {
	dump, err := p.command(ctx, fmt.Sprintf("dump -p -L %d -U %d", id, id))
	if err != nil {
		return 0, 0, err
	}
	hard, err := parseXFSLimitDump(dump, id)
	if err != nil {
		return 0, 0, err
	}
	report, err := p.command(ctx, fmt.Sprintf("report -p -b -n -N -L %d -U %d", id, id))
	if err != nil {
		return 0, 0, err
	}
	if hard > 0 && len(bytes.TrimSpace(report)) == 0 {
		return 0, 0, errors.New("XFS hard quota exists but charged usage is unavailable")
	}
	used, err := parseXFSBlockReport(report, id)
	return hard, used, err
}

func (p *XFSProjectQuotaProvider) Apply(ctx context.Context, volume Identity, limit int64) error {
	if limit <= 0 || limit%1024 != 0 {
		return errors.New("XFS CLI storage limit must be a positive exact multiple of 1024 bytes; rounding is forbidden")
	}
	if _, err := p.Capabilities(ctx, volume); err != nil {
		return err
	}
	root, registry, release, err := p.registry(ctx)
	if err != nil {
		return err
	}
	defer release()
	id, record, err := p.resolveOrAllocateProject(ctx, registry, volume, limit)
	if err != nil {
		return err
	}
	registry.Projects[id] = record
	if err := p.saveRegistry(root, registry); err != nil {
		return err
	}
	if err := p.enforceProjectQuotaOnDisk(ctx, volume, id, limit, record.Applied.InstanceUID == ""); err != nil {
		return err
	}
	record.Applied = volume
	registry.Projects[id] = record
	return p.saveRegistry(root, registry)
}

func (p *XFSProjectQuotaProvider) resolveOrAllocateProject(ctx context.Context, registry *xfsProjectRegistry, volume Identity, limit int64) (uint32, xfsProjectRecord, error) {
	if id, record, exists := findProjectByUID(registry, volume.InstanceUID); exists {
		if err := IdentityAccepts(record.Intent, volume); err != nil {
			return 0, record, err
		}
		if record.Intent.ResourceRevision == volume.ResourceRevision && record.IntentLimit != limit {
			return 0, record, errors.New("XFS quota intent is immutable at the same approved resource revision")
		}
		record.Intent = volume
		record.IntentLimit = limit
		return id, record, nil
	}
	entries, err := os.ReadDir(volume.Path)
	if err != nil || len(entries) != 0 {
		return 0, xfsProjectRecord{}, errors.New("cannot assign an XFS project to a preexisting nonempty directory")
	}
	id, err := p.allocateProjectID(ctx, registry, volume.InstanceUID)
	if err != nil {
		return 0, xfsProjectRecord{}, err
	}
	return id, xfsProjectRecord{Intent: volume, IntentLimit: limit}, nil
}

func (p *XFSProjectQuotaProvider) allocateProjectID(ctx context.Context, registry *xfsProjectRegistry, uid string) (uint32, error) {
	span := uint64(p.maximum) - uint64(p.minimum) + 1
	maxProbes := span
	if maxProbes > 256 {
		maxProbes = 256
	}
	offset := uint64(p.projectID(uid) - p.minimum)
	for step := range maxProbes {
		candidate := p.minimum + uint32((offset+step)%span)
		if _, taken := registry.Projects[candidate]; taken {
			continue
		}
		hard, used, err := p.readQuota(ctx, candidate)
		if err != nil {
			return 0, fmt.Errorf("reserved XFS project ID %d is unverifiable: %w", candidate, err)
		}
		if hard == 0 && used == 0 {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no free XFS project IDs available in range [%d..%d]", p.minimum, p.maximum)
}

func (p *XFSProjectQuotaProvider) enforceProjectQuotaOnDisk(ctx context.Context, volume Identity, id uint32, limit int64, uninitialized bool) error {
	if uninitialized {
		if err := p.checkProject(ctx, volume, id); err != nil {
			entries, readErr := os.ReadDir(volume.Path)
			if readErr != nil || len(entries) != 0 {
				return errors.New("uninitialized XFS project contains data; refusing recursive reassignment")
			}
			if _, err := p.command(ctx, fmt.Sprintf("project -s -d 0 -p %s %d", volume.Path, id)); err != nil {
				return err
			}
		}
	}
	if err := p.checkProject(ctx, volume, id); err != nil {
		return err
	}
	_, used, err := p.readQuota(ctx, id)
	if err != nil || used > limit {
		return fmt.Errorf("XFS charged usage cannot fit storage limit %d (upper bound %d): %w", limit, used, err)
	}
	if _, err := p.command(ctx, fmt.Sprintf("limit -p bsoft=0 bhard=%d %d", limit, id)); err != nil {
		return err
	}
	hard, used, err := p.readQuota(ctx, id)
	if err != nil || hard != limit || used > limit {
		return fmt.Errorf("XFS hard quota change remains unverified: hard=%d approved=%d chargedUpperBound=%d error=%w", hard, limit, used, err)
	}
	return p.checkProject(ctx, volume, id)
}

func (p *XFSProjectQuotaProvider) Observe(ctx context.Context, volume Identity) (QuotaObservation, error) {
	if _, err := p.Capabilities(ctx, volume); err != nil {
		return QuotaObservation{}, err
	}
	_, registry, release, err := p.registry(ctx)
	if err != nil {
		return QuotaObservation{}, err
	}
	defer release()
	id, record, exists := findProjectByUID(registry, volume.InstanceUID)
	if !exists || record.Applied.InstanceUID == "" {
		return QuotaObservation{}, ErrVolumeQuotaNotFound
	}
	if err := IdentityAccepts(record.Intent, volume); err != nil {
		return QuotaObservation{}, err
	}
	if err := p.checkProject(ctx, volume, id); err != nil {
		return QuotaObservation{}, err
	}
	hard, used, err := p.readQuota(ctx, id)
	if err != nil {
		return QuotaObservation{}, err
	}
	if hard <= 0 {
		return QuotaObservation{}, ErrVolumeQuotaNotFound
	}
	return QuotaObservation{Volume: record.Applied, HardLimitBytes: hard, UsedBytes: used, Enforced: true}, nil
}

// Release clears the kernel XFS hard limit and removes the UID project binding after workdir removal.
func (p *XFSProjectQuotaProvider) Release(ctx context.Context, uid string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cleanUID := strings.TrimSpace(uid)
	if err := metav1.ValidateSingleSegmentName("volume instance UID", cleanUID); err != nil {
		return err
	}
	instancePath := filepath.Join(p.workDir, "instances", cleanUID)
	if _, err := os.Lstat(instancePath); err == nil {
		return fmt.Errorf("cannot release XFS quota while instance directory %s still exists", instancePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := p.platform.ProbeMount(ctx, p.mount); err != nil {
		return err
	}
	if err := p.verifyQuotaState(ctx); err != nil {
		return err
	}
	return p.releaseProjectRecord(ctx, cleanUID)
}

func (p *XFSProjectQuotaProvider) releaseProjectRecord(ctx context.Context, cleanUID string) error {
	root, registry, unlock, err := p.registry(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	id, _, exists := findProjectByUID(registry, cleanUID)
	if !exists {
		return nil
	}
	if _, err := p.command(ctx, fmt.Sprintf("limit -p bsoft=0 bhard=0 %d", id)); err != nil {
		return err
	}
	if err := p.platform.SyncMount(p.mount); err != nil {
		return err
	}
	hard, used, err := p.readQuota(ctx, id)
	if err != nil {
		return err
	}
	if hard != 0 || used != 0 {
		return fmt.Errorf("XFS project %d retains active quota after release (hard=%d used=%d)", id, hard, used)
	}
	delete(registry.Projects, id)
	return p.saveRegistry(root, registry)
}

var _ QuotaProvider = (*XFSProjectQuotaProvider)(nil)
var _ io.Writer = (*quotaCommandOutput)(nil)
