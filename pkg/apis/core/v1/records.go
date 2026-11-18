package v1

import (
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// TenantRecord is the strongly-typed GORM model for the tenants declarative table.
type TenantRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	DisplayName     string            `gorm:"column:display_name;not null;default:''"`
	Type            string            `gorm:"column:type;not null;default:'Organization'"`
	Owner           string            `gorm:"column:owner;not null"`
	Quota           ResourceList      `gorm:"column:quota;serializer:json;type:jsonb;not null"`
	UnlimitedQuota  []ResourceName    `gorm:"column:unlimited_quota;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (TenantRecord) TableName() string { return "tenants" }

// TenantStateRecord is the strongly-typed GORM model for dynamic tenant state (tenant_states).
type TenantStateRecord struct {
	TenantUID          string             `gorm:"column:tenant_uid;primaryKey"`
	Version            int64              `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64              `gorm:"column:observed_generation;not null;default:0"`
	Phase              string             `gorm:"column:phase;not null;default:'Active'"`
	Usage              ResourceList       `gorm:"column:usage;serializer:json;type:jsonb;not null"`
	Conditions         []metav1.Condition `gorm:"column:conditions;serializer:json;type:jsonb;not null"`
	UpdatedAt          time.Time          `gorm:"column:updated_at;not null"`
}

func (TenantStateRecord) TableName() string { return "tenant_states" }

