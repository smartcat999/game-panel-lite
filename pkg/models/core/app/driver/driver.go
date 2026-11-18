package driver

import (
	"sort"
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// MaxExtractedBackupAddons bounds the number of addon records extracted from a single archive.
const MaxExtractedBackupAddons = 200

// ExtractContext holds normalized archive entries and optional subtree scoping for addon extraction.
type ExtractContext struct {
	TextFiles   map[string]string
	BinaryPaths []string
	SourcePath  string

	relOnce sync.Once
	relText map[string]string
	relBins []string
}

// SortedTextKeys returns archive text file paths sorted lexicographically.
func (c *ExtractContext) SortedTextKeys() []string {
	if c == nil {
		return nil
	}
	return SortedTextFileKeys(c.TextFiles)
}

// RelTextFiles lazily returns archive text files normalized and filtered under SourcePath.
func (c *ExtractContext) RelTextFiles() map[string]string {
	if c == nil {
		return nil
	}
	c.initRel()
	return c.relText
}

// RelBinaryPaths lazily returns archive paths normalized and filtered under SourcePath.
func (c *ExtractContext) RelBinaryPaths() []string {
	if c == nil {
		return nil
	}
	c.initRel()
	return c.relBins
}

func (c *ExtractContext) initRel() {
	c.relOnce.Do(func() {
		src := NormalizeArchiveMapPath(c.SourcePath)
		c.relText = make(map[string]string, len(c.TextFiles))
		for k, v := range c.TextFiles {
			if rel, ok := RelUnderArchiveSource(k, src); ok {
				c.relText[rel] = v
			}
		}
		c.relBins = make([]string, 0, len(c.BinaryPaths))
		for _, p := range c.BinaryPaths {
			if rel, ok := RelUnderArchiveSource(p, src); ok {
				c.relBins = append(c.relBins, rel)
			}
		}
	})
}

// AddonExtractor discovers and normalizes backup addons for a single AddonInstallMode.
type AddonExtractor interface {
	Mode() string
	Priority() int
	InferBinding(ctx *ExtractContext) *corev1.AppAddonBinding
	NormalizePackageID(pkg string) string
	Extract(ctx *ExtractContext, binding *corev1.AppAddonBinding) []corev1.InstanceBackupAddon
	MergeDuplicate(existing *corev1.InstanceBackupAddon, incoming corev1.InstanceBackupAddon)
}

// BaseExtractor provides default fallback implementations for simple AddonExtractors.
type BaseExtractor struct{}

// InferBinding returns nil by default for extractors without archive layout sniffing.
func (BaseExtractor) InferBinding(_ *ExtractContext) *corev1.AppAddonBinding { return nil }

// NormalizePackageID trims leading and trailing whitespace from pkg.
func (BaseExtractor) NormalizePackageID(pkg string) string { return strings.TrimSpace(pkg) }

// MergeDuplicate merges non-empty metadata fields onto existing without mode-specific scope rules.
func (BaseExtractor) MergeDuplicate(existing *corev1.InstanceBackupAddon, incoming corev1.InstanceBackupAddon) {
	if existing == nil {
		return
	}
	existing.Repository = FirstNonEmpty(existing.Repository, incoming.Repository)
	existing.Title = FirstNonEmpty(existing.Title, incoming.Title)
	existing.Version = FirstNonEmpty(existing.Version, incoming.Version)
	existing.TargetDir = FirstNonEmpty(existing.TargetDir, incoming.TargetDir)
	existing.InstallMode = FirstNonEmpty(existing.InstallMode, incoming.InstallMode)
	if incoming.ConfigCount > existing.ConfigCount {
		existing.ConfigCount = incoming.ConfigCount
	}
	existing.Disabled = existing.Disabled && incoming.Disabled
}

var (
	extractorMu sync.RWMutex
	extractors  = make(map[string]AddonExtractor)
)

// RegisterAddonExtractor registers ext keyed by its canonical Mode() identifier.
func RegisterAddonExtractor(ext AddonExtractor) {
	if ext == nil || strings.TrimSpace(ext.Mode()) == "" {
		return
	}
	extractorMu.Lock()
	extractors[strings.ToLower(strings.TrimSpace(ext.Mode()))] = ext
	extractorMu.Unlock()
}

// LookupAddonExtractor returns the AddonExtractor registered for mode.
func LookupAddonExtractor(mode string) (AddonExtractor, bool) {
	extractorMu.RLock()
	ext, ok := extractors[strings.ToLower(strings.TrimSpace(mode))]
	extractorMu.RUnlock()
	return ext, ok
}

// RegisteredAddonModes returns the sorted list of registered addon extractor modes.
func RegisteredAddonModes() []string {
	extractorMu.RLock()
	modes := make([]string, 0, len(extractors))
	for _, ext := range extractors {
		modes = append(modes, ext.Mode())
	}
	extractorMu.RUnlock()
	sort.Strings(modes)
	return modes
}

// InferAddonBinding probes registered extractors in priority order to detect an archive's addon layout.
func InferAddonBinding(ctx *ExtractContext) *corev1.AppAddonBinding {
	extractorMu.RLock()
	ordered := make([]AddonExtractor, 0, len(extractors))
	for _, ext := range extractors {
		ordered = append(ordered, ext)
	}
	extractorMu.RUnlock()
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Priority() != ordered[j].Priority() {
			return ordered[i].Priority() < ordered[j].Priority()
		}
		return ordered[i].Mode() < ordered[j].Mode()
	})
	for _, ext := range ordered {
		if binding := ext.InferBinding(ctx); binding != nil && binding.InstallMode != "" {
			return binding
		}
	}
	return nil
}
