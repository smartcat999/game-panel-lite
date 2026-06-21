package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type ServerDesiredState string

const (
	DesiredRunning ServerDesiredState = "running"
	DesiredStopped ServerDesiredState = "stopped"
	DesiredDeleted ServerDesiredState = "deleted"
)

type ServerPhase string

const (
	PhasePending     ServerPhase = "pending"
	PhaseReconciling ServerPhase = "reconciling"
	PhaseRunning     ServerPhase = "running"
	PhaseStopped     ServerPhase = "stopped"
	PhaseFailed      ServerPhase = "failed"
	PhaseDeleting    ServerPhase = "deleting"
	PhaseDeleted     ServerPhase = "deleted"
)

type ServerActualState string

const (
	ActualRunning ServerActualState = "running"
	ActualStopped ServerActualState = "stopped"
	ActualMissing ServerActualState = "missing"
	ActualUnknown ServerActualState = "unknown"
)

type ServerResources struct {
	CPULimitCores float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB int     `json:"memoryLimitMb,omitempty"`
}

type ServerNetworkSpec struct {
	Port     int    `json:"port,omitempty"`
	HostPort int    `json:"hostPort,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type ServerRuntimeSpec struct {
	DataDir string   `json:"dataDir,omitempty"`
	Image   string   `json:"image,omitempty"`
	Env     []string `json:"env,omitempty"`
	Cmd     []string `json:"cmd,omitempty"`
}

type ServerSpec struct {
	Generation      int                `json:"generation"`
	DesiredState    ServerDesiredState `json:"desiredState"`
	Version         string             `json:"version,omitempty"`
	Config          map[string]any     `json:"config,omitempty"`
	SourceWorldID   string             `json:"sourceWorldId,omitempty"`
	SourceWorldName string             `json:"sourceWorldName,omitempty"`
	ModIDs          []string           `json:"modIds,omitempty"`
	Resources       ServerResources    `json:"resources,omitempty"`
	Network         ServerNetworkSpec  `json:"network,omitempty"`
	Runtime         ServerRuntimeSpec  `json:"runtime,omitempty"`
}

type ServerRuntimeStatus struct {
	Phase              ServerPhase       `json:"phase"`
	ActualState        ServerActualState `json:"actualState"`
	RuntimeID          string            `json:"runtimeId,omitempty"`
	PlayersOnline      int               `json:"playersOnline,omitempty"`
	ObservedGeneration int               `json:"observedGeneration"`
	AppliedGeneration  int               `json:"appliedGeneration"`
	Conditions         []ServerCondition `json:"conditions,omitempty"`
	LastError          string            `json:"lastError,omitempty"`
	LastReconcileAt    time.Time         `json:"lastReconcileAt,omitempty"`
	LastTransitionAt   time.Time         `json:"lastTransitionAt,omitempty"`
}

type ServerCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	ObservedGeneration int       `json:"observedGeneration,omitempty"`
	LastTransitionAt   time.Time `json:"lastTransitionAt"`
}

type GameServer struct {
	ID          string              `json:"id" gorm:"primaryKey"`
	Name        string              `json:"name"`
	GameKey     GameKey             `json:"gameKey" gorm:"index"`
	ProviderKey ProviderKey         `json:"providerKey" gorm:"index"`
	Spec        ServerSpec          `json:"spec" gorm:"serializer:json"`
	Status      ServerRuntimeStatus `json:"status" gorm:"serializer:json"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

func ServerMaxPlayers(server GameServer) int {
	value, ok := server.Spec.Config["maxPlayers"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func ServerStatusFromRuntime(desired ServerDesiredState, status ServerRuntimeStatus) ServerStatus {
	phase := status.Phase
	switch phase {
	case PhasePending, PhaseReconciling:
		switch desired {
		case DesiredStopped:
			if status.ActualState == ActualRunning {
				return StatusStopping
			}
			return StatusStopped
		case DesiredDeleted:
			return StatusDeleting
		default:
			return StatusStarting
		}
	case PhaseRunning:
		return StatusRunning
	case PhaseStopped:
		return StatusStopped
	case PhaseFailed:
		return StatusErrored
	case PhaseDeleting, PhaseDeleted:
		return StatusDeleting
	default:
		return StatusStopped
	}
}
