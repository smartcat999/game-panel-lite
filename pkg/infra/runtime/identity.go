package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/network"
)

type executionIdentity struct {
	UID, NodeUID, AttemptID     string
	Epoch, Revision, Generation int64
}

type executionRecord struct {
	executionIdentity
	Closing bool
}

func identityOf(in *corev1.Instance) (executionIdentity, error) {
	if in == nil || in.Metadata.UID == "" || in.Status.ResourceControl == nil {
		return executionIdentity{}, fmt.Errorf("runtime execution requires an approved instance identity")
	}
	if err := metav1.ValidateSingleSegmentName("instance UID", in.Metadata.UID); err != nil {
		return executionIdentity{}, err
	}
	c := in.Status.ResourceControl
	a := c.CurrentAttempt
	if a != nil {
		deleting := in.Metadata.DeletionTimestamp != nil && c.Stage == corev1.InstanceStageDeleting
		if c.Stage != corev1.InstanceStageActive && !deleting || in.Metadata.DeletionTimestamp != nil && !deleting || (a.Phase != corev1.AttemptPhaseBound && (!deleting || a.Phase != corev1.AttemptPhaseClosing)) {
			return executionIdentity{}, fmt.Errorf("runtime identity is not a currently bound or deleting attempt")
		}
	}
	if c.ApprovedRevision <= 0 || in.Metadata.Generation <= 0 || a == nil || a.ID == "" || a.NodeUID == "" || a.NodeName == "" || a.Epoch <= 0 || in.Spec.NodeName != a.NodeName {
		return executionIdentity{}, fmt.Errorf("runtime execution requires a valid approved binding")
	}
	return executionIdentity{UID: in.Metadata.UID, NodeUID: a.NodeUID, AttemptID: a.ID, Epoch: a.Epoch, Revision: c.ApprovedRevision, Generation: in.Metadata.Generation}, nil
}

func (id executionIdentity) sameBinding(other executionIdentity) bool {
	return id.UID == other.UID && id.NodeUID == other.NodeUID && id.AttemptID == other.AttemptID && id.Epoch == other.Epoch
}

func (id executionIdentity) accepts(request executionIdentity) error {
	if id.UID != request.UID || id.NodeUID != request.NodeUID {
		return fmt.Errorf("runtime object belongs to a different instance or node UID")
	}
	if request.Epoch < id.Epoch || request.Revision < id.Revision || request.Generation < id.Generation {
		return fmt.Errorf("stale runtime execution would downgrade the workload")
	}
	if request.Epoch == id.Epoch && request.AttemptID != id.AttemptID {
		return fmt.Errorf("runtime attempt differs at the same epoch")
	}
	return nil
}

func (id executionIdentity) labels() map[string]string {
	return map[string]string{
		corev1.LabelRuntimeInstanceUID:      id.UID,
		corev1.LabelRuntimeNodeUID:          id.NodeUID,
		corev1.LabelRuntimeAttemptID:        id.AttemptID,
		corev1.LabelRuntimeEpoch:            strconv.FormatInt(id.Epoch, 10),
		corev1.LabelRuntimeResourceRevision: strconv.FormatInt(id.Revision, 10),
		corev1.LabelRuntimeGeneration:       strconv.FormatInt(id.Generation, 10),
	}
}

func identityFromLabels(labels map[string]string) (executionIdentity, error) {
	id := executionIdentity{UID: labels[corev1.LabelRuntimeInstanceUID], NodeUID: labels[corev1.LabelRuntimeNodeUID], AttemptID: labels[corev1.LabelRuntimeAttemptID]}
	var err error
	if id.Epoch, err = strconv.ParseInt(labels[corev1.LabelRuntimeEpoch], 10, 64); err != nil {
		return id, fmt.Errorf("container lacks a valid binding epoch")
	}
	if id.Revision, err = strconv.ParseInt(labels[corev1.LabelRuntimeResourceRevision], 10, 64); err != nil {
		return id, fmt.Errorf("container lacks a valid approved resource revision")
	}
	if id.Generation, err = strconv.ParseInt(labels[corev1.LabelRuntimeGeneration], 10, 64); err != nil {
		return id, fmt.Errorf("container lacks a valid generation")
	}
	if id.UID == "" || id.NodeUID == "" || id.AttemptID == "" || id.Epoch <= 0 || id.Revision <= 0 || id.Generation <= 0 {
		return id, fmt.Errorf("container lacks an approved execution identity")
	}
	return id, nil
}

