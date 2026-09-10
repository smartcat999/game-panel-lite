package nodeworkload

import (
	"context"
	"fmt"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

type FakeGameProvider struct {
	Artifact       string
	Metrics        []instanceobservability.MetricSample
	ConsoleHistory []string
}

func (p *FakeGameProvider) Materialize(_ context.Context, intent Intent) (Specification, error) {
	return Specification{Artifact: p.Artifact, Env: map[string]string{"GAME_VERSION": intent.GameVersion}}, nil
}

func (p *FakeGameProvider) ExecuteConsole(_ context.Context, _ RuntimeHandle, command string) error {
	p.ConsoleHistory = append(p.ConsoleHistory, command)
	return nil
}

func (p *FakeGameProvider) CollectMetrics(_ context.Context, _ RuntimeHandle, _ []providercontract.Metric, _ time.Time) ([]instanceobservability.MetricSample, error) {
	return append([]instanceobservability.MetricSample(nil), p.Metrics...), nil
}

type FakeRuntimeProvider struct {
	Name       string
	Policies   []NetworkPolicy
	BackupData map[string]BackupArtifact
}

func (p *FakeRuntimeProvider) Reconcile(_ context.Context, logicalInstanceID string, _ Specification, policy NetworkPolicy) (RuntimeResult, error) {
	p.Policies = append(p.Policies, policy)
	attempt := fmt.Sprintf("rta_%s_%d", p.Name, len(p.Policies))
	return RuntimeResult{Handle: RuntimeHandle{RuntimeAttemptID: attempt}, Logs: []instanceobservability.LogEntry{{ID: "log_" + attempt, LogicalInstanceID: logicalInstanceID, RuntimeAttemptID: attempt, Stream: "system", Message: "runtime " + p.Name + " ready", ObservedAt: time.Now().UTC()}}, Metrics: []instanceobservability.MetricSample{{ID: "met_" + attempt, LogicalInstanceID: logicalInstanceID, RuntimeAttemptID: attempt, Metric: "cpu.utilization", Value: 0.1, Unit: "ratio", Source: "platform", SampledAt: time.Now().UTC()}}}, nil
}

func (p *FakeRuntimeProvider) Backup(_ context.Context, dataScope, objectKey string) (BackupArtifact, error) {
	artifact := BackupArtifact{ObjectKey: objectKey, SizeBytes: 128, Checksums: map[string]string{"sha256": "fake"}}
	if p.BackupData == nil {
		p.BackupData = make(map[string]BackupArtifact)
	}
	p.BackupData[dataScope] = artifact
	return artifact, nil
}

func (p *FakeRuntimeProvider) Restore(_ context.Context, dataScope string, artifact BackupArtifact) error {
	if artifact.ObjectKey == "" || dataScope == "" {
		return ErrInvalidWorkload
	}
	return nil
}

type MemoryTelemetry struct {
	ByInstance map[string][]instanceobservability.Observation
}

func (s *MemoryTelemetry) Append(_ context.Context, observation instanceobservability.Observation) error {
	if s.ByInstance == nil {
		s.ByInstance = make(map[string][]instanceobservability.Observation)
	}
	s.ByInstance[observation.LogicalInstanceID] = append(s.ByInstance[observation.LogicalInstanceID], observation)
	return nil
}