// RuntimeClassRecord is the strongly-typed GORM model for the runtime_classes table.
type RuntimeClassRecord struct {
	Name            string                  `gorm:"column:name;primaryKey"`
	UID             string                  `gorm:"column:uid;unique;not null"`
	Handler         string                  `gorm:"column:handler;not null"`
	IsDefault       bool                    `gorm:"column:is_default;not null;default:false"`
	Parameters      map[string]string       `gorm:"column:parameters;serializer:json;type:jsonb;not null"`
	Scheduling      *RuntimeClassScheduling `gorm:"column:scheduling;serializer:json;type:jsonb"`
	ResourceVersion int64                   `gorm:"column:resource_version;not null;default:1"`
	Generation      int64                   `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time               `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time              `gorm:"column:deleted_at"`
	Finalizers      []string                `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string       `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string       `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (RuntimeClassRecord) TableName() string { return "runtime_classes" }

// AppRecord is the strongly-typed GORM model for the apps marketplace template table.
type AppRecord struct {
	Name               string             `gorm:"column:name;primaryKey"`
	UID                string             `gorm:"column:uid;unique;not null"`
	DisplayName        string             `gorm:"column:display_name;not null"`
	Tagline            string             `gorm:"column:tagline;not null;default:''"`
	Summary            string             `gorm:"column:summary;not null;default:''"`
	Category           string             `gorm:"column:category;not null;default:'survival'"`
	CoverImage         string             `gorm:"column:cover_image;not null;default:''"`
	Image              string             `gorm:"column:image;not null"`
	ImageTemplate      string             `gorm:"column:image_template;not null;default:''"`
	Versions           []string           `gorm:"column:versions;serializer:json;type:jsonb;not null"`
	RecommendedVersion string             `gorm:"column:recommended_version;not null;default:''"`
	ServerBinding      *AppServerBinding  `gorm:"column:server_binding;serializer:json;type:jsonb"`
	AddonBinding       *AppAddonBinding   `gorm:"column:addon_binding;serializer:json;type:jsonb"`
	RuntimeClassName   string             `gorm:"column:runtime_class_name;not null;default:''"`
	PreStartCommand    string             `gorm:"column:pre_start_command;not null;default:''"`
	StartupCommand     string             `gorm:"column:startup_command;not null;default:''"`
	PreStopCommand     string             `gorm:"column:pre_stop_command;not null;default:''"`
	Command            []string           `gorm:"column:command;serializer:json;type:jsonb;not null"`
	WorkingDir         string             `gorm:"column:working_dir;not null;default:''"`
	Ports              []InstancePort     `gorm:"column:ports;serializer:json;type:jsonb;not null"`
	Env                map[string]string  `gorm:"column:env;serializer:json;type:jsonb;not null"`
	RecommendedPlan    string             `gorm:"column:recommended_plan;not null;default:'standard'"`
	Resources          ResourceList       `gorm:"column:resources;serializer:json;type:jsonb;not null"`
	ConfigSchema       []AppConfigField   `gorm:"column:config_schema;serializer:json;type:jsonb;not null"`
	ConfigPresets      []AppConfigPreset  `gorm:"column:config_presets;serializer:json;type:jsonb;not null"`
	FileTemplates      []AppFileTemplate  `gorm:"column:file_templates;serializer:json;type:jsonb;not null"`
	ConsoleDelivery    string             `gorm:"column:console_delivery;not null;default:''"`
	ConsoleTargets     []AppConsoleTarget `gorm:"column:console_targets;serializer:json;type:jsonb;not null"`
	ConsoleActions     []AppConsoleAction `gorm:"column:console_actions;serializer:json;type:jsonb;not null"`
	StoragePolicy      *AppStoragePolicy  `gorm:"column:storage_policy;serializer:json;type:jsonb"`
	Metrics            []AppMetricSpec    `gorm:"column:metrics;serializer:json;type:jsonb;not null"`
	Capabilities       []string           `gorm:"column:capabilities;serializer:json;type:jsonb;not null"`
	Published          bool               `gorm:"column:published;not null;default:false"`
	BuiltIn            bool               `gorm:"column:built_in;not null;default:false"`
	ResourceVersion    int64              `gorm:"column:resource_version;not null;default:1"`
	Generation         int64              `gorm:"column:generation;not null;default:1"`
	CreatedAt          time.Time          `gorm:"column:created_at;not null"`
	DeletedAt          *time.Time         `gorm:"column:deleted_at"`
	Finalizers         []string           `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels             map[string]string  `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations        map[string]string  `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (AppRecord) TableName() string { return "apps" }

// PrewarmJobRecord is the strongly-typed GORM model for the prewarm_jobs declarative table.
type PrewarmJobRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	NodeNames       []string          `gorm:"column:node_names;serializer:json;type:jsonb;not null"`
	NodeSelector    map[string]string `gorm:"column:node_selector;serializer:json;type:jsonb;not null"`
	Artifacts       []ArtifactRef     `gorm:"column:artifacts;serializer:json;type:jsonb;not null"`
	Images          []string          `gorm:"column:images;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (PrewarmJobRecord) TableName() string { return "prewarm_jobs" }

// PrewarmJobStateRecord is the strongly-typed GORM model for dynamic prewarm job states (prewarm_job_states).
type PrewarmJobStateRecord struct {
	JobUID             string              `gorm:"column:job_uid;primaryKey"`
	Version            int64               `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64               `gorm:"column:observed_generation;not null;default:0"`
	Phase              string              `gorm:"column:phase;not null;default:'Pending'"`
	TargetNodes        int32               `gorm:"column:target_nodes;not null;default:0"`
	ReadyNodes         int32               `gorm:"column:ready_nodes;not null;default:0"`
	FailedNodes        int32               `gorm:"column:failed_nodes;not null;default:0"`
	Nodes              []PrewarmNodeStatus `gorm:"column:nodes;serializer:json;type:jsonb;not null"`
	Conditions         []metav1.Condition  `gorm:"column:conditions;serializer:json;type:jsonb;not null"`
	UpdatedAt          time.Time           `gorm:"column:updated_at;not null"`
}

func (PrewarmJobStateRecord) TableName() string { return "prewarm_job_states" }

// NodeRecord is the strongly-typed GORM model for the nodes declarative table.
type NodeRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Unschedulable   bool              `gorm:"column:unschedulable;not null;default:false"`
	ExternalIP      string            `gorm:"column:external_ip;not null;default:''"`
	AllowedApps     []string          `gorm:"column:allowed_apps;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (NodeRecord) TableName() string { return "nodes" }

// NodeHeartbeatRecord is the strongly-typed GORM model for dynamic node heartbeat and capacity state (node_heartbeats).
type NodeHeartbeatRecord struct {
	NodeUID            string             `gorm:"column:node_uid;primaryKey"`
	NodeName           string             `gorm:"column:node_name;not null"`
	Version            int64              `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64              `gorm:"column:observed_generation;not null;default:0"`
	Phase              string             `gorm:"column:phase;not null;default:'Ready'"`
	Addresses          []NodeAddress      `gorm:"column:addresses;serializer:json;type:jsonb;not null"`
	Runtimes           []NodeRuntime      `gorm:"column:runtimes;serializer:json;type:jsonb;not null"`
	Capacity           ResourceList       `gorm:"column:capacity;serializer:json;type:jsonb;not null"`
	Allocatable        ResourceList       `gorm:"column:allocatable;serializer:json;type:jsonb;not null"`
	Usage              *NodeUsage         `gorm:"column:usage;serializer:json;type:jsonb"`
	CachedArtifacts    []string           `gorm:"column:cached_artifacts;serializer:json;type:jsonb;not null"`
	CachedBlobs        []string           `gorm:"column:cached_blobs;serializer:json;type:jsonb;not null"`
	WarmImages         []string           `gorm:"column:warm_images;serializer:json;type:jsonb;not null"`
	LastHeartbeatTime  *time.Time         `gorm:"column:last_heartbeat_time"`
	Conditions         []metav1.Condition `gorm:"column:conditions;serializer:json;type:jsonb;not null"`
	UpdatedAt          time.Time          `gorm:"column:updated_at;not null"`
}

