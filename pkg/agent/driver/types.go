package driver

import (
	"os"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

// CachedArtifactMeta records verified metadata for an Artifact materialized in <workDir>/.blob-cache.
type CachedArtifactMeta struct {
	Coordinate    string                        `json:"coordinate"`
	Digest        string                        `json:"digest"`
	HexDigest     string                        `json:"hexDigest"`
	SizeBytes     int64                         `json:"sizeBytes"`
	ArchiveFormat string                        `json:"archiveFormat"`
	FileName      string                        `json:"fileName,omitempty"`
	TargetPath    string                        `json:"targetPath,omitempty"`
	ModIDs        []string                      `json:"modIds,omitempty"`
	MapFolders    []string                      `json:"mapFolders,omitempty"`
	ConfigSchema  []registryv1.AddonConfigField `json:"configSchema,omitempty"`
}

// AddonsManifest tracks materialized addon files, directories, and active modes inside an instance workspace.
type AddonsManifest struct {
	Coordinates []string          `json:"coordinates,omitempty"`
	Digests     map[string]string `json:"digests,omitempty"`
	Files       []string          `json:"files,omitempty"`
	Dirs        []string          `json:"dirs,omitempty"`
	Modes       []string          `json:"modes,omitempty"`
	TModTargets []string          `json:"tmodTargets,omitempty"`
	MapFolders  []string          `json:"mapFolders,omitempty"`
}

// WorkspaceContext carries mutable materialization state for one instance workspace across addon injections.
type WorkspaceContext struct {
	Root             *os.Root
	RequireReflink   bool
	DisableReflink   bool
	ServerReplaced   bool
	Prev             AddonsManifest
	Next             *AddonsManifest
	BlobFilePath     func(hexDigest string) string
	BlobExtractedDir func(hexDigest string) string
	State            map[string]any
}

// AddonInjector injects, finalizes, and cleans up addons for a single installMode in an instance workspace.
type AddonInjector interface {
	Mode() string
	Inject(ws *WorkspaceContext, addon corev1.InstanceAddon, meta *CachedArtifactMeta) error
	Finalize(ws *WorkspaceContext) error
	Cleanup(root *os.Root, prev AddonsManifest)
}

// BaseInjector provides default no-op Finalize and Cleanup hooks for simple injectors.
type BaseInjector struct{}

// Finalize is a no-op default hook when an injector requires no post-injection manifest generation.
func (BaseInjector) Finalize(*WorkspaceContext) error { return nil }

// Cleanup is a no-op default hook when removing addons leaves no extra generated config files.
func (BaseInjector) Cleanup(*os.Root, AddonsManifest) {}
