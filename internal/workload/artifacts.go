package workload

import (
	"encoding/hex"
	"fmt"
	"path"
	"strings"
)

// ValidateArtifacts checks portable paths and identities before a worker mutates
// a runtime. Size ceilings are deployment policy and are enforced by the adapter.
func ValidateArtifacts(options Options) error {
	if len(options.Artifacts) == 0 {
		return nil
	}
	paths := make(map[string]bool, len(options.Files)+len(options.Artifacts))
	for name := range options.Files {
		paths[strings.ToLower(path.Clean(name))] = true
	}
	for _, item := range options.Artifacts {
		digest, err := hex.DecodeString(item.SHA256)
		if item.ID == "" || len(item.ID) > 128 || strings.TrimSpace(item.ID) != item.ID || strings.ContainsAny(item.ID, "/\\\x00") || item.SizeBytes <= 0 || err != nil || len(digest) != 32 || strings.ToLower(item.SHA256) != item.SHA256 {
			return fmt.Errorf("invalid workload artifact identity")
		}
		for _, char := range item.ID {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
				return fmt.Errorf("invalid workload artifact ID")
			}
		}
		if item.Path == "" || item.Path == "." || path.IsAbs(item.Path) || path.Clean(item.Path) != item.Path || strings.ContainsAny(item.Path, "\\:\x00") || strings.HasPrefix(item.Path, "../") {
			return fmt.Errorf("invalid artifact path %q", item.Path)
		}
		for _, part := range strings.Split(item.Path, "/") {
			if part == ".." || strings.HasPrefix(part, ".gamepanel-prepare-") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
				return fmt.Errorf("invalid artifact path %q", item.Path)
			}
		}
		normalized := strings.ToLower(item.Path)
		if paths[normalized] {
			return fmt.Errorf("duplicate artifact destination %q", item.Path)
		}
		paths[normalized] = true
	}
	for name := range paths {
		for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if paths[parent] {
				return fmt.Errorf("artifact/configuration destination conflict %q", parent)
			}
		}
	}
	return nil
}