func (NodeHeartbeatRecord) TableName() string { return "node_heartbeats" }

// InstanceRecord is the strongly-typed GORM model for the instances declarative & resource-control table.
type InstanceRecord struct {
	Tenant            string                   `gorm:"column:tenant;primaryKey"`
	Name              string                   `gorm:"column:name;primaryKey"`
	UID               string                   `gorm:"column:uid;unique;not null"`
	RuntimeClassName  string                   `gorm:"column:runtime_class_name;not null;default:''"`
	State             string                   `gorm:"column:state;not null;default:'Running'"`
	NodeName          string                   `gorm:"column:node_name;not null;default:''"`
	NodeSelector      map[string]string        `gorm:"column:node_selector;serializer:json;type:jsonb;not null"`
	Image             string                   `gorm:"column:image;not null;default:''"`
	Command           []string                 `gorm:"column:command;serializer:json;type:jsonb;not null"`
	Args              []string                 `gorm:"column:args;serializer:json;type:jsonb;not null"`
	WorkingDir        string                   `gorm:"column:working_dir;not null;default:''"`
	ServerArtifact    *ArtifactRef             `gorm:"column:server_artifact;serializer:json;type:jsonb"`
	Addons            []InstanceAddon          `gorm:"column:addons;serializer:json;type:jsonb;not null"`
	Resources         ResourceRequirements     `gorm:"column:resources;serializer:json;type:jsonb;not null"`
	Ports             []InstancePort           `gorm:"column:ports;serializer:json;type:jsonb;not null"`
	Env               map[string]string        `gorm:"column:env;serializer:json;type:jsonb;not null"`
	Files             []InstanceFile           `gorm:"column:files;serializer:json;type:jsonb;not null"`
	BackupPolicy      *InstanceBackupPolicy    `gorm:"column:backup_policy;serializer:json;type:jsonb"`
	Schedules         []InstanceSchedule       `gorm:"column:schedules;serializer:json;type:jsonb;not null"`
	Metrics           []AppMetricSpec          `gorm:"column:metrics;serializer:json;type:jsonb;not null"`
	ControlStage      string                   `gorm:"column:control_stage;not null;default:''"`
	ResourceControl   *InstanceResourceControl `gorm:"column:resource_control;serializer:json;type:jsonb"`
	ControlConditions []metav1.Condition       `gorm:"column:control_conditions;serializer:json;type:jsonb;not null"`
	ResourceVersion   int64                    `gorm:"column:resource_version;not null;default:1"`
	Generation        int64                    `gorm:"column:generation;not null;default:1"`
	CreatedAt         time.Time                `gorm:"column:created_at;not null"`
	DeletedAt         *time.Time               `gorm:"column:deleted_at"`
	Finalizers        []string                 `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels            map[string]string        `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations       map[string]string        `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (InstanceRecord) TableName() string { return "instances" }

// InstanceObservationRecord is the strongly-typed GORM model for dynamic runtime status, schedules, and backups (instance_observations).
type InstanceObservationRecord struct {
	InstanceUID        string                   `gorm:"column:instance_uid;primaryKey"`
	Tenant             string                   `gorm:"column:tenant;not null"`
	InstanceName       string                   `gorm:"column:instance_name;not null"`
	Version            int64                    `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64                    `gorm:"column:observed_generation;not null;default:0"`
	Phase              string                   `gorm:"column:phase;not null;default:'Pending'"`
	RuntimeID          string                   `gorm:"column:runtime_id;not null;default:''"`
	Endpoint           InstanceEndpoint         `gorm:"column:endpoint;serializer:json;type:jsonb;not null"`
	StartedAt          *time.Time               `gorm:"column:started_at"`
	ObservedAt         *time.Time               `gorm:"column:observed_at"`
	TransitionID       string                   `gorm:"column:transition_id;not null;default:''"`
	ObservedRevision   int64                    `gorm:"column:observed_revision;not null;default:0"`
	ObservedAttemptID  string                   `gorm:"column:observed_attempt_id;not null;default:''"`
	ObservedEpoch      int64                    `gorm:"column:observed_epoch;not null;default:0"`
	ObservedResources  ResourceList             `gorm:"column:observed_resources;serializer:json;type:jsonb;not null"`
	ObservedPorts      []InstancePort           `gorm:"column:observed_ports;serializer:json;type:jsonb;not null"`
	Conditions         []metav1.Condition       `gorm:"column:conditions;serializer:json;type:jsonb;not null"`
	ScheduleStatuses   []InstanceScheduleStatus `gorm:"column:schedule_statuses;serializer:json;type:jsonb;not null"`
	Backups            []InstanceBackupRecord   `gorm:"column:backups;serializer:json;type:jsonb;not null"`
	Usage              *InstanceUsage           `gorm:"column:usage;serializer:json;type:jsonb"`
	UpdatedAt          time.Time                `gorm:"column:updated_at;not null"`
}

