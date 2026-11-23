package v1

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

const (
	GroupName    = "core.gamepanel.io"
	GroupVersion = "core.gamepanel.io/v1"
)

// TenantType distinguishes personal spaces from multi-member organizations.
type TenantType string

const (
	TenantTypePersonal     TenantType = "Personal"
	TenantTypeOrganization TenantType = "Organization"
)

type TenantPhase string

const (
	TenantPhaseActive      TenantPhase = "Active"
	TenantPhaseSuspended   TenantPhase = "Suspended"
	TenantPhaseTerminating TenantPhase = "Terminating"
)

// QuotaPolicyChangeKind identifies why a quota policy ledger entry was recorded.
type QuotaPolicyChangeKind string

const (
	QuotaPolicyChangeKindBootstrap   QuotaPolicyChangeKind = "Bootstrap"
	QuotaPolicyChangeKindAdminAdjust QuotaPolicyChangeKind = "AdminAdjust"
	QuotaPolicyChangeKindExpiry      QuotaPolicyChangeKind = "Expiry"
)

// QuotaPolicySnapshot renders an effective quota policy state in API responses.
type QuotaPolicySnapshot struct {
	Quota          ResourceList   `json:"quota,omitempty"`
	UnlimitedQuota []ResourceName `json:"unlimitedQuota,omitempty"`
	ExpiresAt      *time.Time     `json:"expiresAt,omitempty"`
}

// QuotaPolicyChange is a single committed entry in a Tenant's quota policy ledger.
type QuotaPolicyChange struct {
	metav1.TypeMeta       `json:",inline"`
	Metadata              metav1.ObjectMeta     `json:"metadata"`
	ChangeKind            QuotaPolicyChangeKind `json:"changeKind"`
	FromVersion           int64                 `json:"fromVersion"`
	ToVersion             int64                 `json:"toVersion"`
	TenantResourceVersion int64                 `json:"tenantResourceVersion"`
	Old                   QuotaPolicySnapshot   `json:"old"`
	Target                QuotaPolicySnapshot   `json:"target"`
}

// QuotaPolicyInvariantCheck reports the result of one ledger invariant check.
type QuotaPolicyInvariantCheck struct {
	ID      string `json:"id"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// QuotaPolicyVerification summarizes ledger chain integrity checks for a Tenant.
type QuotaPolicyVerification struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta           `json:"metadata"`
	PolicyVersion   int64                       `json:"policyVersion"`
	UpstreamVersion int64                       `json:"upstreamVersion"`
	CheckedChanges  int                         `json:"checkedChanges"`
	Valid           bool                        `json:"valid"`
	Checks          []QuotaPolicyInvariantCheck `json:"checks"`
}

// Tenant represents an isolated resource and quota boundary (Personal or Organization).
type Tenant struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            TenantSpec        `json:"spec"`
	Status          TenantStatus      `json:"status,omitempty"`
}

type TenantSpec struct {
	DisplayName    string         `json:"displayName"`
	Type           TenantType     `json:"type"`
	Owner          string         `json:"owner"`
	Quota          ResourceList   `json:"quota,omitempty"`
	UnlimitedQuota []ResourceName `json:"unlimitedQuota,omitempty"`
}

type TenantStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	Phase              TenantPhase        `json:"phase"`
	Reserved           ResourceList       `json:"reserved,omitempty"`
	Usage              ResourceList       `json:"usage,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

func (t *Tenant) GetTypeMeta() *metav1.TypeMeta     { return &t.TypeMeta }
func (t *Tenant) GetObjectMeta() *metav1.ObjectMeta { return &t.Metadata }

func (t *Tenant) DefaultMeta() {
	t.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "Tenant"}
	if t.Spec.Type == "" {
		t.Spec.Type = TenantTypeOrganization
	}
	if t.Spec.Quota == nil {
		t.Spec.Quota = ResourceList{}
	}
	if len(t.Spec.Quota) == 0 && len(t.Spec.UnlimitedQuota) == 0 {
		t.Spec.UnlimitedQuota = []ResourceName{
			ResourceCPU,
			ResourceMemory,
			ResourceStorage,
			ResourceInstances,
			ResourceBackups,
			ResourceBackupStorage,
		}
	}
	if t.Status.Phase == "" {
		t.Status.Phase = TenantPhaseActive
	}
}

func (t *Tenant) Validate() error {
	if err := metav1.ValidateSingleSegmentName("tenant metadata.name", t.Metadata.Name); err != nil {
		return err
	}
	if t.Metadata.Name == "instances" || t.Metadata.Name == "unassigned" {
		return fmt.Errorf("tenant metadata.name %q is reserved", t.Metadata.Name)
	}
	switch t.Spec.Type {
	case TenantTypePersonal, TenantTypeOrganization:
	default:
		return fmt.Errorf("invalid tenant spec.type %q", t.Spec.Type)
	}
	if strings.TrimSpace(t.Spec.Owner) == "" {
		return errors.New("tenant spec.owner is required")
	}
	return t.Spec.ValidateQuota()
}

// RuntimeClass defines a cluster-scoped runtime class (modeled after K8s RuntimeClass / StorageClass / IngressClass).
type RuntimeClass struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta       `json:"metadata"`
	Handler         string                  `json:"handler"`
	Parameters      map[string]string       `json:"parameters,omitempty"`
	Scheduling      *RuntimeClassScheduling `json:"scheduling,omitempty"`
	IsDefault       bool                    `json:"isDefault,omitempty"`
}

type RuntimeClassScheduling struct {
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

func (rc *RuntimeClass) GetTypeMeta() *metav1.TypeMeta     { return &rc.TypeMeta }
func (rc *RuntimeClass) GetObjectMeta() *metav1.ObjectMeta { return &rc.Metadata }

func (rc *RuntimeClass) DefaultMeta() {
	rc.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "RuntimeClass"}
}

func (rc *RuntimeClass) Validate() error {
	if err := metav1.ValidateSingleSegmentName("runtimeclass metadata.name", rc.Metadata.Name); err != nil {
		return err
	}
	if strings.TrimSpace(rc.Handler) == "" {
		return errors.New("runtimeclass handler is required")
	}
	return nil
}

// NodePhase describes the lifecycle phase of a compute Node.
type NodePhase string

const (
	NodePhaseReady    NodePhase = "Ready"
	NodePhaseNotReady NodePhase = "NotReady"

	NodeAddressInternalIP = "InternalIP"
	NodeAddressExternalIP = "ExternalIP"
	NodeAddressHostname   = "Hostname"
)

type NodeAddress struct {
	Type    string `json:"type"` // ExternalIP | InternalIP
	Address string `json:"address"`
}

type NodeRuntime struct {
	Handler string `json:"handler"` // docker | kubernetes | cloud | memory
	Version string `json:"version,omitempty"`
}

// Node represents a compute node running node-agent.
type Node struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            NodeSpec          `json:"spec"`
	Status          NodeStatus        `json:"status,omitempty"`
}

type NodeSpec struct {
	Unschedulable bool     `json:"unschedulable"`
	ExternalIP    string   `json:"externalIP,omitempty"`
	AllowedApps   []string `json:"allowedApps,omitempty"`
}

