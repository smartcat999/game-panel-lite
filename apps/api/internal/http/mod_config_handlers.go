package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

const maxModConfigBytes = 1024 * 1024

type modConfigFileResponse struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	UpdatedAt time.Time `json:"updatedAt"`
	Content   string    `json:"content,omitempty"`
}

func (h *Handler) listModConfigs(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.modConfigServer(w, r)
	if !ok {
		return
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, []modConfigFileResponse{})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]modConfigFileResponse, 0, len(entries))
	for _, entry := range entries {
		name, err := safety.SafeFileName(entry.Name(), ".json")
		if err != nil || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxModConfigBytes {
			continue
		}
		items = append(items, modConfigFileResponse{Name: name, SizeBytes: info.Size(), UpdatedAt: info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) getModConfig(w http.ResponseWriter, r *http.Request) {
	_, dir, ok := h.modConfigServer(w, r)
	if !ok {
		return
	}
	item, err := readModConfig(dir, chi.URLParam(r, "name"))
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "mod config not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) saveModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, dir, ok := h.mutableModConfigServer(w, r)
	if !ok {
		return
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxModConfigBytes+1024)).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	item, err := writeModConfig(dir, chi.URLParam(r, "name"), []byte(payload.Content))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.saved", "Saved mod config "+item.Name, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) uploadModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, dir, ok := h.mutableModConfigServer(w, r)
	if !ok {
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "mod config file is required")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxModConfigBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "unable to read mod config")
		return
	}
	item, err := writeModConfig(dir, header.Filename, content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.uploaded", "Uploaded mod config "+item.Name, map[string]any{"name": item.Name})
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) deleteModConfig(w http.ResponseWriter, r *http.Request) {
	unlock := h.lockServerMutation(chi.URLParam(r, "id"))
	defer unlock()
	server, dir, ok := h.mutableModConfigServer(w, r)
	if !ok {
		return
	}
	name, path, err := modConfigPath(dir, chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "mod config not found")
		return
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		writeError(w, http.StatusBadRequest, "invalid mod config file")
		return
	}
	if err := os.Remove(path); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "mod_config.deleted", "Deleted mod config "+name, map[string]any{"name": name})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) modConfigServer(w http.ResponseWriter, r *http.Request) (domain.GameServer, string, bool) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return domain.GameServer{}, "", false
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		writeError(w, http.StatusBadRequest, "mod configs are only supported for tModLoader servers")
		return domain.GameServer{}, "", false
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return domain.GameServer{}, "", false
	}
	dir, err := safety.SafeJoin(dataDir, "ModConfigs")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameServer{}, "", false
	}
	if info, err := os.Lstat(dir); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		writeError(w, http.StatusConflict, "invalid mod config directory")
		return domain.GameServer{}, "", false
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameServer{}, "", false
	}
	return server, dir, true
}

func (h *Handler) mutableModConfigServer(w http.ResponseWriter, r *http.Request) (domain.GameServer, string, bool) {
	server, dir, ok := h.modConfigServer(w, r)
	if !ok {
		return domain.GameServer{}, "", false
	}
	if h.gameUpdateLocked(r.Context(), server.ID) {
		writeError(w, http.StatusConflict, "server maintenance is in progress")
		return domain.GameServer{}, "", false
	}
	if isGameServerBusyForModMutation(server) {
		writeError(w, http.StatusConflict, "server lifecycle action already in progress")
		return domain.GameServer{}, "", false
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameServer{}, "", false
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return domain.GameServer{}, "", false
	}
	return server, dir, true
}

func modConfigPath(dir, rawName string) (string, string, error) {
	name, err := safety.SafeFileName(strings.TrimSpace(rawName), ".json")
	if err != nil {
		return "", "", fmt.Errorf("invalid mod config name: %w", err)
	}
	path, err := safety.SafeJoin(dir, name)
	return name, path, err
}

func readModConfig(dir, rawName string) (modConfigFileResponse, error) {
	name, path, err := modConfigPath(dir, rawName)
	if err != nil {
		return modConfigFileResponse{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return modConfigFileResponse{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxModConfigBytes {
		return modConfigFileResponse{}, fmt.Errorf("invalid mod config file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return modConfigFileResponse{}, err
	}
	if !json.Valid(content) {
		return modConfigFileResponse{}, fmt.Errorf("mod config is not valid JSON")
	}
	return modConfigFileResponse{Name: name, SizeBytes: info.Size(), UpdatedAt: info.ModTime(), Content: string(content)}, nil
}

func writeModConfig(dir, rawName string, content []byte) (modConfigFileResponse, error) {
	name, path, err := modConfigPath(dir, rawName)
	if err != nil {
		return modConfigFileResponse{}, err
	}
	if len(content) == 0 || len(content) > maxModConfigBytes {
		return modConfigFileResponse{}, fmt.Errorf("mod config must be between 1 byte and 1 MiB")
	}
	var object map[string]any
	if err := json.Unmarshal(content, &object); err != nil || object == nil {
		return modConfigFileResponse{}, fmt.Errorf("mod config must contain a valid JSON object")
	}
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return modConfigFileResponse{}, fmt.Errorf("invalid mod config file")
	}
	temp, err := os.CreateTemp(dir, ".mod-config-*.tmp")
	if err != nil {
		return modConfigFileResponse{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(content); err == nil {
		err = temp.Chmod(0o666)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tempName, path)
	}
	if err != nil {
		return modConfigFileResponse{}, err
	}
	return readModConfig(dir, name)
}
