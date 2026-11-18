package driver

import (
	"path"
	"sort"
	"strings"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// SliceBalancedDelimBody slices a balanced openCh/closeCh body while respecting quoted strings.
func SliceBalancedDelimBody(s string, openCh, closeCh byte) (string, bool) {
	depth, inQuote, escaped := 1, false, false
	for i := range len(s) {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inQuote {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			switch ch {
			case openCh:
				depth++
			case closeCh:
				depth--
				if depth == 0 {
					return s[:i], true
				}
			}
		}
	}
	return "", false
}

// SortedTextFileKeys returns the keys of m sorted lexicographically.
func SortedTextFileKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SortedBoolKeys returns the true keys of m sorted lexicographically.
func SortedBoolKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, ok := range m {
		if ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// NormalizeRelConfigPath normalizes a non-root relative config file path.
func NormalizeRelConfigPath(raw string) string {
	norm, err := metav1.NormalizeArchiveEntryPath(raw, false)
	if err != nil {
		return ""
	}
	return norm
}

// NormalizeArchiveMapPath normalizes an archive directory/root path, defaulting to ".".
func NormalizeArchiveMapPath(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "."
	}
	norm, err := metav1.NormalizeArchiveEntryPath(raw, true)
	if err != nil {
		return "."
	}
	return norm
}

// RelUnderArchiveSource maps entryPath relative to sourcePath when sourcePath is not ".".
func RelUnderArchiveSource(entryPath, sourcePath string) (string, bool) {
	normEntry, err := metav1.NormalizeArchiveEntryPath(entryPath, false)
	if err != nil {
		return "", false
	}
	normSrc := NormalizeArchiveMapPath(sourcePath)
	if normSrc == "." {
		return normEntry, true
	}
	if normEntry == normSrc {
		return path.Base(normEntry), true
	}
	if strings.HasPrefix(normEntry, normSrc+"/") {
		return strings.TrimPrefix(normEntry, normSrc+"/"), true
	}
	return "", false
}

// JoinArchiveSubpath joins a normalized archive root and child segment cleanly.
func JoinArchiveSubpath(root, rel string) string {
	normRoot := NormalizeArchiveMapPath(root)
	cleanRel := strings.Trim(path.Clean(strings.ReplaceAll(rel, "\\", "/")), "/")
	if cleanRel == "" || cleanRel == "." {
		return normRoot
	}
	if normRoot == "." {
		return cleanRel
	}
	return path.Join(normRoot, cleanRel)
}

// HasPathPrefix reports whether target equals prefix or resides underneath prefix/.
func HasPathPrefix(target, prefix string) bool {
	if prefix == "" || prefix == "." {
		return true
	}
	return target == prefix || strings.HasPrefix(target, prefix+"/")
}

// FirstNonEmpty returns the first value with non-whitespace characters.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ContainsStringFold reports whether items contains target case-insensitively.
func ContainsStringFold(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(item, target) {
			return true
		}
	}
	return false
}

// StripWorkshopPrefix removes a leading case-insensitive "workshop-" prefix from a package ID.
func StripWorkshopPrefix(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(trimmed), "workshop-") {
		return trimmed[len("workshop-"):]
	}
	return trimmed
}

// ParseSimpleKVLines parses simple "key = value" lines (trimmed) from text content.
func ParseSimpleKVLines(content string) map[string]string {
	out := make(map[string]string)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "--") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		if key != "" {
			out[key] = strings.TrimSpace(v)
		}
	}
	return out
}
