package runtime

import (
	"context"
	"io"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type ContainerSpec struct {
	InstanceID      string
	Name            string
	Image           string
	Port            int
	HostPort        int
	AdditionalPorts []ContainerPort
	Resources       ContainerResources
	DataDir         string
	ConfigText      string
	Options         ContainerOptions
}

type ContainerPort struct {
	Port     int
	HostPort int
	Protocol string
}

type ContainerResources struct {
	CPULimitCores float64
	MemoryLimitMB int
}

type ContainerOptions struct {
	Env          []string
	Cmd          []string
	DataMounts   []string
	Files        map[string]string
	PortProtocol string
}

type DockerStatus struct {
	Available     bool      `json:"available"`
	Message       string    `json:"message"`
	Host          string    `json:"host"`
	Architecture  string    `json:"architecture,omitempty"`
	LastCheckedAt time.Time `json:"lastCheckedAt"`
}

type WorkloadStats struct {
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryMB      int64   `json:"memoryMb"`
	MemoryLimitMB int64   `json:"memoryLimitMb"`
}

type HostStats struct {
	RunningWorkloads int     `json:"runningWorkloads"`
	CPUCores         int     `json:"cpuCores"`
	TotalCPUPercent  float64 `json:"totalCpuPercent"`
	TotalMemoryMB    int64   `json:"totalMemoryMb"`
	MemoryLimitMB    int64   `json:"memoryLimitMb"`
	StorageUsedBytes int64   `json:"storageUsedBytes"`
}

type ImagePrepareProgress struct {
	Message  string
	Progress int
}

type ImagePrepareProgressFunc func(ImagePrepareProgress)

// GameUpdateRequest contains only the runtime-owned inputs required to inspect
// or update a dedicated server installation. The executor never accepts a
// caller-provided command: each runtime implementation owns the fixed updater
// command it runs.
type GameUpdateRequest struct {
	JobID     string
	RuntimeID string
	Image     string
	DataDir   string
	AppID     string
}

type GameUpdateResult struct {
	InstalledBuildID string
	LatestBuildID    string
}

type GameUpdateProgress struct {
	Stage    string
	Progress int
	Message  string
}

type GameUpdateProgressFunc func(GameUpdateProgress)

const (
	GameUpdateStageRefreshingMetadata = "refreshing_metadata"
	GameUpdateStageValidating         = "validating"
	GameUpdateStageDownloading        = "downloading"
	GameUpdateStageInstalling         = "installing"
	GameUpdateStageFinalizing         = "finalizing"
	GameUpdateStageSucceeded          = "succeeded"
)

type GameUpdateExecutor interface {
	CheckGameUpdate(ctx context.Context, request GameUpdateRequest) (GameUpdateResult, error)
	ApplyGameUpdate(ctx context.Context, request GameUpdateRequest, onProgress GameUpdateProgressFunc) (GameUpdateResult, error)
}

type GameUpdateCleaner interface {
	CleanupGameUpdate(ctx context.Context, jobID string) error
}

type WorkloadHealth struct {
	Status         string
	HasHealthCheck bool
}

const (
	WorkloadHealthStarting  = "starting"
	WorkloadHealthHealthy   = "healthy"
	WorkloadHealthUnhealthy = "unhealthy"
)

type WorkloadHealthInspector interface {
	InspectWorkloadHealth(ctx context.Context, runtimeID string) (WorkloadHealth, error)
}

type ImageProgressPreparer interface {
	PrepareImageWithProgress(ctx context.Context, image string, onProgress ImagePrepareProgressFunc) error
}

type ImageArchiveManager interface {
	SaveImageArchive(ctx context.Context, image string, path string) error
	LoadImageArchive(ctx context.Context, path string) error
}

const (
	ImageStatusReady       = "ready"
	ImageStatusMissing     = "missing"
	ImageStatusUpdateReady = "update_available"
	ImageStatusPreparing   = "preparing"
	ImageStatusFailed      = "failed"
	ImageStatusUnsupported = "unsupported"
)

type Adapter interface {
	Check(ctx context.Context) DockerStatus
	ImageStatus(ctx context.Context, image string) domain.RuntimeImageStatus
	PrepareImage(ctx context.Context, image string) error
	HostStats(ctx context.Context) (HostStats, error)
	WorkloadAdapter
	WorkloadIOAdapter
}

type WorkloadAdapter interface {
	CreateWorkload(ctx context.Context, spec domain.WorkloadSpec) (string, error)
	StartWorkload(ctx context.Context, runtimeID string) error
	StopWorkload(ctx context.Context, runtimeID string) error
	RemoveWorkload(ctx context.Context, runtimeID string) error
	InspectWorkload(ctx context.Context, runtimeID string) (domain.WorkloadStatus, error)
}

type WorkloadIOAdapter interface {
	StatsWorkload(ctx context.Context, runtimeID string) (WorkloadStats, error)
	LogsWorkload(ctx context.Context, runtimeID string, follow bool) (io.ReadCloser, error)
	LogSnapshotWorkload(ctx context.Context, runtimeID string) (io.ReadCloser, error)
	SendCommandWorkload(ctx context.Context, runtimeID string, command string) error
}

func ContainerSpecFromWorkload(spec domain.WorkloadSpec) ContainerSpec {
	configText := ""
	files := map[string]string{}
	for name, content := range spec.Options.Files {
		if name == "serverconfig.txt" {
			configText = content
			continue
		}
		files[name] = content
	}
	return ContainerSpec{
		InstanceID: spec.ServerID,
		Name:       spec.Name,
		Image:      spec.Image,
		Port:       spec.Network.Port,
		HostPort:   spec.Network.HostPort,
		AdditionalPorts: func() []ContainerPort {
			ports := make([]ContainerPort, 0, len(spec.Network.AdditionalPorts))
			for _, port := range spec.Network.AdditionalPorts {
				ports = append(ports, ContainerPort{Port: port.Port, HostPort: port.HostPort, Protocol: port.Protocol})
			}
			return ports
		}(),
		Resources: ContainerResources{
			CPULimitCores: spec.Resources.CPULimitCores,
			MemoryLimitMB: spec.Resources.MemoryLimitMB,
		},
		DataDir:    spec.DataDir,
		ConfigText: configText,
		Options: ContainerOptions{
			Env:          append([]string{}, spec.Options.Env...),
			Cmd:          append([]string{}, spec.Options.Cmd...),
			DataMounts:   append([]string{}, spec.Options.DataMounts...),
			Files:        files,
			PortProtocol: spec.Network.Protocol,
		},
	}
}

func WorkloadSpecFromContainer(spec ContainerSpec) domain.WorkloadSpec {
	files := map[string]string{}
	if spec.ConfigText != "" {
		files["serverconfig.txt"] = spec.ConfigText
	}
	for name, content := range spec.Options.Files {
		files[name] = content
	}
	return domain.WorkloadSpec{
		ServerID: spec.InstanceID,
		Name:     spec.Name,
		Image:    spec.Image,
		DataDir:  spec.DataDir,
		Network: domain.WorkloadNetwork{
			Port:     spec.Port,
			HostPort: spec.HostPort,
			Protocol: spec.Options.PortProtocol,
			AdditionalPorts: func() []domain.WorkloadPort {
				ports := make([]domain.WorkloadPort, 0, len(spec.AdditionalPorts))
				for _, port := range spec.AdditionalPorts {
					ports = append(ports, domain.WorkloadPort{Port: port.Port, HostPort: port.HostPort, Protocol: port.Protocol})
				}
				return ports
			}(),
		},
		Resources: domain.WorkloadResources{
			CPULimitCores: spec.Resources.CPULimitCores,
			MemoryLimitMB: spec.Resources.MemoryLimitMB,
		},
		Options: domain.WorkloadOptions{
			Env:        append([]string{}, spec.Options.Env...),
			Cmd:        append([]string{}, spec.Options.Cmd...),
			DataMounts: append([]string{}, spec.Options.DataMounts...),
			Files:      files,
		},
	}
}

func WorkloadStatusFromServerStatus(runtimeID string, status domain.ServerStatus) domain.WorkloadStatus {
	return domain.WorkloadStatus{RuntimeID: runtimeID, State: serverStatusToActualState(status)}
}

func serverStatusToActualState(status domain.ServerStatus) domain.ServerActualState {
	switch status {
	case domain.StatusRunning:
		return domain.ActualRunning
	case domain.StatusStopped:
		return domain.ActualStopped
	default:
		return domain.ActualUnknown
	}
}

func ServerStatusFromWorkloadStatus(status domain.WorkloadStatus) domain.ServerStatus {
	switch status.State {
	case domain.ActualRunning:
		return domain.StatusRunning
	case domain.ActualStopped, domain.ActualMissing:
		return domain.StatusStopped
	default:
		return domain.StatusErrored
	}
}