// NormalizeAllowedApps trims whitespace, drops empty entries, and deduplicates app IDs preserving order.
func NormalizeAllowedApps(apps []string) []string {
	if len(apps) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(apps))
	out := make([]string, 0, len(apps))
	for _, raw := range apps {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type NodeUsage struct {
	CPUMillicores        int64 `json:"cpuMillicores"`
	MemoryBytes          int64 `json:"memoryBytes"`
	StorageBytes         int64 `json:"storageBytes"`
	NetworkRxBytesPerSec int64 `json:"networkRxBytesPerSec,omitempty"`
	NetworkTxBytesPerSec int64 `json:"networkTxBytesPerSec,omitempty"`
}

type NodeStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	Phase              NodePhase          `json:"phase"`
	Addresses          []NodeAddress      `json:"addresses,omitempty"`
	Runtimes           []NodeRuntime      `json:"runtimes,omitempty"`
	Capacity           ResourceList       `json:"capacity,omitempty"`
	Allocatable        ResourceList       `json:"allocatable,omitempty"`
	Reserved           ResourceList       `json:"reserved,omitempty"`
	Usage              *NodeUsage         `json:"usage,omitempty"`
	CachedArtifacts    []string           `json:"cachedArtifacts,omitempty"`
	CachedBlobs        []string           `json:"cachedBlobs,omitempty"`
	WarmImages         []string           `json:"warmImages,omitempty"`
	LastHeartbeatTime  *time.Time         `json:"lastHeartbeatTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// NodeMetrics represents the /metrics subresource of a Node.
type NodeMetrics struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta      `json:"metadata"`
	Samples         []InstanceMetricSample `json:"samples"`
	Capacity        ResourceList           `json:"capacity,omitempty"`
	Allocatable     ResourceList           `json:"allocatable,omitempty"`
	Usage           *NodeUsage             `json:"usage,omitempty"`
}

func (n *Node) GetTypeMeta() *metav1.TypeMeta     { return &n.TypeMeta }
func (n *Node) GetObjectMeta() *metav1.ObjectMeta { return &n.Metadata }

func (n *Node) DefaultMeta() {
	n.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "Node"}
	if n.Status.Phase == "" {
		n.Status.Phase = NodePhaseReady
	}
	n.Spec.AllowedApps = NormalizeAllowedApps(n.Spec.AllowedApps)
}

func (n *Node) Validate() error {
	if err := metav1.ValidateSingleSegmentName("node metadata.name", n.Metadata.Name); err != nil {
		return err
	}
	for _, app := range n.Spec.AllowedApps {
		if err := metav1.ValidateSingleSegmentName("node spec.allowedApps", app); err != nil {
			return err
		}
	}
	if err := n.Status.Capacity.Validate(); err != nil {
		return fmt.Errorf("node status.capacity: %w", err)
	}
	if err := n.Status.Allocatable.Validate(); err != nil {
		return fmt.Errorf("node status.allocatable: %w", err)
	}
	return nil
}

// AllowsApp reports whether a Node permits scheduling and prewarming the given App ID.
// When spec.allowedApps is empty, all Apps are permitted.
func (n *Node) AllowsApp(appName string) bool {
	if n == nil || len(n.Spec.AllowedApps) == 0 {
		return true
	}
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return true
	}
	return slices.Contains(n.Spec.AllowedApps, appName)
}

// AllowsInstance reports whether a Node permits scheduling the given Instance based on its App label.
func (n *Node) AllowsInstance(inst *Instance) bool {
	if inst == nil {
		return true
	}
	appName := strings.TrimSpace(inst.Metadata.Labels[LabelInstanceApp])
	if appName == "" {
		appName = strings.TrimSpace(inst.Metadata.Labels[LabelInstancePreset])
	}
	return n.AllowsApp(appName)
}

// SupportsHandler checks whether the Node reports support for the given runtime handler.
func (n *Node) SupportsHandler(handler string) bool {
	for _, rt := range n.Status.Runtimes {
		if rt.Handler == handler {
			return true
		}
	}
	return false
}

// InstanceStateValue represents the desired running state of an Instance in spec.state.
type InstanceStateValue string

const (
	InstanceStateRunning InstanceStateValue = "Running"
	InstanceStateStopped InstanceStateValue = "Stopped"
	InstanceStateRestart InstanceStateValue = "Restart"

	AnnotationRestartRequestedAt = "core.gamepanel.io/restart-requested-at"
)

// InstancePhase represents the observed lifecycle phase of an Instance.
type InstancePhase string

const (
	InstancePhasePending     InstancePhase = "Pending"
	InstancePhaseScheduled   InstancePhase = "Scheduled"
	InstancePhaseStarting    InstancePhase = "Starting"
	InstancePhaseRunning     InstancePhase = "Running"
	InstancePhaseStopping    InstancePhase = "Stopping"
	InstancePhaseStopped     InstancePhase = "Stopped"
	InstancePhaseFailed      InstancePhase = "Failed"
	InstancePhaseTerminating InstancePhase = "Terminating"
	// InstancePhaseUnknown means the hosting node stopped reporting, so the real state is unknown.
	InstancePhaseUnknown InstancePhase = "Unknown"
	// InstancePhaseDeleted is recorded in metering/audit history when an Instance is physically deleted.
	InstancePhaseDeleted InstancePhase = "Deleted"
)

const (
	FinalizerInstanceCleanup = "core.gamepanel.io/instance-cleanup"
	FinalizerTenantCleanup   = "core.gamepanel.io/tenant-cleanup"
	LabelTopologyRegion      = "topology.gamepanel.io/region"
	LabelKubernetesHostname  = "kubernetes.io/hostname"

	ProtocolTCP = "TCP"
	ProtocolUDP = "UDP"

	DefaultDockerDriverName   = "docker"
	DefaultHostPathDriverName = "hostpath"
	DefaultHostPortDriverName = "hostport"

	BackupPhasePendingQuota  = "PendingQuota"
	BackupPhaseReady         = "Ready"
	BackupPhaseUploading     = "Uploading"
	BackupPhaseUnknown       = "Unknown"
	BackupPhaseInspecting    = "Inspecting"
	BackupPhaseInspectFailed = "InspectFailed"
	BackupPhaseCompleted     = "Completed"
	BackupPhaseRestoring     = "Restoring"
	BackupPhaseDeleting      = "Deleting"
	BackupPhaseDeleted       = "Deleted"

	BackupStageQuota       = "Quota"
	BackupStagePreHook     = "PreHook"
	BackupStageArchiving   = "Archiving"
	BackupStageUploading   = "Uploading"
	BackupStageInspecting  = "Inspecting"
	BackupStageStopping    = "Stopping"
	BackupStageDownloading = "Downloading"
	BackupStageExtracting  = "Extracting"
	BackupStageRestarting  = "Restarting"
	BackupStageSettling    = "Settling"
	BackupStageCompleted   = "Completed"
	BackupStageFailed      = "Failed"

	RestoreResultRunning   = "Running"
	RestoreResultSucceeded = "Succeeded"
	RestoreResultFailed    = "Failed"

	BackupInspectPhasePending   = "Pending"
	BackupInspectPhaseCompleted = "Completed"
	BackupInspectPhaseFailed    = "Failed"

	CurrentBackupInspectSchemaVersion = 3

	FileActionList                 = "list"
	FileActionRead                 = "read"
	FileActionWrite                = "write"
	FileActionMkdir                = "mkdir"
	FileActionRename               = "rename"
	FileActionDelete               = "delete"
	FileActionBackup               = "backup"
	FileActionRestore              = "restore"
	FileActionInspectBackup        = "inspect_backup"
	FileActionDownloadBackup       = "download_backup"
	FileActionUploadBackup         = "upload_backup"
	FileActionCompleteUploadBackup = "complete_upload_backup"
	FileActionDeleteBackup         = "delete_backup"

	ArchiveFormatTarGz = "tar.gz"
	ArchiveFormatZip   = "zip"

	ReservedWorkspacePrefix = ".gp-"

	ScheduleActionCommand = "command"
	ScheduleActionBackup  = "backup"
	ScheduleActionRestart = "restart"

	ScheduleResultRunning   = "Running"
	ScheduleResultUnknown   = "Unknown"
	ScheduleResultSucceeded = "Succeeded"
	ScheduleResultFailed    = "Failed"

	ScheduleStageQueued             = "Queued"
	ScheduleStageExecutingCommand   = "ExecutingCommand"
	ScheduleStageArchivingBackup    = "ArchivingBackup"
	ScheduleStageRestartingInstance = "RestartingInstance"
	ScheduleStageCompleted          = "Completed"
	ScheduleStageFailed             = "Failed"

	TaskStreamEventLog      = "log"
	TaskStreamEventProgress = "progress"
	TaskStreamEventResult   = "result"
	TaskStreamEventError    = "error"

	MetricModeRegexValue = "regex_value"
	MetricModeRegexCount = "regex_count"
	MetricModeLogDelta   = "log_delta"

	MetricSourceStdinLog = "stdin_log"
	MetricSourceExec     = "exec"
	MetricSourceFile     = "file"

	ReasonAgentHealthy              = "AgentHealthy"
	ReasonHeartbeatTimeout          = "HeartbeatTimeout"
	ReasonRuntimeResponsive         = "RuntimeResponsive"
	ReasonRuntimeOperationFailed    = "RuntimeOperationFailed"
	ReasonClockSynced               = "ClockSynced"
	ReasonClockSkewDetected         = "ClockSkewDetected"
	ReasonTenantActive              = "TenantActive"
	ReasonTenantSuspended           = "TenantSuspended"
	ReasonInstanceSuspendedByTenant = "SuspendedByTenant"
	ReasonNodeNotReady              = "NodeNotReady"
	ReasonCleanupSucceeded          = "CleanupSucceeded"
	ReasonReconcileError            = "ReconcileError"
	ReasonPullingImage              = "PullingImage"
	ReasonSyncingPackage            = "SyncingPackage"
	ReasonStarting                  = "Starting"
	ReasonRestarting                = "Restarting"
	ReasonRestartPending            = "RestartPending"
	ReasonContainerRunning          = "ContainerRunning"
	ReasonContainerStopped          = "ContainerStopped"
	ReasonContainerStillRunning     = "ContainerStillRunning"
	ReasonContainerNotRunning       = "ContainerNotRunning"
	ReasonContainerRestarting       = "ContainerRestarting"
	ReasonContainerStateMissing     = "ContainerStateMissing"
	ReasonOOMKilled                 = "OOMKilled"
	ReasonPrewarmPending            = "PrewarmPending"
	ReasonPrewarmRunning            = "PrewarmRunning"
	ReasonPrewarmSucceeded          = "PrewarmSucceeded"
	ReasonPrewarmFailed             = "PrewarmFailed"

	KindPrewarmJob              = "PrewarmJob"
	KindQuotaPolicyChange       = "QuotaPolicyChange"
	KindQuotaPolicyChangeList   = "QuotaPolicyChangeList"
	KindQuotaPolicyVerification = "QuotaPolicyVerification"

	ResourceTenantQuota = "tenants/quota"
	SubresourceState    = "state"

	AddonInstallModeFlatDir      = "flat_dir"
	AddonInstallModeTModLatest   = "tmod_latest"
	AddonInstallModeDSTLocalMods = "dst_local_mods"
	AddonInstallModeWorkshopTree = "workshop_tree"
	AddonInstallModeBepInEx      = "bepinex"
	AddonInstallModeOxide        = "oxide"
	AddonInstallModeSML          = "sml"
	AddonInstallModeCS2Plugin    = "cs2_plugin"
	AddonInstallModeCS2Workshop  = "cs2_workshop"

	// Instance → App identity (used by instance rendering and cache-affinity scheduling).
	LabelInstanceApp             = "gamepanel.io/app"
	LabelInstancePreset          = "gamepanel.io/preset"
	AnnotationInstanceAppConfig  = "core.gamepanel.io/app-config"
	AnnotationInstanceAppVersion = "core.gamepanel.io/app-version"
	AnnotationAppBuiltInHash     = "core.gamepanel.io/builtin-hash"

	// Runtime container execution identity and metadata labels.
	LabelRuntimeInstanceUID      = "gamepanel.io/instance-uid"
	LabelRuntimeNodeUID          = "gamepanel.io/node-uid"
	LabelRuntimeAttemptID        = "gamepanel.io/attempt-id"
	LabelRuntimeEpoch            = "gamepanel.io/epoch"
	LabelRuntimeResourceRevision = "gamepanel.io/resource-revision"
	LabelRuntimeGeneration       = "gamepanel.io/generation"
	LabelRuntimeTenant           = "gamepanel.io/tenant"
	LabelRuntimeInstanceName     = "gamepanel.io/instance-name"
	LabelRuntimeSpecHash         = "gamepanel.io/spec-hash"
	LabelRuntimeRole             = "gamepanel.io/role"
	LabelRuntimeTask             = "gamepanel.io/task"
	LabelRuntimeNodeShell        = "gamepanel.io/node-shell"

	DefaultNodeShellContainerName = "gamepanel-node-shell"

	// App marketplace import labels and annotations.
	LabelAppCategory     = "gamepanel.io/category"
	LabelAppSource       = "gamepanel.io/source"
	AnnotationEggVersion = "core.gamepanel.io/egg-version"
	AnnotationEggAuthor  = "core.gamepanel.io/egg-author"

	// Declarative default-version prewarm management constants.
	AnnotationAutoPrewarmDefaults = "core.gamepanel.io/auto-prewarm-defaults"
	LabelManagedBy                = "gamepanel.io/managed-by"
	ManagedByAutoDefault          = "auto-default"
	ManagedByLegacyAppVersions    = "app-versions"
	ManagedByLegacyAppAddons      = "app-addons"
	PrewarmSyncDefaultsName       = "__auto_default_apps__"
	InstanceSyncAppMetricsPrefix  = "__app_metrics__/"
)

type InstancePort struct {
	Name          string `json:"name"`
	Protocol      string `json:"protocol"` // TCP | UDP
	TargetPort    int    `json:"targetPort"`
	PublishedPort int    `json:"publishedPort,omitempty"`
}

type InstanceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type InstanceEndpoint struct {
	HostIP string         `json:"hostIP,omitempty"`
	Ports  []InstancePort `json:"ports,omitempty"`
}

// Instance is the unified workload resource referencing a cluster RuntimeClass via spec.runtimeClassName.
type Instance struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            InstanceSpec      `json:"spec"`
	Status          InstanceStatus    `json:"status,omitempty"`
}

type InstanceBackupPolicy struct {
	Enabled            bool     `json:"enabled"`
	IntervalHours      int      `json:"intervalHours,omitempty"`
	RetentionCount     int      `json:"retentionCount,omitempty"`
	Subtree            string   `json:"subtree,omitempty"`
	AnchorFiles        []string `json:"anchorFiles,omitempty"`
	ReplaceOnRestore   bool     `json:"replaceOnRestore,omitempty"`
	BackupRequiresStop bool     `json:"backupRequiresStop,omitempty"`
	ConsoleDelivery    string   `json:"consoleDelivery,omitempty"`
	PreBackupCommand   string   `json:"preBackupCommand,omitempty"`
	PostBackupCommand  string   `json:"postBackupCommand,omitempty"`
}

// InstanceSchedule defines a declarative periodic maintenance task on an Instance.
type InstanceSchedule struct {
	ID              string     `json:"id"`                        // unique schedule id, e.g. "sched-save-30m"
	Name            string     `json:"name"`                      // display title, e.g. "每 30 分钟自动保存世界"
	Action          string     `json:"action"`                    // "command" | "restart" | "backup"
	Command         string     `json:"command,omitempty"`         // console command (or newline-separated commands) for "command" or pre-restart notice
	IntervalMinutes int        `json:"intervalMinutes,omitempty"` // periodic interval in minutes (e.g. 30, 60, 360)
	DailyAt         string     `json:"dailyAt,omitempty"`         // optional fixed daily time "HH:MM" in local CST (e.g. "04:00")
	RetainCount     int        `json:"retainCount,omitempty"`     // max backups to retain when action == "backup"
	Enabled         bool       `json:"enabled"`
	ManualTriggerAt *time.Time `json:"manualTriggerAt,omitempty"` // requests immediate execution on the next reconcile
}

// InstanceScheduleStatus records the last execution state of a declarative InstanceSchedule.
type InstanceScheduleStatus struct {
	ID         string     `json:"id"`
	RunID      string     `json:"runID,omitempty"`
	ClaimedAt  *time.Time `json:"claimedAt,omitempty"`
	LastRunAt  *time.Time `json:"lastRunAt,omitempty"`
	LastResult string     `json:"lastResult,omitempty"` // "Running" | "Unknown" | "Succeeded" | "Failed"
	LastOutput string     `json:"lastOutput,omitempty"`
	Stage      string     `json:"stage,omitempty"`
	Progress   int32      `json:"progress,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// AppMetricItemAction defines an optional quick action for an item in a list-bearing metric (e.g. Kick player).
type AppMetricItemAction struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Command string `json:"command"`          // template using {{item}}, e.g. "kick {{item}}"
	Danger  bool   `json:"danger,omitempty"` // whether to style as a destructive action
}

