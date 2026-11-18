package driver

import (
	"context"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
)

// DefaultSteamExecutorDriver is the built-in OS subprocess SteamCMD runner driver name.
const DefaultSteamExecutorDriver = "steamcmd"

// BuiltPayload describes the materialized local artifact ready for S3 blob upload and Artifact status commit.
type BuiltPayload struct {
	LocalPath     string
	StagingDir    string
	FileName      string
	MediaType     string
	ArchiveFormat string
	TargetPath    string
	SizeBytes     int64
	SHA256Hex     string
	Build         string
	GameVersions  []string
	Loaders       []string
	ModIDs        []string
	MapFolders    []string
	Dependencies  []registryv1.PackageDependency
	ConfigSchema  []registryv1.AddonConfigField
	PublishedAt   *time.Time
	Changelog     string
	UpstreamURL   string
}

// Reporter streams stage transitions, transfer speeds, and structured logs for a claimed artifact sync.
type Reporter interface {
	ResetLogs()
	Logf(format string, args ...any)
	LogLine(raw string)
	Stage(stage registryv1.ArtifactSyncStage, pct int32, msg string)
	Transfer(stage registryv1.ArtifactSyncStage, pct int32, processed, total, speed int64, msg string)
}

// SteamHooks exposes lifecycle callbacks invoked while waiting for, acquiring, and streaming a SteamCMD download slot.
type SteamHooks struct {
	OnWait     func()
	OnAcquired func()
	OnLine     func(string)
}

// SteamExecutor abstracts SteamCMD execution so unit and integration tests can mock pulls without a Steam binary.
type SteamExecutor interface {
	Run(ctx context.Context, installDir string, args []string, hooks SteamHooks) error
}

// SteamExecutorFactory constructs a SteamExecutor bound to the worker's registry configuration.
type SteamExecutorFactory func(cfg config.RegistryConfig) SteamExecutor

// Runtime exposes worker storage, HTTP transfer, and reuse primitives to SyncDriver plugins without import cycles.
type Runtime interface {
	WorkDir() string
	SteamExec() SteamExecutor
	ReusableArtifactPayload(ctx context.Context, rep Reporter, art *registryv1.Artifact, buildID string, pubAt *time.Time, upstreamURL string) *BuiltPayload
	DownloadHTTPPayload(ctx context.Context, rep Reporter, svc *addonmirror.Service, art *registryv1.Artifact, rel *addonmirror.ResolvedRelease, tmpDir string) (*BuiltPayload, error)
	InspectCachedWorkshopPayload(ctx context.Context, rep Reporter, art *registryv1.Artifact, steamAppID int, rel *addonmirror.ResolvedRelease, tmpDir string) *BuiltPayload
}

// SyncRequest provides all resolved repository, upstream, mirror, and progress state to a SyncDriver.
type SyncRequest struct {
	Runtime  Runtime
	Reporter Reporter
	Mirror   *addonmirror.Service
	Source   addonmirror.Source
	Artifact *registryv1.Artifact
	Repo     *registryv1.Repository
	Upstream *registryv1.Upstream
	TmpDir   string
}

// SyncDriver synchronizes one upstream/protocol flavor into a local BuiltPayload ready for S3 blob upload.
type SyncDriver interface {
	Name() string
	Sync(ctx context.Context, req SyncRequest) (*BuiltPayload, error)
}

// WorkshopInspection holds metadata and addon configuration schema extracted from workshop files.
type WorkshopInspection struct {
	ModIDs       []string
	MapFolders   []string
	ConfigSchema []registryv1.AddonConfigField
}

// WorkshopInspector extracts structured mod IDs, map folders, and config schemas from downloaded workshop content.
type WorkshopInspector interface {
	InspectDir(contentDir string) WorkshopInspection
	InspectArchive(archiveFormat, archivePath string) WorkshopInspection
}
