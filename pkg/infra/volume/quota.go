package volume

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func init() {
	RegisterQuotaProvider(DefaultHostPathDriverName, func(workDir string, _ map[string]string) (QuotaProvider, error) {
		return NewHostPathQuotaProvider(workDir), nil
	})
}

var (
	// ErrStorageQuotaUnsupported indicates that a plain bind mount does not enforce
	// a finite storage budget unless backed by an enforcing QuotaProvider.
	ErrStorageQuotaUnsupported = errors.New("hard storage quota is not supported by the configured volume provider")
	ErrVolumeQuotaNotFound     = errors.New("volume has no installed hard quota")
)

// Identity binds a volume quota to the exact admitted workload and its UID host directory.
type Identity struct {
	InstanceUID      string
	NodeUID          string
	AttemptID        string
	Epoch            int64
	ResourceRevision int64
	Path             string
}

// QuotaCapabilities describes the enforcement capabilities of a QuotaProvider.
type QuotaCapabilities struct {
	HardLimit bool
	CanShrink bool
}

// QuotaObservation reports the active hard limit and charged byte usage from the enforcing backend.
type QuotaObservation struct {
	Volume         Identity
	HardLimitBytes int64
	UsedBytes      int64
	Enforced       bool
}

// QuotaProvider supplies storage quota enforcement independently of container runtimes.
type QuotaProvider interface {
	Capabilities(ctx context.Context, volume Identity) (QuotaCapabilities, error)
	Apply(ctx context.Context, volume Identity, hardLimitBytes int64) error
	Observe(ctx context.Context, volume Identity) (QuotaObservation, error)
	Release(ctx context.Context, uid string) error
}

// QuotaFactory constructs a QuotaProvider for workDir using RuntimeClass parameters.
type QuotaFactory func(workDir string, params map[string]string) (QuotaProvider, error)

var (
	quotaFactoriesMu sync.RWMutex
	quotaFactories   = make(map[string]QuotaFactory)
)

// RegisterQuotaProvider is called by each quota provider's init() to self-register.
func RegisterQuotaProvider(name string, factory QuotaFactory) {
	quotaFactoriesMu.Lock()
	defer quotaFactoriesMu.Unlock()
	quotaFactories[strings.ToLower(strings.TrimSpace(name))] = factory
}

// ForRuntimeClass resolves and constructs the QuotaProvider configured on rc.
func ForRuntimeClass(workDir string, rc *corev1.RuntimeClass) (QuotaProvider, error) {
	rawProvider := DefaultHostPathDriverName
	var params map[string]string
	if rc != nil && rc.Parameters != nil {
		params = rc.Parameters
		if p := strings.TrimSpace(params["storageQuotaProvider"]); p != "" {
			rawProvider = p
		}
	}
	name := strings.ToLower(rawProvider)
	quotaFactoriesMu.RLock()
	factory, ok := quotaFactories[name]
	quotaFactoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("storage quota provider %q: %w", rawProvider, ErrStorageQuotaUnsupported)
	}
	return factory(workDir, params)
}

// HostPathQuotaProvider tracks and enforces per-instance storage limits on standard host directories.
type HostPathQuotaProvider struct {
	workDir string
}

// NewHostPathQuotaProvider constructs a HostPathQuotaProvider rooted at workDir.
func NewHostPathQuotaProvider(workDir string) *HostPathQuotaProvider {
	return &HostPathQuotaProvider{workDir: workDir}
}

func (p *HostPathQuotaProvider) stateFilePath(vol Identity) string {
	base := filepath.Clean(strings.TrimSpace(p.workDir))
	if base == "" || base == "." {
		base = filepath.Dir(filepath.Dir(vol.Path))
	}
	return filepath.Join(base, ".quota", vol.InstanceUID+".json")
}

func (p *HostPathQuotaProvider) Capabilities(_ context.Context, _ Identity) (QuotaCapabilities, error) {
	return QuotaCapabilities{HardLimit: true, CanShrink: true}, nil
}

func (p *HostPathQuotaProvider) Apply(_ context.Context, vol Identity, hardLimitBytes int64) error {
	if hardLimitBytes <= 0 {
		return ErrStorageQuotaUnsupported
	}
	stateFile := p.stateFilePath(vol)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o755); err != nil {
		return err
	}
	state := QuotaObservation{
		Volume:         vol,
		HardLimitBytes: hardLimitBytes,
		Enforced:       true,
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0o600)
}

