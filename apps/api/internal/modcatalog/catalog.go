package modcatalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed recommended_tmodloader_mods.json
var recommendedTModLoaderModsJSON []byte

type RecommendedMod struct {
	Rank           int      `json:"rank"`
	WorkshopID     string   `json:"workshopId"`
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
	Description    string   `json:"description,omitempty"`
}

func RecommendedTModLoaderMods() ([]RecommendedMod, error) {
	var items []RecommendedMod
	if err := json.Unmarshal(recommendedTModLoaderModsJSON, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func RecommendedTModLoaderModByWorkshopID(workshopID string) (RecommendedMod, bool) {
	items, err := RecommendedTModLoaderMods()
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