// Ports have already been allocated atomically by the node resource store.
// A driver must never choose an unrecorded host port during execution.
func validateExecutionPorts(in *corev1.Instance) error {
	if in == nil {
		return fmt.Errorf("instance is required")
	}
	if in.Status.ResourceControl == nil {
		return fmt.Errorf("runtime ports differ from the approved assignments")
	}
	return network.Default().ValidateExecutionPorts(in.Spec.Ports, in.Status.ResourceControl.ApprovedPorts)
}

func executionResources(in *corev1.Instance) (corev1.Resources, error) {
	if in == nil || in.Status.ResourceControl == nil {
		return corev1.Resources{}, fmt.Errorf("runtime resources require an approved snapshot")
	}
	actual, err := corev1.NormalizeInstanceResources(in.Spec.Resources, nil)
	if err != nil {
		return corev1.Resources{}, fmt.Errorf("runtime resources: %w", err)
	}
	approved, err := corev1.ParseResources(in.Status.ResourceControl.ApprovedResources)
	if err != nil {
		return corev1.Resources{}, err
	}
	if actual.CPUMillicores != approved.CPUMillicores || actual.MemoryBytes != approved.MemoryBytes || actual.StorageBytes != approved.StorageBytes {
		return corev1.Resources{}, fmt.Errorf("runtime resources differ from the approved snapshot")
	}
	if actual.CPUMillicores > math.MaxInt64/1_000_000 {
		return corev1.Resources{}, fmt.Errorf("runtime CPU limit overflows Docker NanoCPUs")
	}
	return actual, nil
}

func appliedResourceList(cpu, memory, storage int64) corev1.ResourceList {
	return corev1.CanonicalResources(corev1.Resources{CPUMillicores: cpu, MemoryBytes: memory, StorageBytes: storage, InstanceCount: 1})
}

// Execution generations fence requests, but are not container configuration.
// Docker labels are immutable, so preserve the high-water identity separately
// from the tenant-writable bind directory and from a container's creation labels.
var executionGates sync.Map // absolute private journal path -> chan struct{}
var executionTempSequence atomic.Uint64

func lockExecution(ctx context.Context, key string) (func(), error) {
	candidate := make(chan struct{}, 1)
	gate, _ := executionGates.LoadOrStore(key, candidate)
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

func beginDockerExecution(ctx context.Context, workDir string, in *corev1.Instance) (func(), error) {
	return beginDockerOperation(ctx, workDir, in, false)
}

func beginDockerCleanup(ctx context.Context, workDir string, in *corev1.Instance) (func(), error) {
	return beginDockerOperation(ctx, workDir, in, true)
}

func resolveExecutionJournalLocation(workDir, uid string) (base, name, privatePath string, err error) {
	if strings.TrimSpace(workDir) == "" || filepath.Clean(workDir) == "." {
		return "", "", "", errors.New("private runtime workDir is required for execution fencing")
	}
	base, err = filepath.Abs(workDir)
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(uid))
	name = hex.EncodeToString(sum[:]) + ".json"
	return base, name, filepath.Join(base, ".runtime-execution", name), nil
}

