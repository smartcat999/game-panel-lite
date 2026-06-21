package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"host":                h.cfg.Host,
		"port":                h.cfg.Port,
		"dataDir":             h.cfg.DataDir,
		"dbPath":              h.cfg.DBPath,
		"dockerHost":          h.cfg.DockerHost,
		"publicHost":          h.resolvePublicHost(),
		"locale":              h.resolveLocale(r.Context()),
		"imageRegion":         h.cfg.ImageRegion,
		"imageRegistry":       h.cfg.ImageRegistry,
		"imageTag":            h.cfg.ImageTag,
		"providerCatalogPath": h.cfg.ProviderCatalogPath,
	})
}

func (h *Handler) updatePublicHost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		PublicHost string `json:"publicHost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	host := strings.TrimSpace(payload.PublicHost)
	if host != "" {
		if strings.ContainsAny(host, " \t\n\r/") || len(host) > 253 {
			writeError(w, http.StatusBadRequest, "public host must be a valid hostname or IP")
			return
		}
	}
	if host == "" {
		host = ""
	}
	if err := h.store.SetSetting(r.Context(), "publicHost", host); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), "", "settings.publicHost", fmt.Sprintf("Updated public host to %q", host), map[string]any{"publicHost": host})
	writeJSON(w, http.StatusOK, map[string]string{"publicHost": h.resolvePublicHost()})
}

func (h *Handler) updateLocale(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	locale := strings.TrimSpace(payload.Locale)
	if locale != "zh" && locale != "en" {
		writeError(w, http.StatusBadRequest, "locale must be zh or en")
		return
	}
	if err := h.store.SetSetting(r.Context(), "locale", locale); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), "", "settings.locale", fmt.Sprintf("Updated locale to %q", locale), map[string]any{"locale": locale})
	writeJSON(w, http.StatusOK, map[string]string{"locale": locale})
}

func (h *Handler) getServerJoinInfo(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	writeJSON(w, http.StatusOK, h.serverJoinInfo(server))
}

type serverShareResponse struct {
	Enabled         bool      `json:"enabled"`
	Token           string    `json:"token,omitempty"`
	SharePath       string    `json:"sharePath,omitempty"`
	IncludePassword bool      `json:"includePassword"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type publicServerShareResponse struct {
	Name        string                `json:"name"`
	GameKey     domain.GameKey        `json:"gameKey"`
	ProviderKey domain.ProviderKey    `json:"providerKey"`
	Status      domain.ServerStatus   `json:"status"`
	Players     int                   `json:"players"`
	MaxPlayers  int                   `json:"maxPlayers"`
	JoinInfo    domain.ServerJoinInfo `json:"joinInfo"`
}

func (h *Handler) getServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	share, err := h.store.GetServerShareByInstance(r.Context(), server.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusOK, serverShareResponse{Enabled: false})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, shareResponse(share))
}

func (h *Handler) enableServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	var payload struct {
		IncludePassword bool `json:"includePassword"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	now := time.Now()
	share, err := h.store.GetServerShareByInstance(r.Context(), server.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		share = domain.ServerShare{
			Token:      strings.ReplaceAll(uuid.NewString(), "-", ""),
			InstanceID: server.ID,
			CreatedAt:  now,
		}
	}
	share.IncludePassword = payload.IncludePassword
	share.UpdatedAt = now
	if err := h.store.SaveServerShare(r.Context(), &share); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.share.enabled", fmt.Sprintf("Enabled share page for %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusOK, shareResponse(share))
}

func (h *Handler) disableServerShare(w http.ResponseWriter, r *http.Request) {
	server, err := h.store.GetGameServer(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err := h.store.DeleteServerShareByInstance(r.Context(), server.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.recordActivity(r.Context(), server.ID, "server.share.disabled", fmt.Sprintf("Disabled share page for %s", server.Name), activityServerPayload(server))
	writeJSON(w, http.StatusOK, serverShareResponse{Enabled: false})
}

func (h *Handler) getPublicServerShare(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	share, err := h.store.GetServerShareByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	resource, err := h.store.GetGameServer(r.Context(), share.InstanceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "share page not found")
		return
	}
	joinInfo := h.serverJoinInfo(resource)
	if !share.IncludePassword {
		joinInfo.Password = ""
		joinInfo.InviteText = stripInvitePassword(joinInfo.InviteText)
	}
	writeJSON(w, http.StatusOK, publicServerShareResponse{
		Name:        resource.Name,
		GameKey:     resource.GameKey,
		ProviderKey: resource.ProviderKey,
		Status:      domain.ServerStatusFromRuntime(resource.Spec.DesiredState, resource.Status),
		Players:     resource.Status.PlayersOnline,
		MaxPlayers:  maxPlayersFromServer(resource),
		JoinInfo:    joinInfo,
	})
}

func shareResponse(share domain.ServerShare) serverShareResponse {
	return serverShareResponse{
		Enabled:         true,
		Token:           share.Token,
		SharePath:       "/share/" + share.Token,
		IncludePassword: share.IncludePassword,
		CreatedAt:       share.CreatedAt,
		UpdatedAt:       share.UpdatedAt,
	}
}

func stripInvitePassword(invite string) string {
	if invite == "" {
		return invite
	}
	index := strings.Index(strings.ToLower(invite), " password:")
	if index < 0 {
		return invite
	}
	return strings.TrimSpace(invite[:index])
}
