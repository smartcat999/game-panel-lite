package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	workshopsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/workshop"
)

const workshopPreviewTTL = 10 * time.Minute

type cachedWorkshopPreview struct {
	ProviderKey domain.ProviderKey
	InstanceID  string
	Collection  workshopsvc.Collection
	ExpiresAt   time.Time
}

type workshopPreviewItem struct {
	workshopsvc.Item
	Status     string `json:"status"`
	Selectable bool   `json:"selectable"`
}

type workshopPreviewSummary struct {
	Total       int `json:"total"`
	New         int `json:"new"`
	InLibrary   int `json:"inLibrary"`
	InServer    int `json:"inServer"`
	Unavailable int `json:"unavailable"`
}

type workshopPreviewResponse struct {
	PreviewID    string                 `json:"previewId"`
	CollectionID string                 `json:"collectionId"`
	ProviderKey  domain.ProviderKey     `json:"providerKey"`
	ExpiresAt    time.Time              `json:"expiresAt"`
	Items        []workshopPreviewItem  `json:"items"`
	Summary      workshopPreviewSummary `json:"summary"`
}

func (h *Handler) previewWorkshopCollection(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ProviderKey domain.ProviderKey `json:"providerKey"`
		Value       string             `json:"value"`
		InstanceID  string             `json:"instanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if payload.ProviderKey == "" {
		payload.ProviderKey = domain.ProviderTerrariaTModLoader
	}
	if !providerSupportsWorkshopMods(payload.ProviderKey) {
		writeError(w, http.StatusBadRequest, "workshop mods are not supported for this provider")
		return
	}
	payload.InstanceID = strings.TrimSpace(payload.InstanceID)
	if payload.InstanceID != "" {
		server, err := h.store.GetGameServer(r.Context(), payload.InstanceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "server not found")
			return
		}
		if server.ProviderKey != payload.ProviderKey {
			writeError(w, http.StatusBadRequest, "target server does not use the selected provider")
			return
		}
	}
	collection, err := h.workshopResolver.ResolveCollection(r.Context(), payload.ProviderKey, payload.Value)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, workshopsvc.ErrInvalidCollection) ||
			errors.Is(err, workshopsvc.ErrCollectionTooLarge) ||
			errors.Is(err, workshopsvc.ErrUnsupportedProvider) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	libraryMods, err := h.store.ListMods(r.Context(), "unassigned")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	libraryIDs := workshopIDSet(libraryMods)
	serverIDs := map[string]struct{}{}
	if payload.InstanceID != "" {
		serverMods, err := h.store.ListMods(r.Context(), payload.InstanceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		serverIDs = workshopIDSet(serverMods)
	}

	response := workshopPreviewResponse{
		PreviewID:    uuid.NewString(),
		CollectionID: collection.ID,
		ProviderKey:  payload.ProviderKey,
		ExpiresAt:    time.Now().Add(workshopPreviewTTL),
		Items:        make([]workshopPreviewItem, 0, len(collection.Items)),
	}
	importableCollection := workshopsvc.Collection{ID: collection.ID, Items: make([]workshopsvc.Item, 0, len(collection.Items))}
	for _, item := range collection.Items {
		status := "new"
		selectable := true
		if payload.ProviderKey == domain.ProviderDST && !isDSTServerWorkshopTags(item.Tags) {
			status = "unavailable"
			selectable = false
			response.Summary.Unavailable++
			response.Items = append(response.Items, workshopPreviewItem{
				Item:       item,
				Status:     status,
				Selectable: selectable,
			})
			continue
		}
		switch {
		case containsWorkshopID(serverIDs, item.WorkshopID):
			status = "in_server"
			response.Summary.InServer++
		case containsWorkshopID(libraryIDs, item.WorkshopID):
			status = "in_library"
			response.Summary.InLibrary++
		default:
			response.Summary.New++
		}
		importableCollection.Items = append(importableCollection.Items, item)
		response.Items = append(response.Items, workshopPreviewItem{
			Item:       item,
			Status:     status,
			Selectable: selectable && status != "in_server",
		})
	}
	response.Summary.Total = len(response.Items)
	h.cacheWorkshopPreview(response.PreviewID, cachedWorkshopPreview{
		ProviderKey: payload.ProviderKey,
		InstanceID:  payload.InstanceID,
		Collection:  importableCollection,
		ExpiresAt:   response.ExpiresAt,
	})
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) cacheWorkshopPreview(id string, preview cachedWorkshopPreview) {
	now := time.Now()
	h.workshopPreviewsMu.Lock()
	defer h.workshopPreviewsMu.Unlock()
	for key, cached := range h.workshopPreviews {
		if now.After(cached.ExpiresAt) {
			delete(h.workshopPreviews, key)
		}
	}
	h.workshopPreviews[id] = preview
}

func (h *Handler) workshopPreviewItems(id string, providerKey domain.ProviderKey, instanceID string, workshopIDs []string) (map[string]workshopsvc.Item, error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil
	}
	h.workshopPreviewsMu.Lock()
	preview, ok := h.workshopPreviews[id]
	h.workshopPreviewsMu.Unlock()
	if !ok || time.Now().After(preview.ExpiresAt) {
		return nil, errors.New("workshop preview expired; preview the collection again")
	}
	if preview.ProviderKey != providerKey || preview.InstanceID != strings.TrimSpace(instanceID) {
		return nil, errors.New("workshop preview does not match the import target")
	}
	available := make(map[string]workshopsvc.Item, len(preview.Collection.Items))
	for _, item := range preview.Collection.Items {
		available[item.WorkshopID] = item
	}
	selected := make(map[string]workshopsvc.Item, len(workshopIDs))
	for _, id := range workshopIDs {
		item, ok := available[id]
		if !ok {
			return nil, errors.New("workshop import contains an item that was not in the preview")
		}
		selected[id] = item
	}
	return selected, nil
}

func workshopIDSet(mods []domain.ModFile) map[string]struct{} {
	result := make(map[string]struct{}, len(mods))
	for _, item := range mods {
		if item.Source == "workshop" && item.WorkshopID != "" {
			result[item.WorkshopID] = struct{}{}
		}
	}
	return result
}

func containsWorkshopID(values map[string]struct{}, id string) bool {
	_, ok := values[id]
	return ok
}

func isDSTServerWorkshopTags(tags []string) bool {
	for _, tag := range tags {
		switch strings.ToLower(strings.TrimSpace(tag)) {
		case "server_only_mod", "all_clients_require_mod":
			return true
		}
	}
	return false
}

func applyWorkshopItemMetadata(item *domain.ModFile, metadata workshopsvc.Item) {
	tags, _ := json.Marshal(metadata.Tags)
	item.Title = metadata.Title
	item.CreatorSteamID = metadata.CreatorSteamID
	item.PreviewURL = metadata.PreviewURL
	item.Description = metadata.Description
	item.SizeBytes = metadata.FileSize
	item.Subscriptions = metadata.Subscriptions
	item.Favorited = metadata.Favorited
	item.Views = metadata.Views
	item.UpdatedAtSteam = metadata.TimeUpdated
	item.TagsJSON = string(tags)
	hydrateModMetadata(item)
}