// AppMetricSpec defines a declarative custom game metric and how it is collected from logs, exec, or files.
type AppMetricSpec struct {
	ID             string                `json:"id"`                       // e.g. "online_players", "world_cycles", "server_tps"
	Label          string                `json:"label"`                    // e.g. "在线玩家", "世界天数", "服务端 TPS"
	Unit           string                `json:"unit,omitempty"`           // e.g. "人", "天", "TPS"
	Mode           string                `json:"mode"`                     // regex_value | regex_count | log_delta
	Source         string                `json:"source,omitempty"`         // stdin_log (default) | exec | file
	FilePath       string                `json:"filePath,omitempty"`       // relative file path inside workingDir when source == "file"
	ProbeCommand   string                `json:"probeCommand,omitempty"`   // optional command sent by node-agent to trigger output
	IntervalSec    int                   `json:"intervalSec,omitempty"`    // optional active probe interval in seconds (default 15s)
	Pattern        string                `json:"pattern,omitempty"`        // regex for regex_value ($1) or regex_count
	IncPattern     string                `json:"incPattern,omitempty"`     // regex for log_delta increment (e.g. player joined)
	DecPattern     string                `json:"decPattern,omitempty"`     // regex for log_delta decrement (e.g. player left)
	ResetPattern   string                `json:"resetPattern,omitempty"`   // regex that resets log_delta counter to 0 (e.g. boot banner)
	MaxConfigKey   string                `json:"maxConfigKey,omitempty"`   // optional configSchema field name for upper bound display
	MaxValue       string                `json:"maxValue,omitempty"`       // resolved upper bound value (e.g. "20")
	ItemPattern    string                `json:"itemPattern,omitempty"`    // optional regex extracting list items (e.g. player ID & name)
	BindParam      string                `json:"bindParam,omitempty"`      // parameter name in consoleActions bound to each item (e.g. "player")
	QuickActionIDs []string              `json:"quickActionIds,omitempty"` // consoleActions IDs surfaced as 1-click buttons on each item row
	ItemActions    []AppMetricItemAction `json:"itemActions,omitempty"`    // optional inline quick actions for each extracted item
}

// InstanceCustomMetric represents a collected point-in-time custom game metric (scalar + optional entity items).
type InstanceCustomMetric struct {
	ID             string                `json:"id"`
	Label          string                `json:"label"`
	Unit           string                `json:"unit,omitempty"`
	Value          float64               `json:"value"`
	Display        string                `json:"display"`
	Max            string                `json:"max,omitempty"`
	Items          []string              `json:"items,omitempty"`
	BindParam      string                `json:"bindParam,omitempty"`
	QuickActionIDs []string              `json:"quickActionIds,omitempty"`
	ItemActions    []AppMetricItemAction `json:"itemActions,omitempty"`
}

