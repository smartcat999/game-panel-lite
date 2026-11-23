package v1

import (
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	GroupName    = "registry.gamepanel.io"
	GroupVersion = "registry.gamepanel.io/v1"

	KindUpstream          = "Upstream"
	KindRepository        = "Repository"
	KindPackage           = "Package"
	KindArtifact          = "Artifact"
	KindArtifactLogChunk  = "ArtifactLogChunk"
	KindBlob              = "Blob"
	KindBlobUpload        = "BlobUpload"
	KindReplicationPolicy = "ReplicationPolicy"

	SubresourceLog = "log"

	TopicRepositories = "registry.v1.repositories"
	TopicArtifacts    = "registry.v1.artifacts"
	TopicReplication  = "registry.v1.replicationpolicies"

	FinalizerRepositoryCleanup   = "registry.gamepanel.io/repository-cleanup"
	FinalizerArtifactBlobRef     = "registry.gamepanel.io/blob-ref"
	AnnotationBoundBlobDigest    = "registry.gamepanel.io/bound-blob-digest"
	AnnotationSyncClaimedBy      = "registry.gamepanel.io/sync-claimed-by"
	AnnotationSyncLeaseUntil     = "registry.gamepanel.io/sync-lease-until"
	AnnotationRetryRequestedAt   = "registry.gamepanel.io/retry-requested-at"
	AnnotationHydrateRequestedAt = "registry.gamepanel.io/hydrate-requested-at"
	AnnotationSteamPlatforms     = "registry.gamepanel.io/steam-platforms"

	PackageSourceSteamCMDScheme = "steamcmd://"
	PrincipalRegistryWorker     = "system:registry-worker"

	UpstreamProviderPaperMC       = "papermc"
	UpstreamProviderPurpur        = "purpur"
	UpstreamProviderVanilla       = "vanilla"
	UpstreamProviderFabricMC      = "fabricmc"
	UpstreamProviderNeoForge      = "neoforge"
	UpstreamProviderModrinth      = "modrinth"
	UpstreamProviderThunderstore  = "thunderstore"
	UpstreamProviderUMod          = "umod"
	UpstreamProviderFicsit        = "ficsit"
	UpstreamProviderCS2Plugins    = "cs2plugins"
	UpstreamProviderSevenDaysMods = "sevendaysmods"
	UpstreamProviderModIO         = "modio"
	UpstreamProviderSteam         = "steam"
	UpstreamProviderHTTP          = "http"

	RepositoryModeHosted  = "Hosted"
	RepositoryModeProxy   = "Proxy"
	RepositoryModeVirtual = "Virtual"

	FormatServerPackage   = "server_package"
	FormatWorkshopMod     = "workshop_mod"
	FormatModrinthAddon   = "modrinth_addon"
	FormatThunderstoreMod = "thunderstore_mod"
	FormatUModPlugin      = "umod_plugin"
	FormatFicsitMod       = "ficsit_mod"
	FormatCS2Plugin       = "cs2_plugin"
	FormatSevenDaysMod    = "sevendays_mod"
	FormatModIOMod        = "modio_mod"
	FormatGeneric         = "generic"

	ArchiveFormatRaw   = "raw"
	ArchiveFormatTarGz = "tar.gz"
	ArchiveFormatZip   = "zip"
	ArchiveFormatJar   = "jar"
	ArchiveFormatTMod  = "tmod"

	ModrinthProjectPlugin = "plugin"
	ModrinthProjectMod    = "mod"

	ModrinthLoaderPaper    = "paper"
	ModrinthLoaderFabric   = "fabric"
	ModrinthLoaderNeoForge = "neoforge"

	CatalogSortPopular   = "popular"
	CatalogSortDownloads = "downloads"
	CatalogSortTrend     = "trend"
	CatalogSortTopRated  = "toprated"
	CatalogSortNewest    = "newest"
	CatalogSortUpdated   = "updated"

	ReplicationModeTrackReleases    = "TrackReleases"
	ReplicationModeTopPopular       = "TopPopular"
	ReplicationModeExplicitPackages = "ExplicitPackages"

	ConditionReasonReady        = "Ready"
	ConditionReasonPending      = "Pending"
	ConditionReasonSyncing      = "Syncing"
	ConditionReasonSyncFailed   = "SyncFailed"
	ConditionReasonUpstreamDown = "UpstreamDegraded"
	ConditionReasonDeleting     = "Deleting"
)

type RepositoryPhase string

const (
	RepositoryPhaseActive   RepositoryPhase = "Active"
	RepositoryPhaseDegraded RepositoryPhase = "Degraded"
	RepositoryPhaseDeleting RepositoryPhase = "Deleting"
)

type ArtifactPhase string

const (
	ArtifactPhasePending  ArtifactPhase = "Pending"
	ArtifactPhaseSyncing  ArtifactPhase = "Syncing"
	ArtifactPhaseReady    ArtifactPhase = "Ready"
	ArtifactPhaseFailed   ArtifactPhase = "Failed"
	ArtifactPhaseDeleting ArtifactPhase = "Deleting"
)

