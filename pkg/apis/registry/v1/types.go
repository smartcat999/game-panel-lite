package v1

import (
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// Upstream defines external source connectivity, proxy routing, and credentials (flattened resource).
type Upstream struct {
	metav1.TypeMeta     `json:",inline"`
	Metadata            metav1.ObjectMeta `json:"metadata"`
	Provider            string            `json:"provider"` // papermc | purpur | vanilla | fabricmc | neoforge | modrinth | thunderstore | umod | steam | http
	Endpoint            string            `json:"endpoint,omitempty"`
	ProxyURL            string            `json:"proxyURL,omitempty"`
	SteamAppID          int               `json:"steamAppId,omitempty"`
	ModrinthLoader      string            `json:"modrinthLoader,omitempty"`
	ModrinthProjectType string            `json:"modrinthProjectType,omitempty"`
	APIKey              string            `json:"apiKey,omitempty"`
	RateLimitRPS        int32             `json:"rateLimitRps,omitempty"`
}

func (u *Upstream) GetTypeMeta() *metav1.TypeMeta     { return &u.TypeMeta }
func (u *Upstream) GetObjectMeta() *metav1.ObjectMeta { return &u.Metadata }

// RepositoryRetentionPolicy governs automatic cleanup of unpinned cached versions in a Repository.
type RepositoryRetentionPolicy struct {
	MaxVersionsPerPackage int32 `json:"maxVersionsPerPackage,omitempty"`
	MaxAgeDays            int32 `json:"maxAgeDays,omitempty"`
}

type RepositorySpec struct {
	DisplayName     string                     `json:"displayName,omitempty"`
	Description     string                     `json:"description,omitempty"`
	Mode            string                     `json:"mode"`   // Hosted | Proxy | Virtual
	Format          string                     `json:"format"` // server_package | workshop_mod | modrinth_addon | thunderstore_mod | umod_plugin | generic
	UpstreamName    string                     `json:"upstreamName,omitempty"`
	MemberRepos     []string                   `json:"memberRepos,omitempty"`
	PublicRead      bool                       `json:"publicRead,omitempty"`
	RetentionPolicy *RepositoryRetentionPolicy `json:"retentionPolicy,omitempty"`
}

type RepositoryStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	Phase              RepositoryPhase    `json:"phase"`
	PackageCount       int64              `json:"packageCount"`
	ArtifactCount      int64              `json:"artifactCount"`
	StorageBytes       int64              `json:"storageBytes"`
	LastSyncedAt       *time.Time         `json:"lastSyncedAt,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// Repository is an asynchronous state-machine resource representing a Hosted, Proxy, or Virtual registry repository.
type Repository struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            RepositorySpec    `json:"spec"`
	Status          RepositoryStatus  `json:"status,omitempty"`
}

func (r *Repository) GetTypeMeta() *metav1.TypeMeta     { return &r.TypeMeta }
func (r *Repository) GetObjectMeta() *metav1.ObjectMeta { return &r.Metadata }

// PackageDependency describes a required upstream package dependency.
type PackageDependency struct {
	Package string `json:"package"`
	Title   string `json:"title,omitempty"`
}

// AddonConfigOption describes one selectable choice for a mod/plugin configuration option.
type AddonConfigOption struct {
	Label       string               `json:"label"`
	Value       string               `json:"value"`
	ValueType   AddonConfigValueType `json:"valueType,omitempty"`
	Description string               `json:"description,omitempty"`
}

// AddonConfigField describes one typed configuration field extracted from a mod/plugin artifact.
type AddonConfigField struct {
	Name        string               `json:"name"`
	Label       string               `json:"label"`
	Description string               `json:"description,omitempty"`
	Type        AddonConfigFieldType `json:"type"`
	Default     string               `json:"default,omitempty"`
	DefaultType AddonConfigValueType `json:"defaultType,omitempty"`
	Min         *float64             `json:"min,omitempty"`
	Max         *float64             `json:"max,omitempty"`
	Step        *float64             `json:"step,omitempty"`
	Options     []AddonConfigOption  `json:"options,omitempty"`
}

// Package represents a logical server or addon entry inside a Repository (flattened resource).
type Package struct {
	metav1.TypeMeta   `json:",inline"`
	Metadata          metav1.ObjectMeta   `json:"metadata"`
	Repository        string              `json:"repository"`
	Package           string              `json:"package"`
	Title             string              `json:"title"`
	Summary           string              `json:"summary,omitempty"`
	Description       string              `json:"description,omitempty"`
	Author            string              `json:"author,omitempty"`
	IconURL           string              `json:"iconUrl,omitempty"`
	GalleryURLs       []string            `json:"galleryUrls,omitempty"`
	SourceURL         string              `json:"sourceUrl,omitempty"`
	Downloads         int64               `json:"downloads,omitempty"`
	SizeBytes         int64               `json:"sizeBytes,omitempty"`
	LatestVersion     string              `json:"latestVersion,omitempty"`
	AvailableVersions []string            `json:"availableVersions,omitempty"`
	UpdatedAt         string              `json:"updatedAt,omitempty"`
	Tags              []string            `json:"tags,omitempty"`
	ModIDs            []string            `json:"modIds,omitempty"`
	MapFolders        []string            `json:"mapFolders,omitempty"`
	Dependencies      []PackageDependency `json:"dependencies,omitempty"`
	ConfigSchema      []AddonConfigField  `json:"configSchema,omitempty"`
}

func (p *Package) GetTypeMeta() *metav1.TypeMeta     { return &p.TypeMeta }
func (p *Package) GetObjectMeta() *metav1.ObjectMeta { return &p.Metadata }

// BlobDescriptor identifies an immutable content-addressed physical payload in S3.
type BlobDescriptor struct {
	Digest        string `json:"digest"`
	SizeBytes     int64  `json:"sizeBytes"`
	MediaType     string `json:"mediaType,omitempty"`
	ArchiveFormat string `json:"archiveFormat,omitempty"` // raw | tar.gz | zip | jar | tmod
	FileName      string `json:"fileName,omitempty"`
}

type ArtifactSpec struct {
	Repository    string          `json:"repository"`
	Package       string          `json:"package"`
	Version       string          `json:"version"`
	SourceURL     string          `json:"sourceURL,omitempty"`
	ArchiveFormat string          `json:"archiveFormat,omitempty"`
	TargetPath    string          `json:"targetPath,omitempty"`
	GameVersions  []string        `json:"gameVersions,omitempty"`
	Loaders       []string        `json:"loaders,omitempty"`
	Descriptor    *BlobDescriptor `json:"descriptor,omitempty"`
}

type ArtifactStatus struct {
	ObservedGeneration int64               `json:"observedGeneration"`
	Phase              ArtifactPhase       `json:"phase"`
	Stage              ArtifactSyncStage   `json:"stage,omitempty"`
	Progress           int32               `json:"progress,omitempty"`
	ProcessedBytes     int64               `json:"processedBytes,omitempty"`
	TotalBytes         int64               `json:"totalBytes,omitempty"`
	SpeedBytesPerSec   int64               `json:"speedBytesPerSec,omitempty"`
	StartedAt          *time.Time          `json:"startedAt,omitempty"`
	Build              string              `json:"build,omitempty"`
	Descriptor         *BlobDescriptor     `json:"descriptor,omitempty"`
	ModIDs             []string            `json:"modIds,omitempty"`
	MapFolders         []string            `json:"mapFolders,omitempty"`
	Dependencies       []PackageDependency `json:"dependencies,omitempty"`
	ConfigSchema       []AddonConfigField  `json:"configSchema,omitempty"`
	PublishedAt        *time.Time          `json:"publishedAt,omitempty"`
	Changelog          string              `json:"changelog,omitempty"`
	UpstreamURL        string              `json:"upstreamURL,omitempty"`
	SyncedAt           *time.Time          `json:"syncedAt,omitempty"`
	Message            string              `json:"message,omitempty"`
	Conditions         []metav1.Condition  `json:"conditions,omitempty"`
}

// Artifact is an asynchronous state-machine resource representing an immutable versioned release.
type Artifact struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            ArtifactSpec      `json:"spec"`
	Status          ArtifactStatus    `json:"status,omitempty"`
}

func (a *Artifact) GetTypeMeta() *metav1.TypeMeta     { return &a.TypeMeta }
func (a *Artifact) GetObjectMeta() *metav1.ObjectMeta { return &a.Metadata }

// ArtifactLogChunk carries log streaming actions appended to or resetting an Artifact task log.
type ArtifactLogChunk struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Action          ArtifactLogAction `json:"action"`
	Lines           []string          `json:"lines,omitempty"`
}

func (l *ArtifactLogChunk) GetTypeMeta() *metav1.TypeMeta     { return &l.TypeMeta }
func (l *ArtifactLogChunk) GetObjectMeta() *metav1.ObjectMeta { return &l.Metadata }

// Blob represents a deduplicated content-addressed S3 object (flattened resource).
type Blob struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Digest          string            `json:"digest"`
	SizeBytes       int64             `json:"sizeBytes"`
	MediaType       string            `json:"mediaType,omitempty"`
	ObjectKey       string            `json:"objectKey"`
	RefCount        int64             `json:"refCount"`
}

func (b *Blob) GetTypeMeta() *metav1.TypeMeta     { return &b.TypeMeta }
func (b *Blob) GetObjectMeta() *metav1.ObjectMeta { return &b.Metadata }

// BlobUploadPart describes a single S3 multipart upload part.
type BlobUploadPart struct {
	PartNumber int    `json:"partNumber"`
	URL        string `json:"url,omitempty"`
	ETag       string `json:"etag,omitempty"`
}

// BlobUpload represents a direct S3 presigned single/multipart upload session (flattened resource).
type BlobUpload struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Digest          string            `json:"digest,omitempty"`
	SizeBytes       int64             `json:"sizeBytes"`
	MediaType       string            `json:"mediaType,omitempty"`
	FileName        string            `json:"fileName,omitempty"`
	ObjectKey       string            `json:"objectKey,omitempty"`
	UploadID        string            `json:"uploadID,omitempty"`
	PartSize        int64             `json:"partSize,omitempty"`
	Parts           []BlobUploadPart  `json:"parts,omitempty"`
	PresignedURL    string            `json:"presignedURL,omitempty"`
	ExpiresAt       *time.Time        `json:"expiresAt,omitempty"`
	UploadState     BlobUploadState   `json:"uploadState,omitempty"`
}

func (u *BlobUpload) GetTypeMeta() *metav1.TypeMeta     { return &u.TypeMeta }
func (u *BlobUpload) GetObjectMeta() *metav1.ObjectMeta { return &u.Metadata }

// ReplicationPolicy configures proactive upstream-to-S3 synchronization (flattened resource).
type ReplicationPolicy struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Repository      string            `json:"repository"`
	Enabled         bool              `json:"enabled"`
	Mode            string            `json:"mode"` // TrackReleases | TopPopular | ExplicitPackages
	IntervalMinutes int32             `json:"intervalMinutes,omitempty"`
	TopN            int32             `json:"topN,omitempty"`
	Packages        []string          `json:"packages,omitempty"`
	GameVersion     string            `json:"gameVersion,omitempty"`
	LastRunAt       *time.Time        `json:"lastRunAt,omitempty"`
	LastMessage     string            `json:"lastMessage,omitempty"`
}

func (p *ReplicationPolicy) GetTypeMeta() *metav1.TypeMeta     { return &p.TypeMeta }
func (p *ReplicationPolicy) GetObjectMeta() *metav1.ObjectMeta { return &p.Metadata }