// ArtifactRef pins an immutable versioned artifact in registry.gamepanel.io/v1.
type ArtifactRef struct {
	Repository string `json:"repository"`
	Package    string `json:"package"`
	Version    string `json:"version"`
	TargetPath string `json:"targetPath,omitempty"`
}

// Coordinate returns the canonical <repository>.<package>.<version> artifact name.
func (r *ArtifactRef) Coordinate() string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s", strings.TrimSpace(r.Repository), strings.TrimSpace(r.Package), strings.TrimSpace(r.Version))
}

func (r *ArtifactRef) Validate(fieldPrefix string) error {
	if r == nil {
		return nil
	}
	if err := metav1.ValidateSingleSegmentName(fieldPrefix+".repository", r.Repository); err != nil {
		return err
	}
	if err := metav1.ValidateSingleSegmentName(fieldPrefix+".package", r.Package); err != nil {
		return err
	}
	if strings.TrimSpace(r.Version) == "" || strings.Contains(r.Version, "/") {
		return fmt.Errorf("%s.version is required and must not contain '/'", fieldPrefix)
	}
	if tp := strings.TrimSpace(r.TargetPath); tp != "" && tp != "." {
		if err := metav1.ValidateRelativeFilePath(tp); err != nil {
			return fmt.Errorf("%s.targetPath: %w", fieldPrefix, err)
		}
	}
	return nil
}

// InstanceAddon pins a specific addon/mod/plugin version in registry.gamepanel.io/v1.
type InstanceAddon struct {
	Repository  string            `json:"repository"`
	Package     string            `json:"package"`
	Version     string            `json:"version"`
	Title       string            `json:"title,omitempty"`
	InstallMode string            `json:"installMode,omitempty"`
	TargetDir   string            `json:"targetDir,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
}

// Coordinate returns the canonical <repository>.<package>.<version> artifact name.
func (a *InstanceAddon) Coordinate() string {
	if a == nil {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s", strings.TrimSpace(a.Repository), strings.TrimSpace(a.Package), strings.TrimSpace(a.Version))
}

func (a *InstanceAddon) Validate(idx int) error {
	prefix := fmt.Sprintf("instance spec.addons[%d]", idx)
	if err := metav1.ValidateSingleSegmentName(prefix+".repository", a.Repository); err != nil {
		return err
	}
	if err := metav1.ValidateSingleSegmentName(prefix+".package", a.Package); err != nil {
		return err
	}
	if strings.TrimSpace(a.Version) == "" || strings.Contains(a.Version, "/") {
		return fmt.Errorf("%s.version is required and must not contain '/'", prefix)
	}
	if mode := strings.TrimSpace(a.InstallMode); mode != "" && !validAddonInstallMode(mode) {
		return fmt.Errorf("invalid %s.installMode %q", prefix, a.InstallMode)
	}
	if td := strings.TrimSpace(a.TargetDir); td != "" && td != "." {
		if err := metav1.ValidateRelativeFilePath(td); err != nil {
			return fmt.Errorf("%s.targetDir: %w", prefix, err)
		}
	}
	return validateAddonConfig(prefix+".config", a.Config)
}

func validateAddonConfig(prefix string, cfg map[string]string) error {
	if len(cfg) > 256 {
		return fmt.Errorf("%s exceeds max 256 entries", prefix)
	}
	for k, v := range cfg {
		tk := strings.TrimSpace(k)
		if tk == "" || len(tk) > 128 {
			return fmt.Errorf("%s contains invalid key %q", prefix, k)
		}
		if len(v) > 4096 {
			return fmt.Errorf("%s[%q] exceeds max 4096 bytes", prefix, k)
		}
	}
	return nil
}

func validAddonInstallMode(mode string) bool {
	switch mode {
	case AddonInstallModeFlatDir, AddonInstallModeTModLatest, AddonInstallModeDSTLocalMods, AddonInstallModeWorkshopTree, AddonInstallModeBepInEx, AddonInstallModeOxide, AddonInstallModeSML, AddonInstallModeCS2Plugin, AddonInstallModeCS2Workshop:
		return true
	default:
		return false
	}
}

type InstanceSpec struct {
	RuntimeClassName string                `json:"runtimeClassName,omitempty"`
	State            InstanceStateValue    `json:"state"`
	NodeName         string                `json:"nodeName,omitempty"`
	NodeSelector     map[string]string     `json:"nodeSelector,omitempty"`
	Image            string                `json:"image,omitempty"`
	Command          []string              `json:"command,omitempty"`
	Args             []string              `json:"args,omitempty"`
	WorkingDir       string                `json:"workingDir,omitempty"`
	ServerArtifact   *ArtifactRef          `json:"serverArtifact,omitempty"`
	Addons           []InstanceAddon       `json:"addons,omitempty"`
	Resources        ResourceRequirements  `json:"resources,omitempty"`
	Ports            []InstancePort        `json:"ports,omitempty"`
	Env              map[string]string     `json:"env,omitempty"`
	Files            []InstanceFile        `json:"files,omitempty"`
	BackupPolicy     *InstanceBackupPolicy `json:"backupPolicy,omitempty"`
	Schedules        []InstanceSchedule    `json:"schedules,omitempty"`
	Metrics          []AppMetricSpec       `json:"metrics,omitempty"`
}

type InstanceUsage struct {
	CPUMillicores        int64                  `json:"cpuMillicores"`
	MemoryBytes          int64                  `json:"memoryBytes"`
	StorageBytes         int64                  `json:"storageBytes"`
	NetworkRxBytesPerSec int64                  `json:"networkRxBytesPerSec,omitempty"`
	NetworkTxBytesPerSec int64                  `json:"networkTxBytesPerSec,omitempty"`
	CustomMetrics        []InstanceCustomMetric `json:"customMetrics,omitempty"`
}

type InstanceBackupRecord struct {
	UID                string                    `json:"uid,omitempty"`
	OperationID        string                    `json:"operationID,omitempty"`
	AllocationID       string                    `json:"allocationID,omitempty"`
	TaskID             string                    `json:"taskID,omitempty"`
	Imported           bool                      `json:"imported,omitempty"`
	Phase              string                    `json:"phase,omitempty"`
	Stage              string                    `json:"stage,omitempty"`
	Progress           int32                     `json:"progress,omitempty"`
	ProcessedBytes     int64                     `json:"processedBytes,omitempty"`
	TotalBytes         int64                     `json:"totalBytes,omitempty"`
	SpeedBytesPerSec   int64                     `json:"speedBytesPerSec,omitempty"`
	StartedAt          *time.Time                `json:"startedAt,omitempty"`
	Message            string                    `json:"message,omitempty"`
	ReservedBytes      int64                     `json:"reservedBytes,omitempty"`
	Path               string                    `json:"path"`
	SizeBytes          int64                     `json:"sizeBytes,omitempty"`
	SHA256             string                    `json:"sha256,omitempty"`
	CompletedAt        *time.Time                `json:"completedAt,omitempty"`
	LastRestoreTaskID  string                    `json:"lastRestoreTaskID,omitempty"`
	LastRestoreResult  string                    `json:"lastRestoreResult,omitempty"`
	LastRestoreMessage string                    `json:"lastRestoreMessage,omitempty"`
	LastRestoreAt      *time.Time                `json:"lastRestoreAt,omitempty"`
	RestoreSourcePath  string                    `json:"restoreSourcePath,omitempty"`
	RestoreTargetPath  string                    `json:"restoreTargetPath,omitempty"`
	RestoreReplace     *bool                     `json:"restoreReplace,omitempty"`
	Snapshot           *InstanceBackupSnapshot   `json:"snapshot,omitempty"`
	Inspection         *InstanceBackupInspection `json:"inspection,omitempty"`
}

// InstanceBackupAddon records a single mod/plugin entry either snapshotted from the instance
// at backup creation time or extracted from an uploaded archive.
type InstanceBackupAddon struct {
	Repository  string   `json:"repository,omitempty"`
	Package     string   `json:"package"`
	Version     string   `json:"version,omitempty"`
	Title       string   `json:"title,omitempty"`
	InstallMode string   `json:"installMode,omitempty"`
	TargetDir   string   `json:"targetDir,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
	ConfigCount int      `json:"configCount,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	ArchivePath string   `json:"archivePath,omitempty"`
}

// InstanceBackupSnapshot preserves the point-in-time instance metadata (server version,
// declarative configuration values, and installed addons) captured when creating a cloud backup.
type InstanceBackupSnapshot struct {
	CapturedAt    *time.Time                     `json:"capturedAt,omitempty"`
	AppName       string                         `json:"appName,omitempty"`
	ServerVersion string                         `json:"serverVersion,omitempty"`
	ConfigCount   int                            `json:"configCount,omitempty"`
	AddonCount    int                            `json:"addonCount,omitempty"`
	Addons        []InstanceBackupAddon          `json:"addons,omitempty"`
	ConfigFields  []InstanceBackupExtractedField `json:"configFields,omitempty"`
}

// InstanceBackupInspection stores controller-reconciled archive structure and secret-safe
// extracted config metadata inside status.backups so apiserver never streams S3 synchronously.
type InstanceBackupInspection struct {
	SchemaVersion      int                            `json:"schemaVersion,omitempty"`
	Phase              string                         `json:"phase,omitempty"`
	Message            string                         `json:"message,omitempty"`
	InspectedAt        *time.Time                     `json:"inspectedAt,omitempty"`
	ArchiveFormat      string                         `json:"archiveFormat,omitempty"`
	FileCount          int                            `json:"fileCount,omitempty"`
	UncompressedBytes  int64                          `json:"uncompressedBytes,omitempty"`
	RecommendedSource  string                         `json:"recommendedSource,omitempty"`
	CandidateSources   []string                       `json:"candidateSources,omitempty"`
	ArchiveDirectories []string                       `json:"archiveDirectories,omitempty"`
	DirectoryEntries   map[string][]string            `json:"directoryEntries,omitempty"`
	ExtractedFields    []InstanceBackupExtractedField `json:"extractedFields,omitempty"`
	ExtractedAddons    []InstanceBackupAddon          `json:"extractedAddons,omitempty"`
}

// InstanceBackupExtractedField captures a single declarative config value from an archive file.
// Secret fields never store plaintext values; only existence and salted digest are persisted.
type InstanceBackupExtractedField struct {
	Name         string `json:"name"`
	TemplatePath string `json:"templatePath"`
	ArchivePath  string `json:"archivePath"`
	Value        string `json:"value,omitempty"`
	Secret       bool   `json:"secret,omitempty"`
	HasValue     bool   `json:"hasValue,omitempty"`
	SecretDigest string `json:"secretDigest,omitempty"`
}

// InstanceTaskStreamEvent represents a single NDJSON progress or log event streamed from node-agent.
type InstanceTaskStreamEvent struct {
	Type             string         `json:"type"`
	Stage            string         `json:"stage,omitempty"`
	Progress         int32          `json:"progress,omitempty"`
	ProcessedBytes   int64          `json:"processedBytes,omitempty"`
	TotalBytes       int64          `json:"totalBytes,omitempty"`
	SpeedBytesPerSec int64          `json:"speedBytesPerSec,omitempty"`
	Message          string         `json:"message,omitempty"`
	Result           *InstanceFiles `json:"result,omitempty"`
}

type InstanceStatus struct {
	ResourceControl    *InstanceResourceControl `json:"resourceControl,omitempty"`
	ObservedGeneration int64                    `json:"observedGeneration"`
	Phase              InstancePhase            `json:"phase"`
	RuntimeID          string                   `json:"runtimeID,omitempty"`
	Endpoint           InstanceEndpoint         `json:"endpoint,omitempty"`
	Usage              *InstanceUsage           `json:"usage,omitempty"`
	Backups            []InstanceBackupRecord   `json:"backups,omitempty"`
	ScheduleStatuses   []InstanceScheduleStatus `json:"scheduleStatuses,omitempty"`
	StartedAt          *time.Time               `json:"startedAt,omitempty"`
	ObservedAt         *time.Time               `json:"observedAt,omitempty"`
	TransitionID       string                   `json:"transitionId,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
}