type ArtifactSyncStage string

const (
	ArtifactStageQueued      ArtifactSyncStage = "Queued"
	ArtifactStageResolving   ArtifactSyncStage = "Resolving"
	ArtifactStageDownloading ArtifactSyncStage = "Downloading"
	ArtifactStageArchiving   ArtifactSyncStage = "Archiving"
	ArtifactStageUploading   ArtifactSyncStage = "Uploading"
	ArtifactStageCompleted   ArtifactSyncStage = "Completed"
	ArtifactStageFailed      ArtifactSyncStage = "Failed"
)

type ArtifactLogAction string

const (
	ArtifactLogActionReset    ArtifactLogAction = "reset"
	ArtifactLogActionAppend   ArtifactLogAction = "append"
	ArtifactLogActionComplete ArtifactLogAction = "complete"
)

type BlobUploadState string

const (
	BlobUploadStateUploading BlobUploadState = "Uploading"
	BlobUploadStateCompleted BlobUploadState = "Completed"
	BlobUploadStateAborted   BlobUploadState = "Aborted"
)

type AddonConfigFieldType string

const (
	AddonConfigTypeBoolean AddonConfigFieldType = "boolean"
	AddonConfigTypeNumber  AddonConfigFieldType = "number"
	AddonConfigTypeSelect  AddonConfigFieldType = "select"
	AddonConfigTypeText    AddonConfigFieldType = "text"
)

type AddonConfigValueType string

const (
	AddonConfigValueBoolean AddonConfigValueType = "boolean"
	AddonConfigValueNumber  AddonConfigValueType = "number"
	AddonConfigValueString  AddonConfigValueType = "string"
)

var (
	dnsLabelRegex    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	coordUnsafeRegex = regexp.MustCompile(`[^A-Za-z0-9._+-]+`)
)

// NormalizeDigest validates a SHA-256 digest and returns canonical "sha256:<64-lowercase-hex>".
func NormalizeDigest(raw string) (string, string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	hexPart := trimmed
	switch {
	case strings.HasPrefix(trimmed, "sha256:"):
		hexPart = strings.TrimPrefix(trimmed, "sha256:")
	case strings.HasPrefix(trimmed, "sha256-"):
		hexPart = strings.TrimPrefix(trimmed, "sha256-")
	}
	if len(hexPart) != 64 {
		return "", "", fmt.Errorf("invalid sha256 digest %q: expected 64 hex chars", raw)
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", "", fmt.Errorf("invalid sha256 digest %q: %w", raw, err)
	}
	return "sha256:" + hexPart, hexPart, nil
}

// BlobObjectKey maps a SHA-256 digest to its canonical S3 content-addressed object key.
func BlobObjectKey(digest string) (string, error) {
	_, hexPart, err := NormalizeDigest(digest)
	if err != nil {
		return "", err
	}
	return "blobs/sha256/" + hexPart[:2] + "/" + hexPart, nil
}

// FormatPackageName builds deterministic "<repository>.<package>" coordinates.
func FormatPackageName(repository, pkg string) string {
	repo := strings.ToLower(strings.TrimSpace(repository))
	cleanPkg := sanitizeCoord(pkg, true)
	if repo == "" || cleanPkg == "" {
		return ""
	}
	return repo + "." + cleanPkg
}

// ParsePackageName splits "<repository>.<package>" on the first dot.
func ParsePackageName(name string) (string, string, error) {
	idx := strings.IndexByte(strings.TrimSpace(name), '.')
	if idx <= 0 || idx >= len(strings.TrimSpace(name))-1 {
		return "", "", errors.New("package metadata.name must be formatted as <repository>.<package>")
	}
	trimmed := strings.TrimSpace(name)
	return trimmed[:idx], trimmed[idx+1:], nil
}

// FormatArtifactName builds deterministic "<repository>.<package>.<version>" coordinates.
func FormatArtifactName(repository, pkg, version string) string {
	repo := strings.ToLower(strings.TrimSpace(repository))
	cleanPkg := sanitizeCoord(pkg, true)
	cleanVer := sanitizeCoord(version, false)
	if repo == "" || cleanPkg == "" || cleanVer == "" {
		return ""
	}
	return repo + "." + cleanPkg + "." + cleanVer
}

// ParseArtifactName splits "<repository>.<package>.<version>" into its 3 coordinates.
func ParseArtifactName(name string) (string, string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(name), ".", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", errors.New("artifact metadata.name must be formatted as <repository>.<package>.<version>")
	}
	return parts[0], parts[1], parts[2], nil
}

func sanitizeCoord(raw string, replaceDots bool) string {
	val := strings.TrimSpace(raw)
	if replaceDots {
		val = strings.ReplaceAll(val, ".", "-")
	}
	val = coordUnsafeRegex.ReplaceAllString(val, "-")
	return strings.Trim(val, "-")
}