func (InstanceObservationRecord) TableName() string { return "instance_observations" }

// BackupGateRecord is the GORM model for backup_instance_gates.
type BackupGateRecord struct {
	InstanceUID string `gorm:"column:instance_uid;primaryKey"`
	Closed      bool   `gorm:"column:closed;not null;default:false"`
}

func (BackupGateRecord) TableName() string { return "backup_instance_gates" }

// BackupCommandRecord is the GORM model for backup_commands.
type BackupCommandRecord struct {
	OperationID     string     `gorm:"column:operation_id;primaryKey"`
	InstanceUID     string     `gorm:"column:instance_uid;not null"`
	ObjectKey       string     `gorm:"column:object_key;not null"`
	RequestHash     string     `gorm:"column:request_hash;not null"`
	Command         []byte     `gorm:"column:command;type:jsonb;not null"`
	Phase           string     `gorm:"column:phase;not null"`
	Version         int64      `gorm:"column:version;not null;default:1"`
	Result          []byte     `gorm:"column:result;type:jsonb"`
	UploadExpiresAt *time.Time `gorm:"column:upload_expires_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (BackupCommandRecord) TableName() string { return "backup_commands" }

// QuotaAccountRecord is the GORM persistence model for quota_accounts.
type QuotaAccountRecord struct {
	TenantUID              string     `gorm:"column:tenant_uid;primaryKey"`
	PolicyVersion          int64      `gorm:"column:policy_version;not null;default:0"`
	UpstreamVersion        int64      `gorm:"column:upstream_version;not null;default:0"`
	ExpiresAt              *time.Time `gorm:"column:expires_at"`
	LimitCPU               *int64     `gorm:"column:limit_cpu"`
	LimitMem               *int64     `gorm:"column:limit_mem"`
	LimitStorage           *int64     `gorm:"column:limit_storage"`
	LimitInstances         *int64     `gorm:"column:limit_instances"`
	LimitBackups           *int64     `gorm:"column:limit_backups"`
	LimitBackupStorage     *int64     `gorm:"column:limit_backup_storage"`
	AllocatedCPU           int64      `gorm:"column:allocated_cpu;not null;default:0"`
	AllocatedMem           int64      `gorm:"column:allocated_mem;not null;default:0"`
	AllocatedStorage       int64      `gorm:"column:allocated_storage;not null;default:0"`
	AllocatedInstances     int64      `gorm:"column:allocated_instances;not null;default:0"`
	AllocatedBackups       int64      `gorm:"column:allocated_backups;not null;default:0"`
	AllocatedBackupStorage int64      `gorm:"column:allocated_backup_storage;not null;default:0"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (QuotaAccountRecord) TableName() string { return "quota_accounts" }

// QuotaPolicyChangeRecord is the GORM persistence model for quota_policy_changes.
type QuotaPolicyChangeRecord struct {
	ChangeID      string     `gorm:"column:change_id;primaryKey"`
	TenantUID     string     `gorm:"column:tenant_uid;not null"`
	RequestHash   string     `gorm:"column:request_hash;not null"`
	Plan          []byte     `gorm:"column:plan;type:jsonb;not null"`
	SourceVersion int64      `gorm:"column:source_version;not null"`
	ExportedAt    *time.Time `gorm:"column:exported_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
}

func (QuotaPolicyChangeRecord) TableName() string { return "quota_policy_changes" }

// QuotaAllocationRecord is the GORM persistence model for quota_allocations.
type QuotaAllocationRecord struct {
	AllocationID         string          `gorm:"column:allocation_id;primaryKey"`
	TenantUID            string          `gorm:"column:tenant_uid;not null"`
	InstanceUID          string          `gorm:"column:instance_uid;not null"`
	InstanceName         string          `gorm:"column:instance_name;not null"`
	InitialIntent        []byte          `gorm:"column:initial_intent;type:jsonb"`
	IntentHash           string          `gorm:"column:intent_hash;not null;default:''"`
	State                AllocationState `gorm:"column:state;not null"`
	CurrentCPU           int64           `gorm:"column:current_cpu;not null;default:0"`
	CurrentMem           int64           `gorm:"column:current_mem;not null;default:0"`
	CurrentStorage       int64           `gorm:"column:current_storage;not null;default:0"`
	CurrentInstances     int64           `gorm:"column:current_instances;not null;default:0"`
	CurrentBackups       int64           `gorm:"column:current_backups;not null;default:0"`
	CurrentBackupStorage int64           `gorm:"column:current_backup_storage;not null;default:0"`
	Version              int64           `gorm:"column:version;not null;default:1"`
	ResourceRevision     int64           `gorm:"column:resource_revision;not null;default:0"`
	CloseIntentID        string          `gorm:"column:close_intent_id;not null;default:''"`
	ChangePlanID         string          `gorm:"column:change_plan_id;not null;default:''"`
	ChangeRevision       int64           `gorm:"column:change_revision;not null;default:0"`
	UpdatedAt            time.Time       `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (QuotaAllocationRecord) TableName() string { return "quota_allocations" }

// QuotaOperationRecord is the GORM persistence model for quota_operations.
type QuotaOperationRecord struct {
	OperationID   string         `gorm:"column:operation_id;primaryKey"`
	AllocationID  string         `gorm:"column:allocation_id;not null"`
	Kind          Kind           `gorm:"column:kind;not null"`
	ChangePlanID  string         `gorm:"column:change_plan_id;not null;default:''"`
	CloseIntentID string         `gorm:"column:close_intent_id;not null;default:''"`
	RequestHash   string         `gorm:"column:request_hash;not null"`
	Command       []byte         `gorm:"column:command;type:jsonb;not null"`
	State         OperationState `gorm:"column:state;not null;default:'Accepted'"`
	Result        []byte         `gorm:"column:result;type:jsonb"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (QuotaOperationRecord) TableName() string { return "quota_operations" }

// QuotaChangeRecord is the GORM persistence model for quota_changes.
type QuotaChangeRecord struct {
	ChangeID      string    `gorm:"column:change_id;primaryKey"`
	AllocationID  string    `gorm:"column:allocation_id;not null"`
	RequestHash   string    `gorm:"column:request_hash;not null"`
	Plan          []byte    `gorm:"column:plan;type:jsonb;not null"`
	SourceVersion int64     `gorm:"column:source_version;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;autoCreateTime"`
}

func (QuotaChangeRecord) TableName() string { return "quota_changes" }

// NodeAccountRecord is the GORM persistence model for node_accounts.
type NodeAccountRecord struct {
	NodeUID                string    `gorm:"column:node_uid;primaryKey"`
	PolicyVersion          int64     `gorm:"column:policy_version;not null;default:0"`
	LimitCPU               *int64    `gorm:"column:limit_cpu"`
	LimitMem               *int64    `gorm:"column:limit_mem"`
	LimitStorage           *int64    `gorm:"column:limit_storage"`
	LimitInstances         *int64    `gorm:"column:limit_instances"`
	LimitBackups           *int64    `gorm:"column:limit_backups"`
	LimitBackupStorage     *int64    `gorm:"column:limit_backup_storage"`
	AllocatedCPU           int64     `gorm:"column:allocated_cpu;not null;default:0"`
	AllocatedMem           int64     `gorm:"column:allocated_mem;not null;default:0"`
	AllocatedStorage       int64     `gorm:"column:allocated_storage;not null;default:0"`
	AllocatedInstances     int64     `gorm:"column:allocated_instances;not null;default:0"`
	AllocatedBackups       int64     `gorm:"column:allocated_backups;not null;default:0"`
	AllocatedBackupStorage int64     `gorm:"column:allocated_backup_storage;not null;default:0"`
	UpdatedAt              time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (NodeAccountRecord) TableName() string { return "node_accounts" }

// NodeReservationRecord is the GORM persistence model for node_reservations.
type NodeReservationRecord struct {
	AllocationID         string          `gorm:"column:allocation_id;primaryKey"`
	NodeUID              string          `gorm:"column:node_uid;not null"`
	TenantUID            string          `gorm:"column:tenant_uid;not null"`
	InstanceUID          string          `gorm:"column:instance_uid;not null"`
	InstanceName         string          `gorm:"column:instance_name;not null"`
	InitialIntent        []byte          `gorm:"column:initial_intent;type:jsonb"`
	IntentHash           string          `gorm:"column:intent_hash;not null;default:''"`
	AssignedPorts        []byte          `gorm:"column:assigned_ports;type:jsonb;not null;default:'[]'"`
	State                AllocationState `gorm:"column:state;not null"`
	CurrentCPU           int64           `gorm:"column:current_cpu;not null;default:0"`
	CurrentMem           int64           `gorm:"column:current_mem;not null;default:0"`
	CurrentStorage       int64           `gorm:"column:current_storage;not null;default:0"`
	CurrentInstances     int64           `gorm:"column:current_instances;not null;default:0"`
	CurrentBackups       int64           `gorm:"column:current_backups;not null;default:0"`
	CurrentBackupStorage int64           `gorm:"column:current_backup_storage;not null;default:0"`
	Version              int64           `gorm:"column:version;not null;default:1"`
	ResourceRevision     int64           `gorm:"column:resource_revision;not null;default:0"`
	CloseIntentID        string          `gorm:"column:close_intent_id;not null;default:''"`
	ChangePlanID         string          `gorm:"column:change_plan_id;not null;default:''"`
	ChangeRevision       int64           `gorm:"column:change_revision;not null;default:0"`
	UpdatedAt            time.Time       `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (NodeReservationRecord) TableName() string { return "node_reservations" }

// NodeOperationRecord is the GORM persistence model for node_operations.
type NodeOperationRecord struct {
	OperationID   string         `gorm:"column:operation_id;primaryKey"`
	AllocationID  string         `gorm:"column:allocation_id;not null"`
	Kind          Kind           `gorm:"column:kind;not null"`
	ChangePlanID  string         `gorm:"column:change_plan_id;not null;default:''"`
	CloseIntentID string         `gorm:"column:close_intent_id;not null;default:''"`
	RequestHash   string         `gorm:"column:request_hash;not null"`
	Command       []byte         `gorm:"column:command;type:jsonb;not null"`
	State         OperationState `gorm:"column:state;not null;default:'Accepted'"`
	Result        []byte         `gorm:"column:result;type:jsonb"`
	CreatedAt     time.Time      `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (NodeOperationRecord) TableName() string { return "node_operations" }

// NodeChangeRecord is the GORM persistence model for node_changes.
type NodeChangeRecord struct {
	ChangeID      string    `gorm:"column:change_id;primaryKey"`
	AllocationID  string    `gorm:"column:allocation_id;not null"`
	RequestHash   string    `gorm:"column:request_hash;not null"`
	Plan          []byte    `gorm:"column:plan;type:jsonb;not null"`
	SourceVersion int64     `gorm:"column:source_version;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;autoCreateTime"`
}

func (NodeChangeRecord) TableName() string { return "node_changes" }

// NodePortRecord is the GORM persistence model for node_ports.
type NodePortRecord struct {
	NodeUID       string `gorm:"column:node_uid;primaryKey"`
	Protocol      string `gorm:"column:protocol;primaryKey"`
	PublishedPort int32  `gorm:"column:published_port;primaryKey"`
	AttemptID     string `gorm:"column:attempt_id;not null"`
}

func (NodePortRecord) TableName() string { return "node_ports" }