func (in *Instance) GetTypeMeta() *metav1.TypeMeta     { return &in.TypeMeta }
func (in *Instance) GetObjectMeta() *metav1.ObjectMeta { return &in.Metadata }

func (in *Instance) DefaultMeta() {
	in.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "Instance"}
	if in.Spec.State == "" {
		in.Spec.State = InstanceStateRunning
	}
	defaultInstanceResources(&in.Spec.Resources)
	if in.Status.Phase == "" {
		in.Status.Phase = InstancePhasePending
	}
	for i := range in.Spec.Ports {
		proto := strings.ToUpper(strings.TrimSpace(in.Spec.Ports[i].Protocol))
		if proto == "" {
			proto = ProtocolTCP
		}
		in.Spec.Ports[i].Protocol = proto
		if strings.TrimSpace(in.Spec.Ports[i].Name) == "" {
			in.Spec.Ports[i].Name = fmt.Sprintf("port-%d", i+1)
		}
	}
}

func defaultInstanceResources(res *ResourceRequirements) {
	if res.Requests == nil && res.Limits == nil {
		res.Requests = ResourceList{ResourceCPU: "1", ResourceMemory: "1Gi", ResourceStorage: "5Gi"}
		res.Limits = ResourceList{ResourceCPU: "1", ResourceMemory: "1Gi", ResourceStorage: "5Gi"}
		return
	}
	if res.Requests == nil {
		res.Requests = ResourceList{}
	}
	if res.Limits == nil {
		res.Limits = ResourceList{}
	}
	for k, v := range res.Limits {
		if res.Requests[k] == "" {
			res.Requests[k] = v
		}
	}
	for k, v := range res.Requests {
		if res.Limits[k] == "" {
			res.Limits[k] = v
		}
	}
	for _, pair := range [][2]string{{string(ResourceCPU), "1"}, {string(ResourceMemory), "1Gi"}, {string(ResourceStorage), "5Gi"}} {
		rn := ResourceName(pair[0])
		if res.Requests[rn] == "" {
			res.Requests[rn] = Quantity(pair[1])
			res.Limits[rn] = Quantity(pair[1])
		}
	}
}

func (in *Instance) Validate() error {
	if err := metav1.ValidateSingleSegmentName("instance metadata.name", in.Metadata.Name); err != nil {
		return err
	}
	if err := metav1.ValidateSingleSegmentName("instance metadata.tenant", in.Metadata.Tenant); err != nil {
		return err
	}
	switch in.Spec.State {
	case InstanceStateRunning, InstanceStateStopped:
	default:
		return fmt.Errorf("invalid instance spec.state %q", in.Spec.State)
	}
	if strings.TrimSpace(in.Spec.RuntimeClassName) == "" {
		return errors.New("instance spec.runtimeClassName is required")
	}
	if err := in.Spec.ServerArtifact.Validate("instance spec.serverArtifact"); err != nil {
		return err
	}
	for i := range in.Spec.Addons {
		if err := in.Spec.Addons[i].Validate(i); err != nil {
			return err
		}
	}
	return validateInstanceFilesMetricsPorts(&in.Spec)
}

func validateInstanceFilesMetricsPorts(spec *InstanceSpec) error {
	for _, f := range spec.Files {
		if err := metav1.ValidateRelativeFilePath(f.Path); err != nil {
			return fmt.Errorf("instance spec.files: %w", err)
		}
	}
	for _, m := range spec.Metrics {
		if m.Source == MetricSourceFile && strings.TrimSpace(m.FilePath) != "" {
			if err := metav1.ValidateRelativeFilePath(m.FilePath); err != nil {
				return fmt.Errorf("instance spec.metrics[%s].filePath: %w", m.ID, err)
			}
		}
	}
	for _, p := range spec.Ports {
		if p.TargetPort <= 0 || p.TargetPort > 65535 {
			return fmt.Errorf("invalid instance port targetPort %d: must be in [1, 65535]", p.TargetPort)
		}
		if p.PublishedPort < 0 || p.PublishedPort > 65535 {
			return fmt.Errorf("invalid instance port publishedPort %d: must be in [0, 65535]", p.PublishedPort)
		}
		proto := strings.ToUpper(strings.TrimSpace(p.Protocol))
		if proto != "" && proto != ProtocolTCP && proto != ProtocolUDP {
			return fmt.Errorf("invalid instance port protocol %q: must be TCP or UDP", p.Protocol)
		}
	}
	return spec.Resources.Validate()
}

// InstanceState represents the /state subresource of an Instance.
type InstanceState struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta  `json:"metadata"`
	State           InstanceStateValue `json:"state"`
}

// InstanceExec represents the /exec subresource of an Instance.
type InstanceExec struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Command         []string          `json:"command,omitempty"`
	Output          string            `json:"output,omitempty"`
	ExecutedAt      time.Time         `json:"executedAt,omitempty"`
}

