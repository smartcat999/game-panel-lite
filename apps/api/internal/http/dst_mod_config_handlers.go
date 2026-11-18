package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
)

type dstModConfigurationResponse struct {
	WorkshopID string                   `json:"workshopId"`
	Options    []modsvc.DSTConfigOption `json:"options"`
	Values     map[string]any           `json:"values"`
}

func (h *Handler) getDSTModConfiguration(w http.ResponseWriter, r *http.Request) {
	server, item, ok := h.dstConfigurableMod(w, r)
	if !ok {
		return
	}
	path, err := dstModInfoPath(server, item)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	options, err := modsvc.ReadDSTConfigOptions(path, r.URL.Query().Get("locale"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusConflict, "mod files are not downloaded yet")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dstModConfigurationResponse{
		WorkshopID: item.WorkshopID,
		Options:    options,
		Values:     dstModConfigurationValues(server, item.WorkshopID),
	})
}

func (h *Handler) saveDSTModConfiguration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	unlock := h.lockServerMutation(id)
	defer unlock()
	server, item, ok := h.dstConfigurableMod(w, r)
	if !ok {
		return
	}
	if h.gameUpdateLocked(r.Context(), server.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return
	}
	if isGameServerBusyForModMutation(server) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return
	}
	path, err := dstModInfoPath(server, item)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	options, err := modsvc.ReadDSTConfigOptions(path, r.URL.Query().Get("locale"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusConflict, "mod files are not downloaded yet")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var payload struct {
		Values map[string]any `json:"values"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || payload.Values == nil {
		writeError(w, http.StatusBadRequest, "values are required")
		return
	}
	values, err := validateDSTModConfiguration(options, payload.Values)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	setDSTModConfigurationValues(&server, item.WorkshopID, values)
	server.Spec.Generation++
	if server.Spec.Generation <= 0 {
		server.Spec.Generation = 1
	}
	server.UpdatedAt = time.Now()
	if err := h.store.SaveGameServer(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod.configuration_updated", fmt.Sprintf("Updated mod configuration for %s", item.FileName), map[string]any{
		"modId": item.ID, "workshopId": item.WorkshopID,
	})
	writeJSON(w, http.StatusOK, dstModConfigurationResponse{WorkshopID: item.WorkshopID, Options: options, Values: values})
}

func (h *Handler) dstConfigurableMod(w http.ResponseWriter, r *http.Request) (domain.GameServer, domain.ModFile, bool) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return domain.GameServer{}, domain.ModFile{}, false
	}
	if server.ProviderKey != domain.ProviderDST {
		writeError(w, http.StatusBadRequest, "mod option configuration is only supported for Don't Starve Together servers")
		return domain.GameServer{}, domain.ModFile{}, false
	}
	item, err := h.store.GetMod(r.Context(), chi.URLParam(r, "modId"))
	if err != nil || item.InstanceID != server.ID || item.Source != "workshop" || !isDigitsOnly(item.WorkshopID) {
		writeError(w, http.StatusNotFound, "mod not found")
		return domain.GameServer{}, domain.ModFile{}, false
	}
	return server, item, true
}

func dstModInfoPath(server domain.GameServer, item domain.ModFile) (string, error) {
	dataDir, err := serverDataDir(server)
	if err != nil {
		return "", err
	}
	ugc := filepath.Join(dataDir, "ugc_mods", "content", "322330", item.WorkshopID, "modinfo.lua")
	if info, statErr := os.Lstat(ugc); statErr == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("invalid modinfo.lua")
		}
		return ugc, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	legacy := filepath.Join(dataDir, "ugc_mods", "legacy", "workshop-"+item.WorkshopID, "modinfo.lua")
	info, err := os.Lstat(legacy)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("invalid modinfo.lua")
	}
	return legacy, nil
}

func dstModConfigurationValues(server domain.GameServer, workshopID string) map[string]any {
	mods, _ := server.Spec.Config["mods"].(map[string]any)
	configurations, _ := mods["configurations"].(map[string]any)
	values, _ := configurations[workshopID].(map[string]any)
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func setDSTModConfigurationValues(server *domain.GameServer, workshopID string, values map[string]any) {
	if server.Spec.Config == nil {
		server.Spec.Config = map[string]any{}
	}
	mods, _ := server.Spec.Config["mods"].(map[string]any)
	if mods == nil {
		mods = map[string]any{}
	}
	configurations, _ := mods["configurations"].(map[string]any)
	if configurations == nil {
		configurations = map[string]any{}
	}
	configurations[workshopID] = values
	mods["configurations"] = configurations
	server.Spec.Config["mods"] = mods
}

func validateDSTModConfiguration(options []modsvc.DSTConfigOption, submitted map[string]any) (map[string]any, error) {
	declared := make(map[string]modsvc.DSTConfigOption, len(options))
	for _, option := range options {
		declared[option.Name] = option
	}
	result := make(map[string]any, len(submitted))
	for name, value := range submitted {
		option, ok := declared[name]
		if !ok {
			return nil, fmt.Errorf("unknown mod option %q", name)
		}
		matched := false
		for _, choice := range option.Choices {
			if equalDSTConfigValue(value, choice.Value) {
				result[name] = choice.Value
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("invalid value for mod option %q", name)
		}
	}
	return result, nil
}

func equalDSTConfigValue(left, right any) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	if leftIsNumber {
		parsed, err := leftNumber.Float64()
		return err == nil && reflect.DeepEqual(parsed, right)
	}
	return reflect.DeepEqual(left, right)
}