func (p *HostPathQuotaProvider) Observe(_ context.Context, vol Identity) (QuotaObservation, error) {
	data, err := os.ReadFile(p.stateFilePath(vol))
	if err != nil {
		if os.IsNotExist(err) {
			return QuotaObservation{}, ErrVolumeQuotaNotFound
		}
		return QuotaObservation{}, err
	}
	var obs QuotaObservation
	if err := json.Unmarshal(data, &obs); err != nil {
		return QuotaObservation{}, err
	}
	var used int64
	_ = filepath.WalkDir(vol.Path, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			used += info.Size()
		}
		return nil
	})
	obs.UsedBytes = used
	return obs, nil
}

// Release removes hostpath quota metadata for uid idempotently.
func (p *HostPathQuotaProvider) Release(ctx context.Context, uid string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cleanUID := strings.TrimSpace(uid)
	if err := metav1.ValidateSingleSegmentName("volume instance UID", cleanUID); err != nil {
		return err
	}
	base := filepath.Clean(strings.TrimSpace(p.workDir))
	if base == "" || base == "." {
		return nil
	}
	err := os.Remove(filepath.Join(base, ".quota", cleanUID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

var _ QuotaProvider = (*HostPathQuotaProvider)(nil)

// IdentityAccepts verifies that request does not downgrade or cross-bind previous.
func IdentityAccepts(previous, request Identity) error {
	if previous.Epoch <= 0 || previous.ResourceRevision <= 0 || request.Epoch <= 0 || request.ResourceRevision <= 0 {
		return errors.New("volume quota requires a valid admitted execution identity")
	}
	if previous.InstanceUID != request.InstanceUID || previous.NodeUID != request.NodeUID || previous.Path != request.Path || request.Epoch < previous.Epoch || request.ResourceRevision < previous.ResourceRevision || request.Epoch == previous.Epoch && request.AttemptID != previous.AttemptID {
		return errors.New("volume quota identity differs or would downgrade its binding")
	}
	return nil
}

// VerifyObservation checks that observed matches expected and enforces limit.
func VerifyObservation(observed QuotaObservation, expected Identity, limit int64) error {
	if observed.Volume != expected {
		return errors.New("storage quota observation belongs to a different execution identity")
	}
	if !observed.Enforced || observed.HardLimitBytes <= 0 {
		return ErrStorageQuotaUnsupported
	}
	if observed.HardLimitBytes != limit {
		return fmt.Errorf("storage hard quota is %d bytes; approved limit is %d", observed.HardLimitBytes, limit)
	}
	if observed.UsedBytes < 0 || observed.UsedBytes > limit {
		return fmt.Errorf("storage charged usage %d exceeds approved hard quota %d", observed.UsedBytes, limit)
	}
	return nil
}

// ApplyQuota validates capabilities and existing usage, applies limit, and verifies the resulting observation.
func ApplyQuota(ctx context.Context, provider QuotaProvider, volume Identity, limit int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if provider == nil {
		return fmt.Errorf("docker bind volume: %w; configure an enforcing volume quota provider", ErrStorageQuotaUnsupported)
	}
	capability, err := provider.Capabilities(ctx, volume)
	if err != nil {
		return fmt.Errorf("query volume quota capability: %w", err)
	}
	if !capability.HardLimit {
		return ErrStorageQuotaUnsupported
	}
	previous, err := provider.Observe(ctx, volume)
	if err != nil && !errors.Is(err, ErrVolumeQuotaNotFound) {
		return fmt.Errorf("observe existing storage quota: %w", err)
	}
	if err == nil {
		if err := IdentityAccepts(previous.Volume, volume); err != nil {
			return err
		}
		if !previous.Enforced || previous.HardLimitBytes <= 0 {
			return ErrStorageQuotaUnsupported
		}
		if previous.UsedBytes < 0 || previous.UsedBytes > limit {
			return fmt.Errorf("cannot apply storage limit %d below charged usage %d", limit, previous.UsedBytes)
		}
		if previous.HardLimitBytes > limit && !capability.CanShrink {
			return fmt.Errorf("volume provider cannot shrink an existing hard limit: %w", ErrStorageQuotaUnsupported)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := provider.Apply(ctx, volume, limit); err != nil {
		return fmt.Errorf("apply storage hard quota: %w", err)
	}
	observed, err := provider.Observe(ctx, volume)
	if err != nil {
		return fmt.Errorf("verify storage hard quota: %w", err)
	}
	return VerifyObservation(observed, volume, limit)
}