// InstanceFiles represents the /files subresource of an Instance, issuing a short-lived
// direct-to-node ticket or performing S3 pre-signed URL streaming backup/restore without
// routing bulk payloads through the control-plane API server.
type InstanceFiles struct {
	metav1.TypeMeta      `json:",inline"`
	Metadata             metav1.ObjectMeta          `json:"metadata"`
	OperationID          string                     `json:"operationID,omitempty"`
	Action               string                     `json:"action,omitempty"`
	Path                 string                     `json:"path,omitempty"`
	SourcePath           string                     `json:"sourcePath,omitempty"`
	TargetPath           string                     `json:"targetPath,omitempty"`
	ReplaceTarget        *bool                      `json:"replaceTarget,omitempty"`
	ArchiveFormat        string                     `json:"archiveFormat,omitempty"`
	RecommendedSource    string                     `json:"recommendedSource,omitempty"`
	CandidateSources     []string                   `json:"candidateSources,omitempty"`
	ArchiveDirectories   []string                   `json:"archiveDirectories,omitempty"`
	SourceEntries        []string                   `json:"sourceEntries,omitempty"`
	ManagedFiles         []string                   `json:"managedFiles,omitempty"`
	FileCount            int                        `json:"fileCount,omitempty"`
	UncompressedBytes    int64                      `json:"uncompressedBytes,omitempty"`
	ServerVersion        string                     `json:"serverVersion,omitempty"`
	CurrentServerVersion string                     `json:"currentServerVersion,omitempty"`
	ConfigPreview        []InstanceBackupConfigItem `json:"configPreview,omitempty"`
	AddonPreview         []InstanceBackupAddonItem  `json:"addonPreview,omitempty"`
	PresignedURL         string                     `json:"presignedURL,omitempty"`
	MaxBytes             int64                      `json:"maxBytes,omitempty"`
	RateLimitBytesPerSec int64                      `json:"rateLimitBytesPerSec,omitempty"`
	Phase                string                     `json:"phase,omitempty"`
	Ticket               string                     `json:"ticket,omitempty"`
	DirectURL            string                     `json:"directURL,omitempty"`
	ExpiresAt            *time.Time                 `json:"expiresAt,omitempty"`
	SizeBytes            int64                      `json:"sizeBytes,omitempty"`
	SHA256               string                     `json:"sha256,omitempty"`
	CompletedAt          *time.Time                 `json:"completedAt,omitempty"`
	Entries              []InstanceFileEntry        `json:"entries,omitempty"`
}

// InstanceBackupConfigItem describes a single declarative config parameter extracted from
// a backup archive and compared against the Instance's active panel configuration.
type InstanceBackupConfigItem struct {
	Name              string `json:"name"`
	Label             string `json:"label"`
	Group             string `json:"group,omitempty"`
	Type              string `json:"type,omitempty"`
	ArchiveValue      string `json:"archiveValue,omitempty"`
	CurrentValue      string `json:"currentValue,omitempty"`
	ArchiveSet        bool   `json:"archiveSet,omitempty"`
	CurrentSet        bool   `json:"currentSet,omitempty"`
	Different         bool   `json:"different,omitempty"`
	OverriddenByPanel bool   `json:"overriddenByPanel,omitempty"`
	Secret            bool   `json:"secret,omitempty"`
}

// InstanceBackupAddonItem describes a single mod/plugin entry from a backup snapshot
// or inspected archive, compared against the Instance's active Spec.Addons.
type InstanceBackupAddonItem struct {
	Repository      string   `json:"repository,omitempty"`
	Package         string   `json:"package"`
	Title           string   `json:"title,omitempty"`
	BackupVersion   string   `json:"backupVersion,omitempty"`
	CurrentVersion  string   `json:"currentVersion,omitempty"`
	BackupDisabled  bool     `json:"backupDisabled,omitempty"`
	CurrentDisabled bool     `json:"currentDisabled,omitempty"`
	InstalledNow    bool     `json:"installedNow,omitempty"`
	Different       bool     `json:"different,omitempty"`
	ConfigCount     int      `json:"configCount,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
}

type InstanceFileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"isDir"`
	ModTime time.Time `json:"modTime"`
}

// InstanceMetricSample is a single point-in-time resource telemetry sample for an Instance.
type InstanceMetricSample struct {
	Timestamp            time.Time          `json:"timestamp"`
	CPUMillicores        int64              `json:"cpuMillicores"`
	MemoryBytes          int64              `json:"memoryBytes"`
	StorageBytes         int64              `json:"storageBytes"`
	NetworkRxBytesPerSec int64              `json:"networkRxBytesPerSec,omitempty"`
	NetworkTxBytesPerSec int64              `json:"networkTxBytesPerSec,omitempty"`
	CustomMetrics        map[string]float64 `json:"customMetrics,omitempty"`
}

// InstanceMetrics represents the /metrics subresource of an Instance.
type InstanceMetrics struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta      `json:"metadata"`
	Samples         []InstanceMetricSample `json:"samples"`
	CustomMetrics   []InstanceCustomMetric `json:"customMetrics,omitempty"`
}

const (
	CustomMetricIDOnlinePlayers = "online_players"
	CustomMetricIDServerTPS     = "server_tps"
)

const (
	ConfigFieldTypeString   = "string"
	ConfigFieldTypeText     = "text"
	ConfigFieldTypeNumber   = "number"
	ConfigFieldTypeBoolean  = "boolean"
	ConfigFieldTypeSelect   = "select"
	ConfigFieldTypePassword = "password"

	// ConfigOptionsSourceAddonMaps dynamically augments static select options with mapFolders
	// extracted from the Instance's enabled spec.addons Ready artifacts.
	ConfigOptionsSourceAddonMaps = "addon_maps"
)

// AppConfigOption defines a selectable choice for a "select" AppConfigField.
type AppConfigOption struct {
	Value   string `json:"value"`
	Label   string `json:"label"`
	LabelEn string `json:"labelEn,omitempty"`
}

// AppConfigField defines a single structured game configuration parameter in App.ConfigSchema.
type AppConfigField struct {
	Name          string            `json:"name"`
	Label         string            `json:"label"`
	LabelEn       string            `json:"labelEn,omitempty"`
	Type          string            `json:"type"` // text | password | number | select | boolean
	Group         string            `json:"group,omitempty"`
	Required      bool              `json:"required,omitempty"`
	Default       string            `json:"default,omitempty"`
	Min           *float64          `json:"min,omitempty"`
	Max           *float64          `json:"max,omitempty"`
	Step          *float64          `json:"step,omitempty"`
	Options       []AppConfigOption `json:"options,omitempty"`
	OptionsSource string            `json:"optionsSource,omitempty"`
	Help          string            `json:"help,omitempty"`
	HelpEn        string            `json:"helpEn,omitempty"`
	EnvKey        string            `json:"envKey,omitempty"`
	FileTarget    string            `json:"fileTarget,omitempty"`
}

// AppConfigPreset defines a one-click gameplay preset (e.g. Casual, Expert, Hardcore) for an App.
type AppConfigPreset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Values      map[string]string `json:"values"`
}

// AppFileTemplate defines a configuration file template rendered into Instance.Spec.Files.
type AppFileTemplate struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AppConsoleParam defines a dynamic template variable ({{name}}) inside an AppConsoleAction.Command.
type AppConsoleParam struct {
	Name          string            `json:"name"`
	Label         string            `json:"label"`
	Type          string            `json:"type,omitempty"` // text | number | select
	DefaultValue  string            `json:"defaultValue,omitempty"`
	Options       []AppConfigOption `json:"options,omitempty"`
	OptionsSource string            `json:"optionsSource,omitempty"`
}

// AppConsoleAction defines a categorized in-game server command template in App.ConsoleActions.
type AppConsoleAction struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Group   string            `json:"group,omitempty"`
	Command string            `json:"command"`
	Starred bool              `json:"starred,omitempty"`
	Params  []AppConsoleParam `json:"params,omitempty"`
}

// AppConsoleTarget defines a selectable target world/shard for console command routing (e.g. DST Master vs Caves).
type AppConsoleTarget struct {
	ID           string `json:"id"`                     // e.g. "master", "caves"
	Label        string `json:"label"`                  // e.g. "地上世界", "洞穴世界"
	Format       string `json:"format,omitempty"`       // e.g. "{{command}}" or "__GAMEPANEL_DST_CONSOLE__:caves:{{command_base64}}"
	EnabledField string `json:"enabledField,omitempty"` // optional configSchema field name, e.g. "enableCaves"
}

// AppStoragePolicy defines declarative save directory, backup/restore, and world reset policies for an App.
type AppStoragePolicy struct {
	BackupSubtree      string   `json:"backupSubtree,omitempty"`      // relative save directory inside workingDir
	AnchorFiles        []string `json:"anchorFiles,omitempty"`        // relative marker files identifying save root in archives
	ReplaceOnRestore   bool     `json:"replaceOnRestore,omitempty"`   // whether restore replaces backupSubtree cleanly
	BackupRequiresStop bool     `json:"backupRequiresStop,omitempty"` // whether backup/restore requires Stopped phase
	PreBackupCommand   string   `json:"preBackupCommand,omitempty"`   // optional flush command before backup
	PostBackupCommand  string   `json:"postBackupCommand,omitempty"`  // optional resume command after backup
	RegeneratePaths    []string `json:"regeneratePaths,omitempty"`    // relative save paths to remove on world regeneration
}

// AppServerBinding binds an App's server distribution to a Repository + Package in registry.gamepanel.io/v1.
type AppServerBinding struct {
	Repository string `json:"repository"`
	Package    string `json:"package"`
	TargetPath string `json:"targetPath,omitempty"` // e.g. "paper-{{Version}}.jar" or "."
}

func (b *AppServerBinding) Validate() error {
	if b == nil {
		return nil
	}
	if err := metav1.ValidateSingleSegmentName("app serverBinding.repository", b.Repository); err != nil {
		return err
	}
	if err := metav1.ValidateSingleSegmentName("app serverBinding.package", b.Package); err != nil {
		return err
	}
	if tp := strings.TrimSpace(b.TargetPath); tp != "" && tp != "." && !strings.Contains(tp, "{{") {
		if err := metav1.ValidateRelativeFilePath(tp); err != nil {
			return fmt.Errorf("app serverBinding.targetPath: %w", err)
		}
	}
	return nil
}

