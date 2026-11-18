package modcatalog

import (
	_ "embed"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

//go:embed recommended_tmodloader_mods.json
var recommendedTModLoaderModsJSON []byte

//go:embed recommended_dst_mods.json
var recommendedDSTModsJSON []byte

//go:embed recommended_palworld_mods.json
var recommendedPalworldModsJSON []byte

type RecommendedMod struct {
	Rank           int      `json:"rank"`
	Source         string   `json:"source,omitempty"`
	ExternalID     string   `json:"externalId,omitempty"`
	WorkshopID     string   `json:"workshopId,omitempty"`
	FileName       string   `json:"fileName,omitempty"`
	SourceURL      string   `json:"sourceUrl,omitempty"`
	ModName        string   `json:"modName,omitempty"`
	Title          string   `json:"title"`
	CreatorSteamID string   `json:"creatorSteamId,omitempty"`
	PreviewURL     string   `json:"previewUrl,omitempty"`
	FileSize       int64    `json:"fileSize"`
	Subscriptions  int      `json:"subscriptions,omitempty"`
	Favorited      int      `json:"favorited,omitempty"`
	Views          int      `json:"views,omitempty"`
	TimeCreated    int64    `json:"timeCreated,omitempty"`
	TimeUpdated    int64    `json:"timeUpdated,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	Description    string   `json:"description,omitempty"`
}

func RecommendedTModLoaderMods() ([]RecommendedMod, error) {
	return recommendedModsFromJSON(recommendedTModLoaderModsJSON)
}

func RecommendedDSTMods() ([]RecommendedMod, error) {
	return recommendedModsFromJSON(recommendedDSTModsJSON)
}

func RecommendedPalworldMods() ([]RecommendedMod, error) {
	return recommendedModsFromJSON(recommendedPalworldModsJSON)
}

func recommendedModsFromJSON(payload []byte) ([]RecommendedMod, error) {
	var items []RecommendedMod
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func RecommendedTModLoaderModByWorkshopID(workshopID string) (RecommendedMod, bool) {
	return recommendedModByWorkshopID(RecommendedTModLoaderMods, workshopID)
}

func RecommendedDSTModByWorkshopID(workshopID string) (RecommendedMod, bool) {
	return recommendedModByWorkshopID(RecommendedDSTMods, workshopID)
}

func RecommendedModByProviderAndWorkshopID(providerKey domain.ProviderKey, workshopID string) (RecommendedMod, bool) {
	switch providerKey {
	case domain.ProviderDST:
		return RecommendedDSTModByWorkshopID(workshopID)
	default:
		return RecommendedTModLoaderModByWorkshopID(workshopID)
	}
}

func RecommendedModByProviderAndExternalID(providerKey domain.ProviderKey, externalID string) (RecommendedMod, bool) {
	switch providerKey {
	case domain.ProviderPalworld:
		items, err := RecommendedPalworldMods()
		if err != nil {
			return RecommendedMod{}, false
		}
		for _, item := range items {
			if item.ExternalID == externalID {
				return item, true
			}
		}
	}
	return RecommendedMod{}, false
}

func recommendedModByWorkshopID(loader func() ([]RecommendedMod, error), workshopID string) (RecommendedMod, bool) {
	items, err := loader()
	if err != nil {
		return RecommendedMod{}, false
	}
	for _, item := range items {
		if item.WorkshopID == workshopID {
			return item, true
		}
	}
	return RecommendedMod{}, false
}

func RecommendedTModLoaderModByModName(modName string) (RecommendedMod, bool) {
	items, err := RecommendedTModLoaderMods()
	if err != nil {
		return RecommendedMod{}, false
	}
	for _, item := range items {
		if item.ModName == modName {
			return item, true
		}
	}
	return RecommendedMod{}, false
}