func beginDockerOperation(ctx context.Context, workDir string, in *corev1.Instance, closing bool) (func(), error) {
	request, err := identityOf(in)
	if err != nil {
		return nil, err
	}
	base, name, privatePath, err := resolveExecutionJournalLocation(workDir, request.UID)
	if err != nil {
		return nil, err
	}
	release, err := lockExecution(ctx, privatePath)
	if err != nil {
		return nil, err
	}
	if err := persistExecutionFence(ctx, base, name, executionRecord{executionIdentity: request, Closing: closing}); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// verifyDockerReadAccess checks the per-instance execution fence without blocking on or mutating the write journal.
func verifyDockerReadAccess(workDir string, in *corev1.Instance) error {
	request, err := identityOf(in)
	if err != nil {
		return err
	}
	base, name, _, err := resolveExecutionJournalLocation(workDir, request.UID)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	const privateDir = ".runtime-execution"
	info, err := root.Lstat(privateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("runtime journal must be a private directory")
	}
	journal, err := root.OpenRoot(privateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = journal.Close() }()
	_, _, err = validateExistingJournalEntry(journal, name, executionRecord{executionIdentity: request})
	return err
}

func persistExecutionFence(ctx context.Context, base, name string, request executionRecord) error {
	journal, closeJournal, err := openPrivateJournalRoot(base)
	if err != nil {
		return err
	}
	defer closeJournal()
	alreadyRecorded, err := verifyExistingJournalEntry(journal, name, request)
	if err != nil {
		return err
	}
	if alreadyRecorded {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeJournalAtomically(ctx, journal, name, request)
}

func openPrivateJournalRoot(base string) (*os.Root, func(), error) {
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create runtime base: %w", err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, nil, err
	}
	const privateDir = ".runtime-execution"
	if err := root.MkdirAll(privateDir, 0o700); err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("create private runtime journal: %w", err)
	}
	if err := syncRootDirectory(root); err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("sync private runtime journal directory: %w", err)
	}
	info, err := root.Lstat(privateDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		_ = root.Close()
		return nil, nil, fmt.Errorf("runtime journal must be a private directory: %w", err)
	}
	journal, err := root.OpenRoot(privateDir)
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	return journal, func() { _ = journal.Close(); _ = root.Close() }, nil
}

func validateExistingJournalEntry(journal *os.Root, name string, request executionRecord) (executionRecord, bool, error) {
	if info, err := journal.Lstat(name); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		return executionRecord{}, false, errors.New("runtime execution journal must be a private regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return executionRecord{}, false, err
	}
	data, err := journal.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return executionRecord{}, false, nil
	}
	if err != nil {
		return executionRecord{}, false, fmt.Errorf("read runtime execution journal: %w", err)
	}
	var previous executionRecord
	if err := json.Unmarshal(data, &previous); err != nil {
		return executionRecord{}, false, fmt.Errorf("invalid runtime execution journal: %w", err)
	}
	if previous.UID == "" || previous.NodeUID == "" || previous.AttemptID == "" || previous.Epoch <= 0 || previous.Revision <= 0 || previous.Generation <= 0 {
		return executionRecord{}, false, errors.New("runtime execution journal has an invalid identity")
	}
	if err := previous.accepts(request.executionIdentity); err != nil {
		return executionRecord{}, false, err
	}
	if previous.Closing && previous.sameBinding(request.executionIdentity) && !request.Closing {
		return executionRecord{}, false, errors.New("runtime binding is closed for cleanup and cannot execute again")
	}
	return previous, true, nil
}

func verifyExistingJournalEntry(journal *os.Root, name string, request executionRecord) (bool, error) {
	previous, exists, err := validateExistingJournalEntry(journal, name, request)
	if err != nil || !exists || previous != request {
		return false, err
	}
	file, err := journal.Open(name)
	if err != nil {
		return false, err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return false, err
	}
	return true, syncRootDirectory(journal)
}

func writeJournalAtomically(ctx context.Context, journal *os.Root, name string, request executionRecord) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	temp := name + ".tmp-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatUint(executionTempSequence.Add(1), 10)
	file, err := journal.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = journal.Remove(temp) }()
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("persist runtime execution identity: %w", writeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if err := journal.Rename(temp, name); err != nil {
		return fmt.Errorf("commit runtime execution identity: %w", err)
	}
	if err := syncRootDirectory(journal); err != nil {
		return fmt.Errorf("sync runtime execution journal: %w", err)
	}
	return ctx.Err()
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
