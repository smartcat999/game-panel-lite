package v1

import (
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

type UpstreamRecord struct {
	Name                string            `gorm:"column:name;primaryKey"`
	UID                 string            `gorm:"column:uid;unique;not null"`
	Provider            string            `gorm:"column:provider;not null"`
	Endpoint            string            `gorm:"column:endpoint;not null;default:''"`
	ProxyURL            string            `gorm:"column:proxy_url;not null;default:''"`
	SteamAppID          int               `gorm:"column:steam_app_id;not null;default:0"`
	ModrinthLoader      string            `gorm:"column:modrinth_loader;not null;default:''"`
	ModrinthProjectType string            `gorm:"column:modrinth_project_type;not null;default:''"`
	APIKey              string            `gorm:"column:api_key;not null;default:''"`
	RateLimitRPS        int32             `gorm:"column:rate_limit_rps;not null;default:0"`
	ResourceVersion     int64             `gorm:"column:resource_version;not null;default:1"`
	Generation          int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt           time.Time         `gorm:"column:created_at;not null"`
	DeletedAt           *time.Time        `gorm:"column:deleted_at"`
	Finalizers          []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels              map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations         map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (UpstreamRecord) TableName() string { return "registry_upstreams" }

type RepositoryRecord struct {
	Name            string                     `gorm:"column:name;primaryKey"`
	UID             string                     `gorm:"column:uid;unique;not null"`
	DisplayName     string                     `gorm:"column:display_name;not null;default:''"`
	Description     string                     `gorm:"column:description;not null;default:''"`
	Mode            string                     `gorm:"column:mode;not null;default:'Hosted'"`
	Format          string                     `gorm:"column:format;not null;default:'generic'"`
	UpstreamName    string                     `gorm:"column:upstream_name;not null;default:''"`
	MemberRepos     []string                   `gorm:"column:member_repos;serializer:json;type:jsonb;not null;default:'[]'"`
	PublicRead      bool                       `gorm:"column:public_read;not null;default:false"`
	RetentionPolicy *RepositoryRetentionPolicy `gorm:"column:retention_policy;serializer:json;type:jsonb"`
	ResourceVersion int64                      `gorm:"column:resource_version;not null;default:1"`
	Generation      int64                      `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time                  `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time                 `gorm:"column:deleted_at"`
	Finalizers      []string                   `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string          `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string          `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (RepositoryRecord) TableName() string { return "registry_repositories" }

type RepositoryStateRecord struct {
	RepositoryUID      string             `gorm:"column:repository_uid;primaryKey"`
	Version            int64              `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64              `gorm:"column:observed_generation;not null;default:0"`
	Phase              string             `gorm:"column:phase;not null;default:'Active'"`
	PackageCount       int64              `gorm:"column:package_count;not null;default:0"`
	ArtifactCount      int64              `gorm:"column:artifact_count;not null;default:0"`
	StorageBytes       int64              `gorm:"column:storage_bytes;not null;default:0"`
	LastSyncedAt       *time.Time         `gorm:"column:last_synced_at"`
	Conditions         []metav1.Condition `gorm:"column:conditions;serializer:json;type:jsonb;not null;default:'[]'"`
	UpdatedAt          time.Time          `gorm:"column:updated_at;not null"`
}

func (RepositoryStateRecord) TableName() string { return "registry_repository_states" }

type PackageRecord struct {
	Name            string              `gorm:"column:name;primaryKey"`
	UID             string              `gorm:"column:uid;unique;not null"`
	Repository      string              `gorm:"column:repository;not null;index:idx_registry_packages_repo_pkg,unique"`
	Package         string              `gorm:"column:package;not null;index:idx_registry_packages_repo_pkg,unique"`
	Title           string              `gorm:"column:title;not null"`
	Summary         string              `gorm:"column:summary;not null;default:''"`
	Description     string              `gorm:"column:description;not null;default:''"`
	Author          string              `gorm:"column:author;not null;default:''"`
	IconURL         string              `gorm:"column:icon_url;not null;default:''"`
	GalleryURLs     []string            `gorm:"column:gallery_urls;serializer:json;type:jsonb;not null;default:'[]'"`
	SourceURL       string              `gorm:"column:source_url;not null;default:''"`
	Downloads       int64               `gorm:"column:downloads;not null;default:0"`
	SizeBytes       int64               `gorm:"column:size_bytes;not null;default:0"`
	LatestVersion   string              `gorm:"column:latest_version;not null;default:''"`
	UpdatedAtStr    string              `gorm:"column:updated_at_str;not null;default:''"`
	Tags            []string            `gorm:"column:tags;serializer:json;type:jsonb;not null;default:'[]'"`
	ModIDs          []string            `gorm:"column:mod_ids;serializer:json;type:jsonb;not null;default:'[]'"`
	MapFolders      []string            `gorm:"column:map_folders;serializer:json;type:jsonb;not null;default:'[]'"`
	Dependencies    []PackageDependency `gorm:"column:dependencies;serializer:json;type:jsonb;not null;default:'[]'"`
	ConfigSchema    []AddonConfigField  `gorm:"column:config_schema;serializer:json;type:jsonb;not null;default:'[]'"`
	ResourceVersion int64               `gorm:"column:resource_version;not null;default:1"`
	Generation      int64               `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time           `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time          `gorm:"column:deleted_at"`
	Finalizers      []string            `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string   `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string   `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (PackageRecord) TableName() string { return "registry_packages" }

type ArtifactRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Repository      string            `gorm:"column:repository;not null;index:idx_registry_artifacts_coord,unique"`
	Package         string            `gorm:"column:package;not null;index:idx_registry_artifacts_coord,unique"`
	Version         string            `gorm:"column:version;not null;index:idx_registry_artifacts_coord,unique"`
	SourceURL       string            `gorm:"column:source_url;not null;default:''"`
	ArchiveFormat   string            `gorm:"column:archive_format;not null;default:'raw'"`
	TargetPath      string            `gorm:"column:target_path;not null;default:''"`
	GameVersions    []string          `gorm:"column:game_versions;serializer:json;type:jsonb;not null;default:'[]'"`
	Loaders         []string          `gorm:"column:loaders;serializer:json;type:jsonb;not null;default:'[]'"`
	SpecDescriptor  *BlobDescriptor   `gorm:"column:spec_descriptor;serializer:json;type:jsonb"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (ArtifactRecord) TableName() string { return "registry_artifacts" }

type ArtifactStateRecord struct {
	ArtifactUID        string              `gorm:"column:artifact_uid;primaryKey"`
	Version            int64               `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64               `gorm:"column:observed_generation;not null;default:0"`
	Phase              string              `gorm:"column:phase;not null;default:'Pending'"`
	Stage              string              `gorm:"column:stage;not null;default:''"`
	Progress           int32               `gorm:"column:progress;not null;default:0"`
	ProcessedBytes     int64               `gorm:"column:processed_bytes;not null;default:0"`
	TotalBytes         int64               `gorm:"column:total_bytes;not null;default:0"`
	SpeedBytesPerSec   int64               `gorm:"column:speed_bytes_per_sec;not null;default:0"`
	StartedAt          *time.Time          `gorm:"column:started_at"`
	Build              string              `gorm:"column:build;not null;default:''"`
	Descriptor         *BlobDescriptor     `gorm:"column:descriptor;serializer:json;type:jsonb"`
	BlobDigest         string              `gorm:"column:blob_digest;not null;default:''"`
	ModIDs             []string            `gorm:"column:mod_ids;serializer:json;type:jsonb;not null;default:'[]'"`
	MapFolders         []string            `gorm:"column:map_folders;serializer:json;type:jsonb;not null;default:'[]'"`
	Dependencies       []PackageDependency `gorm:"column:dependencies;serializer:json;type:jsonb;not null;default:'[]'"`
	ConfigSchema       []AddonConfigField  `gorm:"column:config_schema;serializer:json;type:jsonb;not null;default:'[]'"`
	PublishedAt        *time.Time          `gorm:"column:published_at"`
	Changelog          string              `gorm:"column:changelog;not null;default:''"`
	UpstreamURL        string              `gorm:"column:upstream_url;not null;default:''"`
	SyncedAt           *time.Time          `gorm:"column:synced_at"`
	Message            string              `gorm:"column:message;not null;default:''"`
	Conditions         []metav1.Condition  `gorm:"column:conditions;serializer:json;type:jsonb;not null;default:'[]'"`
	UpdatedAt          time.Time           `gorm:"column:updated_at;not null"`
}

func (ArtifactStateRecord) TableName() string { return "registry_artifact_states" }

type BlobRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Digest          string            `gorm:"column:digest;unique;not null"`
	SizeBytes       int64             `gorm:"column:size_bytes;not null;default:0"`
	MediaType       string            `gorm:"column:media_type;not null;default:'application/octet-stream'"`
	ObjectKey       string            `gorm:"column:object_key;not null"`
	RefCount        int64             `gorm:"column:ref_count;not null;default:0"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (BlobRecord) TableName() string { return "registry_blobs" }

type BlobRefRecord struct {
	Digest    string    `gorm:"column:digest;primaryKey"`
	HolderUID string    `gorm:"column:holder_uid;primaryKey"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (BlobRefRecord) TableName() string { return "registry_blob_refs" }

type BlobUploadRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Digest          string            `gorm:"column:digest;not null;default:''"`
	SizeBytes       int64             `gorm:"column:size_bytes;not null;default:0"`
	MediaType       string            `gorm:"column:media_type;not null;default:'application/octet-stream'"`
	FileName        string            `gorm:"column:file_name;not null;default:''"`
	ObjectKey       string            `gorm:"column:object_key;not null;default:''"`
	UploadID        string            `gorm:"column:upload_id;not null;default:''"`
	PartSize        int64             `gorm:"column:part_size;not null;default:0"`
	Parts           []BlobUploadPart  `gorm:"column:parts;serializer:json;type:jsonb;not null;default:'[]'"`
	PresignedURL    string            `gorm:"column:presigned_url;not null;default:''"`
	ExpiresAt       *time.Time        `gorm:"column:expires_at"`
	UploadState     string            `gorm:"column:upload_state;not null;default:'Uploading'"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (BlobUploadRecord) TableName() string { return "registry_blob_uploads" }

type ReplicationPolicyRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Repository      string            `gorm:"column:repository;not null;index:idx_registry_rep_policies_repo"`
	Enabled         bool              `gorm:"column:enabled;not null;default:false"`
	Mode            string            `gorm:"column:mode;not null;default:'TrackReleases'"`
	IntervalMinutes int32             `gorm:"column:interval_minutes;not null;default:60"`
	TopN            int32             `gorm:"column:top_n;not null;default:0"`
	Packages        []string          `gorm:"column:packages;serializer:json;type:jsonb;not null;default:'[]'"`
	GameVersion     string            `gorm:"column:game_version;not null;default:''"`
	LastRunAt       *time.Time        `gorm:"column:last_run_at"`
	LastMessage     string            `gorm:"column:last_message;not null;default:''"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null;default:'[]'"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null;default:'{}'"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null;default:'{}'"`
}

func (ReplicationPolicyRecord) TableName() string { return "registry_replication_policies" }