// AppAddonBinding binds an App's mod/plugin ecosystem to a Repository in registry.gamepanel.io/v1.
type AppAddonBinding struct {
	Repository  string            `json:"repository"`
	InstallMode string            `json:"installMode"` // flat_dir | tmod_latest | dst_local_mods | workshop_tree | bepinex | oxide | sml | cs2_plugin | cs2_workshop
	TargetDir   string            `json:"targetDir"`   // e.g. "plugins", "Mods", "mods", "oxide", "FactoryGame/Mods", "game/csgo/addons", "steamapps/workshop/content/108600"
	Label       string            `json:"label,omitempty"`
	Additional  []AppAddonBinding `json:"additional,omitempty"`
}

func validateSingleAddonBinding(prefix string, b *AppAddonBinding) error {
	if err := metav1.ValidateSingleSegmentName(prefix+".repository", b.Repository); err != nil {
		return err
	}
	if !validAddonInstallMode(strings.TrimSpace(b.InstallMode)) {
		return fmt.Errorf("invalid %s.installMode %q", prefix, b.InstallMode)
	}
	td := strings.TrimSpace(b.TargetDir)
	if td == "" {
		return fmt.Errorf("%s.targetDir is required", prefix)
	}
	if td != "." {
		if err := metav1.ValidateRelativeFilePath(td); err != nil {
			return fmt.Errorf("%s.targetDir: %w", prefix, err)
		}
	}
	return nil
}

func (b *AppAddonBinding) Validate() error {
	if b == nil {
		return nil
	}
	if err := validateSingleAddonBinding("app addonBinding", b); err != nil {
		return err
	}
	seen := map[string]struct{}{strings.ToLower(strings.TrimSpace(b.Repository)): {}}
	for i := range b.Additional {
		sub := &b.Additional[i]
		prefix := fmt.Sprintf("app addonBinding.additional[%d]", i)
		if len(sub.Additional) > 0 {
			return fmt.Errorf("%s cannot nest additional bindings", prefix)
		}
		if err := validateSingleAddonBinding(prefix, sub); err != nil {
			return err
		}
		key := strings.ToLower(strings.TrimSpace(sub.Repository))
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s duplicates repository %q", prefix, sub.Repository)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// All returns the primary AppAddonBinding followed by any additional bindings configured on the App.
func (b *AppAddonBinding) All() []AppAddonBinding {
	if b == nil {
		return nil
	}
	out := make([]AppAddonBinding, 0, 1+len(b.Additional))
	out = append(out, AppAddonBinding{
		Repository:  b.Repository,
		InstallMode: b.InstallMode,
		TargetDir:   b.TargetDir,
		Label:       b.Label,
	})
	for _, sub := range b.Additional {
		out = append(out, AppAddonBinding{
			Repository:  sub.Repository,
			InstallMode: sub.InstallMode,
			TargetDir:   sub.TargetDir,
			Label:       sub.Label,
		})
	}
	return out
}

// ForRepository resolves the matching binding for repo (falling back to the primary binding when empty or unmatched).
func (b *AppAddonBinding) ForRepository(repo string) *AppAddonBinding {
	if b == nil {
		return nil
	}
	needle := strings.ToLower(strings.TrimSpace(repo))
	if needle == "" || strings.EqualFold(strings.TrimSpace(b.Repository), needle) {
		return b
	}
	for i := range b.Additional {
		if strings.EqualFold(strings.TrimSpace(b.Additional[i].Repository), needle) {
			return &b.Additional[i]
		}
	}
	return b
}

const (
	// DefaultAppRuntimeImage names the unified prebuilt game runtime container image.
	DefaultAppRuntimeImage = "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/runtime:latest"
	// DefaultAppCustomImage names the default fallback image for custom containers.
	DefaultAppCustomImage = "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/alpine:3.20"
)

// App represents a cluster-scoped game application template in the platform marketplace (应用市场).
type App struct {
	metav1.TypeMeta    `json:",inline"`
	Metadata           metav1.ObjectMeta  `json:"metadata"`
	DisplayName        string             `json:"displayName"`
	Tagline            string             `json:"tagline,omitempty"`
	Summary            string             `json:"summary,omitempty"`
	Category           string             `json:"category,omitempty"` // sandbox | survival | coop | custom
	CoverImage         string             `json:"coverImage,omitempty"`
	Image              string             `json:"image"`
	ImageTemplate      string             `json:"imageTemplate,omitempty"`
	Versions           []string           `json:"versions,omitempty"`
	RecommendedVersion string             `json:"recommendedVersion,omitempty"`
	ServerBinding      *AppServerBinding  `json:"serverBinding,omitempty"`
	AddonBinding       *AppAddonBinding   `json:"addonBinding,omitempty"`
	RuntimeClassName   string             `json:"runtimeClassName,omitempty"`
	PreStartCommand    string             `json:"preStartCommand,omitempty"`
	StartupCommand     string             `json:"startupCommand,omitempty"`
	PreStopCommand     string             `json:"preStopCommand,omitempty"`
	Command            []string           `json:"command,omitempty"`
	WorkingDir         string             `json:"workingDir,omitempty"`
	Ports              []InstancePort     `json:"ports,omitempty"`
	Env                map[string]string  `json:"env,omitempty"`
	RecommendedPlan    string             `json:"recommendedPlan,omitempty"`
	Resources          ResourceList       `json:"resources,omitempty"`
	ConfigSchema       []AppConfigField   `json:"configSchema,omitempty"`
	ConfigPresets      []AppConfigPreset  `json:"configPresets,omitempty"`
	FileTemplates      []AppFileTemplate  `json:"fileTemplates,omitempty"`
	ConsoleDelivery    string             `json:"consoleDelivery,omitempty"` // auto | stdin | rcon | exec
	ConsoleTargets     []AppConsoleTarget `json:"consoleTargets,omitempty"`
	ConsoleActions     []AppConsoleAction `json:"consoleActions,omitempty"`
	StoragePolicy      *AppStoragePolicy  `json:"storagePolicy,omitempty"`
	Metrics            []AppMetricSpec    `json:"metrics,omitempty"`
	Capabilities       []string           `json:"capabilities,omitempty"`
	Published          bool               `json:"published"`
	BuiltIn            bool               `json:"builtIn,omitempty"`
}

func (a *App) GetTypeMeta() *metav1.TypeMeta     { return &a.TypeMeta }
func (a *App) GetObjectMeta() *metav1.ObjectMeta { return &a.Metadata }

func (a *App) DefaultMeta() {
	a.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "App"}
	if strings.TrimSpace(a.Image) == "" {
		a.Image = DefaultAppRuntimeImage
	}
	if strings.TrimSpace(a.Category) == "" {
		a.Category = "survival"
	}
	if strings.TrimSpace(a.RecommendedPlan) == "" {
		a.RecommendedPlan = "standard"
	}
	if len(a.Versions) > 0 && strings.TrimSpace(a.RecommendedVersion) == "" {
		a.RecommendedVersion = a.Versions[0]
	}
	for i := range a.Ports {
		proto := strings.ToUpper(strings.TrimSpace(a.Ports[i].Protocol))
		if proto == "" {
			proto = ProtocolTCP
		}
		a.Ports[i].Protocol = proto
		if strings.TrimSpace(a.Ports[i].Name) == "" {
			a.Ports[i].Name = fmt.Sprintf("port-%d", i+1)
		}
	}
}

func (a *App) Validate() error {
	if strings.TrimSpace(a.Metadata.Name) == "" {
		return errors.New("app metadata.name is required")
	}
	if strings.TrimSpace(a.DisplayName) == "" {
		return errors.New("app displayName is required")
	}
	if strings.TrimSpace(a.Image) == "" {
		return errors.New("app image is required")
	}
	for _, p := range a.Ports {
		if p.TargetPort <= 0 || p.TargetPort > 65535 {
			return fmt.Errorf("invalid app port targetPort %d: must be in [1, 65535]", p.TargetPort)
		}
		proto := strings.ToUpper(strings.TrimSpace(p.Protocol))
		if proto != "" && proto != ProtocolTCP && proto != ProtocolUDP {
			return fmt.Errorf("invalid app port protocol %q: must be TCP or UDP", p.Protocol)
		}
	}
	if err := a.ServerBinding.Validate(); err != nil {
		return err
	}
	return a.AddonBinding.Validate()
}

// PrewarmJobPhase describes the asynchronous execution state of a PrewarmJob or per-node target.
type PrewarmJobPhase string

const (
	PrewarmJobPhasePending   PrewarmJobPhase = "Pending"
	PrewarmJobPhaseRunning   PrewarmJobPhase = "Running"
	PrewarmJobPhaseSucceeded PrewarmJobPhase = "Succeeded"
	PrewarmJobPhaseFailed    PrewarmJobPhase = "Failed"
)

// PrewarmJob dispatches image pull and artifact blob warm-up tasks across selected compute Nodes.
type PrewarmJob struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            PrewarmJobSpec    `json:"spec"`
	Status          PrewarmJobStatus  `json:"status,omitempty"`
}

