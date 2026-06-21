package domain

type WorkloadResources struct {
	CPULimitCores float64 `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB int     `json:"memoryLimitMb,omitempty"`
}

type WorkloadNetwork struct {
	Port     int    `json:"port,omitempty"`
	HostPort int    `json:"hostPort,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type WorkloadOptions struct {
	Env        []string          `json:"env,omitempty"`
	Cmd        []string          `json:"cmd,omitempty"`
	Files      map[string]string `json:"files,omitempty"`
	DataMounts []string          `json:"dataMounts,omitempty"`
}

type WorkloadSpec struct {
	ServerID  string            `json:"serverId"`
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Network   WorkloadNetwork   `json:"network,omitempty"`
	Resources WorkloadResources `json:"resources,omitempty"`
	DataDir   string            `json:"dataDir,omitempty"`
	Options   WorkloadOptions   `json:"options,omitempty"`
}

type ProviderRuntimeConfig struct {
	Port       int             `json:"port,omitempty"`
	Protocol   string          `json:"protocol,omitempty"`
	ConfigText string          `json:"configText,omitempty"`
	Options    WorkloadOptions `json:"options,omitempty"`
}

type WorkloadStatus struct {
	RuntimeID string            `json:"runtimeId,omitempty"`
	State     ServerActualState `json:"state"`
	Message   string            `json:"message,omitempty"`
}
