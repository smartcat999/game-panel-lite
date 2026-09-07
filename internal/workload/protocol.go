// Package workload defines the wire contract shared by the control plane and workers.
// It must not depend on database, HTTP or runtime implementations.
package workload

import "time"

const ArtifactCapability = "artifacts-v1"

type Resources struct {
	CPULimitCores float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB int     `json:"memoryLimitMb,omitempty"`
}

type Network struct {
	Port            int    `json:"port,omitempty"`
	HostPort        int    `json:"hostPort,omitempty"`
	Protocol        string `json:"protocol,omitempty"`
	AdditionalPorts []Port `json:"additionalPorts,omitempty"`
}

type Port struct {
	Port     int    `json:"port"`
	HostPort int    `json:"hostPort"`
	Protocol string `json:"protocol,omitempty"`
}

// Artifact identifies immutable bytes; transport endpoints and host paths are never carried in the descriptor.
type Artifact struct {
	Revision  int64  `json:"revision,omitempty"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Options struct {
	Artifacts  []Artifact        `json:"artifacts,omitempty"`
	Env        []string          `json:"env,omitempty"`
	Cmd        []string          `json:"cmd,omitempty"`
	Files      map[string]string `json:"files,omitempty"`
	DataMounts []string          `json:"dataMounts,omitempty"`
}

type Spec struct {
	ServerID  string    `json:"serverId"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Network   Network   `json:"network,omitempty"`
	Resources Resources `json:"resources,omitempty"`
	DataDir   string    `json:"dataDir,omitempty"`
	Options   Options   `json:"options,omitempty"`
}

type Assignment struct {
	ObservationToken  string     `json:"observationToken"`
	ID                string     `json:"id"`
	UID               string     `json:"uid"`
	ServerID          string     `json:"serverId"`
	NodeID            string     `json:"nodeId"`
	Generation        int        `json:"generation"`
	DesiredState      string     `json:"desiredState"`
	Spec              Spec       `json:"spec"`
	DeletionTimestamp *time.Time `json:"deletionTimestamp,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type Observation struct {
	LeaseHolderID            string      `json:"leaseHolderId,omitempty"`
	LeaseFence               int64       `json:"leaseFence,omitempty"`
	ObservationToken         string      `json:"observationToken"`
	ObservedGeneration       int         `json:"observedGeneration"`
	RuntimeID                string      `json:"runtimeId,omitempty"`
	ActualState              string      `json:"actualState"`
	Conditions               []Condition `json:"conditions,omitempty"`
	LastError                string      `json:"lastError,omitempty"`
	ReconcileDurationSeconds float64     `json:"reconcileDurationSeconds"`
	ObservedAt               time.Time   `json:"observedAt"`
}

type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	ObservedGeneration int       `json:"observedGeneration,omitempty"`
	LastTransitionAt   time.Time `json:"lastTransitionAt"`
}
