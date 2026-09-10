package nodeworkload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

var (
	ErrUnsupportedCapability = errors.New("provider capability is not declared")
	ErrInvalidWorkload       = errors.New("invalid provider workload")
)

type Intent struct {
	WorkspaceID          string
	LogicalInstanceID    string
	RegionID             string
	RegionalDeploymentID string
	ProviderReleaseID    string
	GameVersion          string
	ApplyBehavior        string
	Configuration        map[string]any
	ModLock              []contract.ModLockEntry
	DataScope            string
}

type Specification struct {
	Artifact string
	Args     []string
	Env      map[string]string
}

type RuntimeHandle struct {
	RuntimeAttemptID string
}

type RuntimeResult struct {
	Handle  RuntimeHandle
	Logs    []instanceobservability.LogEntry
	Metrics []instanceobservability.MetricSample
}

type BackupArtifact struct {
	ObjectKey string
	SizeBytes int64
	Checksums map[string]string
}

type NetworkPolicy struct {
	InternetEgressAllowed bool
	DeniedManagementCIDRs []string
}

type GameProvider interface {
	Materialize(context.Context, Intent) (Specification, error)
	ExecuteConsole(context.Context, RuntimeHandle, string) error
	CollectMetrics(context.Context, RuntimeHandle, []providercontract.Metric, time.Time) ([]instanceobservability.MetricSample, error)
}

type RuntimeProvider interface {
	Reconcile(context.Context, string, Specification, NetworkPolicy) (RuntimeResult, error)
	Backup(context.Context, string, string) (BackupArtifact, error)
	Restore(context.Context, string, BackupArtifact) error
}

type TelemetrySink interface {
	Append(context.Context, instanceobservability.Observation) error
}

type Module struct {
	game      GameProvider
	runtime   RuntimeProvider
	telemetry TelemetrySink
	policy    NetworkPolicy
}

func New(game GameProvider, runtime RuntimeProvider, telemetry TelemetrySink, managementCIDRs []string) *Module {
	return &Module{game: game, runtime: runtime, telemetry: telemetry, policy: NetworkPolicy{InternetEgressAllowed: true, DeniedManagementCIDRs: append([]string(nil), managementCIDRs...)}}
}

func (m *Module) Reconcile(ctx context.Context, manifest providercontract.Manifest, intent Intent, sequence int64, now time.Time) (RuntimeHandle, error) {
	if intent.WorkspaceID == "" || intent.LogicalInstanceID == "" || intent.RegionID == "" || intent.RegionalDeploymentID == "" || intent.ProviderReleaseID != manifest.ProviderReleaseID || intent.GameVersion == "" || intent.ApplyBehavior != "hot-reload" && intent.ApplyBehavior != "restart-required" && intent.ApplyBehavior != "recreate-required" || intent.DataScope != "instances/"+intent.LogicalInstanceID || sequence < 1 {
		return RuntimeHandle{}, ErrInvalidWorkload
	}
	specification, err := m.game.Materialize(ctx, intent)
	if err != nil {
		return RuntimeHandle{}, err
	}
	result, err := m.runtime.Reconcile(ctx, intent.LogicalInstanceID, specification, m.policy)
	if err != nil {
		return RuntimeHandle{}, err
	}
	metrics := append([]instanceobservability.MetricSample(nil), result.Metrics...)
	if slices.Contains(manifest.Capabilities, "game-metrics") {
		gameMetrics, err := m.game.CollectMetrics(ctx, result.Handle, manifest.Metrics, now)
		if err != nil {
			return RuntimeHandle{}, err
		}
		metrics = append(metrics, gameMetrics...)
	}
	observation := instanceobservability.Observation{MessageID: telemetryMessageID(intent.LogicalInstanceID, result.Handle.RuntimeAttemptID, sequence), WorkspaceID: intent.WorkspaceID, LogicalInstanceID: intent.LogicalInstanceID, RegionID: intent.RegionID, RegionalDeploymentID: intent.RegionalDeploymentID, RuntimeAttemptID: result.Handle.RuntimeAttemptID, Sequence: sequence, Logs: result.Logs, Metrics: metrics, ObservedAt: now}
	if err := m.telemetry.Append(ctx, observation); err != nil {
		return RuntimeHandle{}, err
	}
	return result.Handle, nil
}

func telemetryMessageID(instanceID, attemptID string, sequence int64) string {
	digest := sha256.Sum256([]byte(instanceID + "|" + attemptID + "|" + time.Unix(sequence, 0).UTC().Format(time.RFC3339Nano)))
	return "msg_" + hex.EncodeToString(digest[:12])
}

func (m *Module) Console(ctx context.Context, manifest providercontract.Manifest, handle RuntimeHandle, command string) error {
	if !slices.Contains(manifest.Capabilities, "console") || command == "" || len(command) > 4096 {
		return ErrUnsupportedCapability
	}
	return m.game.ExecuteConsole(ctx, handle, command)
}

func (m *Module) Backup(ctx context.Context, manifest providercontract.Manifest, logicalInstanceID, objectKey string) (BackupArtifact, error) {
	if !slices.Contains(manifest.Capabilities, "backup") {
		return BackupArtifact{}, ErrUnsupportedCapability
	}
	return m.runtime.Backup(ctx, "instances/"+logicalInstanceID, objectKey)
}

func (m *Module) Restore(ctx context.Context, manifest providercontract.Manifest, logicalInstanceID string, artifact BackupArtifact) error {
	if !slices.Contains(manifest.Capabilities, "backup") {
		return ErrUnsupportedCapability
	}
	return m.runtime.Restore(ctx, "instances/"+logicalInstanceID, artifact)
}
