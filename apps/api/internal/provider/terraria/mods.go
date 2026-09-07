package terraria

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
)

func (p TModLoaderProvider) RuntimeModFiles(filename string) []string {
	return RuntimeModFiles(p.Key(), filename)
}

func (p TModLoaderProvider) ModManifest(mods []domain.ModFile) (map[string]string, error) {
	enabled := []string{}
	workshopIDs := []string{}
	for _, item := range mods {
		if !item.Enabled {
			continue
		}
		if item.Source == "workshop" && item.WorkshopID != "" {
			workshopIDs = append(workshopIDs, item.WorkshopID)
			if name := runtimeModIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(item.FileName), ".tmod") {
			if name := runtimeModIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
		}
	}
	sort.Strings(enabled)
	sort.Strings(workshopIDs)
	payload, err := json.MarshalIndent(enabled, "", "  ")
	if err != nil {
		return nil, err
	}
	install := ""
	if len(workshopIDs) > 0 {
		install = strings.Join(workshopIDs, "\n") + "\n"
	}
	files := map[string]string{}
	for _, path := range p.RuntimeModFiles("enabled.json") {
		files[path] = string(payload) + "\n"
	}
	for _, path := range p.RuntimeModFiles("install.txt") {
		files[path] = install
	}
	return files, nil
}

func runtimeModIdentity(item domain.ModFile) string {
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedModByProviderAndWorkshopID(item.ProviderKey, item.WorkshopID); ok {
			for _, value := range []string{recommended.ModName, recommended.Title} {
				if value = strings.TrimSpace(value); value != "" {
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

func (TModLoaderProvider) ModSupport() domain.ModSupport {
	return domain.ModSupport{UploadExtensions: []string{".tmod"}, Workshop: true}
}
