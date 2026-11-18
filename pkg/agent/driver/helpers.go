package driver

import (
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

// CleanWorkspaceRelPath normalizes a workspace-relative path and rejects traversal outside the root.
func CleanWorkspaceRelPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "." {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimLeft(trimmed, "/\\"))))
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return cleaned
}

// ResolveTargetDir normalizes raw relative path or falls back to defaultDir when empty or invalid.
func ResolveTargetDir(raw, defaultDir string) string {
	targetDir := CleanWorkspaceRelPath(FirstNonEmpty(raw, defaultDir))
	if targetDir == "" {
		return defaultDir
	}
	return targetDir
}

// FirstNonEmpty returns the first trimmed non-empty string in vals.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// AppendUniqueStr appends trimmed val to list if non-empty and not already present.
func AppendUniqueStr(list []string, val string) []string {
	val = strings.TrimSpace(val)
	if val == "" || slices.Contains(list, val) {
		return list
	}
	return append(list, val)
}

// ForceReplaceAddon reports whether an addon artifact coordinate needs file replacement based on previously materialized digests.
func (w *WorkspaceContext) ForceReplaceAddon(coord, digest string) bool {
	if w == nil || len(w.Prev.Digests) == 0 || coord == "" || digest == "" {
		return true
	}
	return w.Prev.Digests[coord] != digest
}

// GetOrInitState retrieves typed per-mode state from ws.State, initializing and storing it on first access.
func GetOrInitState[T any](ws *WorkspaceContext, key string, initFn func() T) T {
	if ws.State == nil {
		ws.State = make(map[string]any)
	}
	if existing, ok := ws.State[key]; ok {
		if typed, ok := existing.(T); ok {
			return typed
		}
	}
	val := initFn()
	ws.State[key] = val
	return val
}

// FormatLuaScalar formats a string-encoded addon config value as a typed Lua literal using schema hints.
func FormatLuaScalar(key, rawVal string, schema []registryv1.AddonConfigField) string {
	trimmed := strings.TrimSpace(rawVal)
	vtype := resolveSchemaValueType(key, rawVal, schema)
	switch vtype {
	case registryv1.AddonConfigValueBoolean:
		if strings.EqualFold(trimmed, "true") {
			return "true"
		}
		return "false"
	case registryv1.AddonConfigValueNumber:
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return trimmed
		}
		return strconv.Quote(rawVal)
	case registryv1.AddonConfigValueString:
		return strconv.Quote(rawVal)
	default:
		if trimmed == "true" || trimmed == "false" {
			return trimmed
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return trimmed
		}
		return strconv.Quote(rawVal)
	}
}

func resolveSchemaValueType(key, rawVal string, schema []registryv1.AddonConfigField) registryv1.AddonConfigValueType {
	for _, field := range schema {
		if field.Name != key {
			continue
		}
		for _, opt := range field.Options {
			if opt.Value == rawVal && opt.ValueType != "" {
				return opt.ValueType
			}
		}
		if field.DefaultType != "" {
			return field.DefaultType
		}
		switch field.Type {
		case registryv1.AddonConfigTypeBoolean:
			return registryv1.AddonConfigValueBoolean
		case registryv1.AddonConfigTypeNumber:
			return registryv1.AddonConfigValueNumber
		case registryv1.AddonConfigTypeText:
			return registryv1.AddonConfigValueString
		}
	}
	return ""
}
