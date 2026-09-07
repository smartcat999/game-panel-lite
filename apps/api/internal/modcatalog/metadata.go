package modcatalog

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Identity resolves the provider catalog name before falling back to stored metadata.
func Identity(item domain.ModFile) string {
	if item.WorkshopID != "" {
		if recommended, ok := RecommendedModByProviderAndWorkshopID(item.ProviderKey, item.WorkshopID); ok {
			for _, value := range []string{recommended.ModName, recommended.Title} {
				value = strings.TrimSpace(value)
				if value != "" {
					return value
				}
			}
		}
	}
	for _, value := range []string{item.ModName, item.Title, strings.TrimSuffix(item.FileName, filepath.Ext(item.FileName))} {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasPrefix(value, "workshop-") {
			return value
		}
	}
	return ""
}

// Dependencies prefers explicit metadata; catalog fallback stays within the provider.
// The returned slice is independent of the input.
func Dependencies(item domain.ModFile) []string {
	if len(item.Dependencies) > 0 {
		return uniqueNames(item.Dependencies)
	}
	if strings.TrimSpace(item.DependenciesJSON) != "" {
		var values []string
		if err := json.Unmarshal([]byte(item.DependenciesJSON), &values); err == nil {
			return uniqueNames(values)
		}
	}
	if item.WorkshopID != "" {
		if recommended, ok := RecommendedModByProviderAndWorkshopID(item.ProviderKey, item.WorkshopID); ok {
			return uniqueNames(recommended.Dependencies)
		}
	}
	if recommended, ok := RecommendedModByProviderAndModName(item.ProviderKey, Identity(item)); ok {
		return uniqueNames(recommended.Dependencies)
	}
	return nil
}

func uniqueNames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
