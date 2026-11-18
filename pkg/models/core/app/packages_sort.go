package app

import (
	"slices"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// isLatestAlias reports whether v is a floating alias ("latest" / "stable") rather than a pinned version.
func isLatestAlias(v string) bool {
	return corev1.IsLatestAlias(v)
}

// isIndexUpstreamApp reports whether an App resolves concrete upstream version tags rather than floating "latest".
func isIndexUpstreamApp(appName string) bool {
	switch appName {
	case corev1.AppMinecraftPaper, corev1.AppTerrariaVanilla,
		corev1.AppTerrariaTModLoader:
		return true
	default:
		return false
	}
}

// CompareVersionsDesc orders game versions semantically from newest to oldest:
//  1. Concrete version numbers are ordered from newest to oldest by numeric segments (e.g. v2026.09.3.1 > v2026.08.3.0, 1.21.11 > 1.21.4).
//  2. Floating aliases ("latest" / "stable") follow concrete versions.
func CompareVersionsDesc(a, b, _ string) int {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == b {
		return 0
	}
	aAlias, bAlias := isLatestAlias(a), isLatestAlias(b)
	if aAlias != bAlias {
		if aAlias {
			return 1
		}
		return -1
	}
	return corev1.CompareSemanticSegmentsDesc(a, b)
}

// normalizeAndSortAppVersions deduplicates versions, prunes dangling "latest" placeholders on index-upstream apps,
// sorts versions newest-first, and ensures RecommendedVersion is valid.
func normalizeAndSortAppVersions(app *corev1.App) {
	if app == nil {
		return
	}
	dedup := collectNormalizedAppVersions(app)
	slices.SortFunc(dedup, func(a, b string) int { return CompareVersionsDesc(a, b, "") })
	app.Versions = dedup
	rec := strings.TrimSpace(app.RecommendedVersion)
	if len(dedup) > 0 && (rec == "" || !slices.Contains(dedup, rec)) {
		app.RecommendedVersion = dedup[0]
	}
}

func collectNormalizedAppVersions(app *corev1.App) []string {
	seen := make(map[string]bool, len(app.Versions))
	raw := make([]string, 0, len(app.Versions))
	hasConcrete := false
	for _, v := range app.Versions {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		raw = append(raw, v)
		if !isLatestAlias(v) {
			hasConcrete = true
		}
	}
	if !hasConcrete || !isIndexUpstreamApp(app.Metadata.Name) {
		return raw
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if isLatestAlias(v) {
			continue
		}
		out = append(out, v)
	}
	return out
}