type PrewarmJobSpec struct {
	NodeNames    []string          `json:"nodeNames,omitempty"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	Artifacts    []ArtifactRef     `json:"artifacts,omitempty"`
	Images       []string          `json:"images,omitempty"`
}

type PrewarmNodeStatus struct {
	NodeName    string          `json:"nodeName"`
	Phase       PrewarmJobPhase `json:"phase"`
	Message     string          `json:"message,omitempty"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
}

type PrewarmJobStatus struct {
	ObservedGeneration int64               `json:"observedGeneration"`
	Phase              PrewarmJobPhase     `json:"phase"`
	TargetNodes        int32               `json:"targetNodes"`
	ReadyNodes         int32               `json:"readyNodes"`
	FailedNodes        int32               `json:"failedNodes"`
	Nodes              []PrewarmNodeStatus `json:"nodes,omitempty"`
	Conditions         []metav1.Condition  `json:"conditions,omitempty"`
}

func (p *PrewarmJob) GetTypeMeta() *metav1.TypeMeta     { return &p.TypeMeta }
func (p *PrewarmJob) GetObjectMeta() *metav1.ObjectMeta { return &p.Metadata }

func (p *PrewarmJob) DefaultMeta() {
	p.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindPrewarmJob}
	if p.Status.Phase == "" {
		p.Status.Phase = PrewarmJobPhasePending
	}
}

func (p *PrewarmJob) Validate() error {
	if err := metav1.ValidateSingleSegmentName("prewarmjob metadata.name", p.Metadata.Name); err != nil {
		return err
	}
	if len(p.Spec.Artifacts) == 0 && len(p.Spec.Images) == 0 {
		return errors.New("prewarmjob spec must specify at least one artifact or image")
	}
	for i := range p.Spec.Artifacts {
		if err := p.Spec.Artifacts[i].Validate(fmt.Sprintf("prewarmjob spec.artifacts[%d]", i)); err != nil {
			return err
		}
	}
	for _, img := range p.Spec.Images {
		if strings.TrimSpace(img) == "" {
			return errors.New("prewarmjob spec.images cannot contain empty entries")
		}
	}
	return nil
}

const (
	InstanceStagePendingQuota   = "PendingQuota"
	InstanceStageScheduling     = "Scheduling"
	InstanceStageActive         = "Active"
	InstanceStageDeleting       = "Deleting"
	InstanceStageReleasingQuota = "ReleasingQuota"

	FinalizerResourceControl = "core.gamepanel.io/resource-control"

	AttemptPhasePending = "Pending"
	AttemptPhaseBound   = "Bound"
	AttemptPhaseClosing = "Closing"
	AttemptPhaseClosed  = "Closed"

	ResourceChangePhaseBudgeting = "Budgeting"
	ResourceChangePhaseApplying  = "Applying"
)

func (s TenantSpec) ValidateQuota() error {
	if err := s.Quota.Validate(); err != nil {
		return err
	}
	seen := map[ResourceName]bool{}
	for _, name := range s.UnlimitedQuota {
		switch name {
		case ResourceCPU, ResourceMemory, ResourceStorage, ResourceInstances, ResourceBackups, ResourceBackupStorage:
		default:
			return fmt.Errorf("unknown unlimited quota dimension %q", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate unlimited quota dimension %q", name)
		}
		if _, numeric := s.Quota[name]; numeric {
			return fmt.Errorf("quota dimension %q cannot be numeric and unlimited", name)
		}
		seen[name] = true
	}
	return nil
}

func (s TenantSpec) IsQuotaUnlimited(name ResourceName) bool {
	for _, candidate := range s.UnlimitedQuota {
		if candidate == name {
			return true
		}
	}
	return false
}

// InstanceResourceControl contains server-owned durable coordination facts.
type InstanceResourceControl struct {
	TenantUID         string                    `json:"tenantUID"`
	AllocationID      string                    `json:"allocationID"`
	Stage             string                    `json:"stage"`
	InitialResources  ResourceList              `json:"initialResources"`
	DesiredRevision   int64                     `json:"desiredRevision"`
	ApprovedRevision  int64                     `json:"approvedRevision"`
	ObservedRevision  int64                     `json:"observedRevision"`
	ObservedAttemptID string                    `json:"observedAttemptID,omitempty"`
	ObservedEpoch     int64                     `json:"observedEpoch,omitempty"`
	ObservedResources ResourceList              `json:"observedResources,omitempty"`
	ObservedPorts     []InstancePort            `json:"observedPorts,omitempty"`
	RuntimeClass      *InstanceRuntimeBinding   `json:"runtimeClass,omitempty"`
	ApprovedResources ResourceList              `json:"approvedResources,omitempty"`
	ApprovedPorts     []InstancePort            `json:"approvedPorts,omitempty"`
	CurrentAttempt    *InstancePlacementAttempt `json:"currentAttempt,omitempty"`
	Change            *InstanceResourceChange   `json:"change,omitempty"`
	Suspended         bool                      `json:"suspended,omitempty"`
}

// EffectiveInstanceState returns InstanceStateStopped when suspended by workspace billing, else spec.state.
func EffectiveInstanceState(inst *Instance) InstanceStateValue {
	if inst == nil {
		return InstanceStateStopped
	}
	if inst.Status.ResourceControl != nil && inst.Status.ResourceControl.Suspended {
		return InstanceStateStopped
	}
	return inst.Spec.State
}

// InstanceRuntimeBinding freezes the execution backend independently of mutable RuntimeClass objects.
type InstanceRuntimeBinding struct {
	UID        string            `json:"uid"`
	Name       string            `json:"name"`
	Handler    string            `json:"handler"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// InstancePlacementAttempt is persisted before any node admission.
type InstancePlacementAttempt struct {
	ID               string         `json:"id"`
	NodeUID          string         `json:"nodeUID"`
	NodeName         string         `json:"nodeName"`
	Epoch            int64          `json:"epoch"`
	ResourceRevision int64          `json:"resourceRevision"`
	Resources        ResourceList   `json:"resources"`
	Ports            []InstancePort `json:"ports,omitempty"`
	Phase            string         `json:"phase"`
}

// InstanceResourceChange freezes the complete old and target budget.
type InstanceResourceChange struct {
	ID              string       `json:"id"`
	FromRevision    int64        `json:"fromRevision"`
	ToRevision      int64        `json:"toRevision"`
	OldResources    ResourceList `json:"oldResources"`
	TargetResources ResourceList `json:"targetResources"`
	Phase           string       `json:"phase"`
}

// AllocationState describes current holdings in quota_allocations and node_reservations.
type AllocationState string

const (
	AllocationPending AllocationState = "Pending"
	AllocationActive  AllocationState = "Active"
	AllocationClosing AllocationState = "Closing"
	AllocationClosed  AllocationState = "Closed"
)

// OperationState describes command state in quota_operations and node_operations.
type OperationState string

const (
	Accepted  OperationState = "Accepted"
	Committed OperationState = "Committed"
	Cancelled OperationState = "Cancelled"
)

// Domain identifies the quota vs node inventory domain.
type Domain string

const (
	QuotaDomain Domain = "quota"
	NodeDomain  Domain = "node"
)

// Kind identifies a resource control command kind.
type Kind string

const (
	Grant          Kind = "Grant"
	Increase       Kind = "Increase"
	SettleDecrease Kind = "SettleDecrease"
	RequestClose   Kind = "RequestClose"
	FinalizeClose  Kind = "FinalizeClose"
)

// TenantInvite represents a signed workspace invitation token and redemption payload.
type TenantInvite struct {
	metav1.TypeMeta   `json:",inline"`
	Metadata          metav1.ObjectMeta `json:"metadata"`
	TenantDisplayName string            `json:"tenantDisplayName,omitempty"`
	Inviter           string            `json:"inviter,omitempty"`
	Role              string            `json:"role,omitempty"`
	ServerName        string            `json:"serverName,omitempty"`
	Token             string            `json:"token,omitempty"`
	ExpiresAt         *time.Time        `json:"expiresAt,omitempty"`
	Username          string            `json:"username,omitempty"`
	Email             string            `json:"email,omitempty"`
	Password          string            `json:"password,omitempty"`
	// Mode selects how anonymous invitees authenticate: "Register" requires a
	// new username, "Login" requires an existing account. Empty keeps the
	// legacy register-or-login behavior.
	Mode         TenantInviteMode `json:"mode,omitempty"`
	SessionToken string           `json:"sessionToken,omitempty"`
}

// TenantInviteMode selects how an anonymous invitee authenticates when
// accepting a workspace invite.
type TenantInviteMode string

const (
	TenantInviteModeRegister TenantInviteMode = "Register"
	TenantInviteModeLogin    TenantInviteMode = "Login"

	InviteActionAccept = "accept"
)
