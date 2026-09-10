package workspaceapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authentication"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/authorization"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/httpfilter"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceconfiguration"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceobservability"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceprovisioning"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

type Services struct {
	Database      *sql.DB
	Sessions      *authentication.SessionService
	Authorizer    httpfilter.Authorizer
	Resolver      httpfilter.ScopeResolver
	Providers     *providercontract.Registry
	Billing       *billing.Module
	Delivery      *deliverycontrol.Postgres
	Provisioning  *instanceprovisioning.Service
	Configuration *instanceconfiguration.Service
	Actions       *instanceaction.Postgres
	Observability *instanceobservability.Postgres
}

func Routes(s Services) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /v1/workspaces", s.authenticated(http.HandlerFunc(s.listWorkspaces)))
	mux.Handle("GET /v1/regions", s.workspaceQuery(authorization.ActionWorkspaceRead, http.HandlerFunc(s.listRegions)))
	mux.Handle("GET /v1/providers/releases", s.workspaceQuery(authorization.ActionWorkspaceRead, http.HandlerFunc(s.listProviders)))
	mux.Handle("GET /v1/providers/releases/{providerReleaseId}/manifest", s.workspaceQuery(authorization.ActionWorkspaceRead, http.HandlerFunc(s.getManifest)))
	mux.Handle("GET /v1/providers/releases/{providerReleaseId}/mods", s.workspaceQuery(authorization.ActionWorkspaceRead, http.HandlerFunc(s.getMods)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/instances", s.workspacePath(authorization.ActionInstanceRead, http.HandlerFunc(s.listInstances)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/instances", s.workspacePath(authorization.ActionInstanceCreate, http.HandlerFunc(s.createInstance)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}", s.instancePath(authorization.ActionInstanceRead, http.HandlerFunc(s.getInstance)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceAction}", colonAction("logicalInstanceAction", "logicalInstanceId", map[string]http.Handler{
		"start":   s.instancePath(authorization.ActionInstanceStart, http.HandlerFunc(s.changeState("start"))),
		"stop":    s.instancePath(authorization.ActionInstanceStop, http.HandlerFunc(s.changeState("stop"))),
		"restart": s.instancePath(authorization.ActionInstanceRestart, http.HandlerFunc(s.changeState("restart"))),
	}))
	mux.Handle("POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts", s.instancePath(authorization.ActionInstanceConfigure, http.HandlerFunc(s.createDraft)))
	mux.Handle("PUT /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts/{draftId}", s.instancePath(authorization.ActionInstanceConfigure, http.HandlerFunc(s.saveDraft)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/configuration-drafts/{draftAction}", colonAction("draftAction", "draftId", map[string]http.Handler{
		"apply": s.instancePath(authorization.ActionInstanceConfigure, http.HandlerFunc(s.applyDraft)),
	}))
	mux.Handle("GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/revisions/{revisionId}", s.instancePath(authorization.ActionInstanceRead, http.HandlerFunc(s.getRevision)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/logs", s.instancePath(authorization.ActionInstanceRead, http.HandlerFunc(s.getLogs)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/metrics", s.instancePath(authorization.ActionInstanceRead, http.HandlerFunc(s.getMetrics)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/instances/{logicalInstanceId}/console-commands", s.instancePath(authorization.ActionInstanceConsole, http.HandlerFunc(s.console)))
	mux.Handle("GET /v1/workspaces/{workspaceId}/backups", s.workspacePath(authorization.ActionBackupRead, http.HandlerFunc(s.listBackups)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/backups", s.workspacePath(authorization.ActionBackupCreate, http.HandlerFunc(s.createBackup)))
	mux.Handle("POST /v1/workspaces/{workspaceId}/backups/{backupAction}", colonAction("backupAction", "backupId", map[string]http.Handler{
		"restore": s.backupPath(authorization.ActionBackupRestore, http.HandlerFunc(s.restoreBackup)),
	}))
	return mux
}

func colonAction(sourcePathValue, resourcePathValue string, actions map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		resourceID, action, found := strings.Cut(request.PathValue(sourcePathValue), ":")
		handler, supported := actions[action]
		if !found || resourceID == "" || !supported {
			http.NotFound(response, request)
			return
		}
		request.SetPathValue(resourcePathValue, resourceID)
		handler.ServeHTTP(response, request)
	})
}

func WithFallback(routes, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		if path == "/v1/workspaces" || path == "/v1/regions" || strings.HasPrefix(path, "/v1/providers/releases") || strings.HasPrefix(path, "/v1/workspaces/") && (strings.Contains(path, "/instances") || strings.Contains(path, "/backups")) {
			routes.ServeHTTP(response, request)
			return
		}
		fallback.ServeHTTP(response, request)
	})
}

func (s Services) authenticated(next http.Handler) http.Handler {
	return httpfilter.Authenticate(s.Sessions, next)
}

func (s Services) workspacePath(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceWorkspace, ResourceIDs: httpfilter.PathID("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) workspaceQuery(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceWorkspace, ResourceIDs: httpfilter.QueryIDs("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) instancePath(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceInstance, ResourceIDs: httpfilter.PathID("logicalInstanceId"), ClaimedScopeID: httpfilter.PathScopeID("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) backupPath(action authorization.Action, next http.Handler) http.Handler {
	policy := httpfilter.RoutePolicy{Action: action, ResourceType: httpfilter.ResourceBackup, ResourceIDs: httpfilter.PathID("backupId"), ClaimedScopeID: httpfilter.PathScopeID("workspaceId")}
	return s.authenticated(httpfilter.Authorize(s.Resolver, s.Authorizer, policy, next))
}

func (s Services) listWorkspaces(response http.ResponseWriter, request *http.Request) {
	principal, _ := httpfilter.PrincipalFromContext(request.Context())
	rows, err := s.Database.QueryContext(request.Context(), `SELECT workspace_id FROM memberships WHERE user_id=$1 ORDER BY workspace_id LIMIT 101`, principal.ID)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "workspaces_unavailable")
		return
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			writeError(response, http.StatusInternalServerError, "workspaces_unavailable")
			return
		}
		ids = append(ids, id)
	}
	if rows.Err() != nil || len(ids) > 100 {
		writeError(response, http.StatusInternalServerError, "workspaces_unavailable")
		return
	}
	if len(ids) == 0 {
		writeJSON(response, http.StatusOK, []any{})
		return
	}
	workspaceRows, err := s.Database.QueryContext(request.Context(), `SELECT id,slug,name FROM workspaces WHERE id=ANY($1) ORDER BY id`, ids)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "workspaces_unavailable")
		return
	}
	defer workspaceRows.Close()
	result := make([]map[string]string, 0, len(ids))
	for workspaceRows.Next() {
		var id, slug, name string
		if err := workspaceRows.Scan(&id, &slug, &name); err != nil {
			writeError(response, http.StatusInternalServerError, "workspaces_unavailable")
			return
		}
		result = append(result, map[string]string{"id": id, "slug": slug, "name": name})
	}
	writeJSON(response, http.StatusOK, result)
}

func (s Services) listRegions(response http.ResponseWriter, request *http.Request) {
	rows, err := s.Database.QueryContext(request.Context(), `SELECT id,code,name,available FROM regions WHERE available=true ORDER BY code LIMIT 100`)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "regions_unavailable")
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, code, name string
		var available bool
		if err := rows.Scan(&id, &code, &name, &available); err != nil {
			writeError(response, http.StatusInternalServerError, "regions_unavailable")
			return
		}
		result = append(result, map[string]any{"id": id, "code": code, "name": name, "available": available})
	}
	writeJSON(response, http.StatusOK, result)
}

func (s Services) listProviders(response http.ResponseWriter, request *http.Request) {
	manifests, err := s.Providers.List(request.Context(), 100)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "providers_unavailable")
		return
	}
	result := make([]map[string]any, 0, len(manifests))
	for _, manifest := range manifests {
		result = append(result, map[string]any{"id": manifest.ProviderReleaseID, "gameKey": manifest.GameKey, "displayName": manifest.DisplayName, "releaseVersion": manifest.ReleaseVersion, "gameVersions": manifest.GameVersions, "capabilities": manifest.Capabilities})
	}
	writeJSON(response, http.StatusOK, result)
}

func (s Services) getManifest(response http.ResponseWriter, request *http.Request) {
	manifest, err := s.Providers.Verified(request.Context(), request.PathValue("providerReleaseId"))
	if err != nil {
		writeError(response, http.StatusNotFound, "provider_release_not_found")
		return
	}
	writeJSON(response, http.StatusOK, manifest)
}

func (s Services) getMods(response http.ResponseWriter, request *http.Request) {
	manifest, err := s.Providers.Verified(request.Context(), request.PathValue("providerReleaseId"))
	if err != nil || manifest.ModCatalog == nil {
		writeError(response, http.StatusNotFound, "mod_catalog_not_found")
		return
	}
	writeJSON(response, http.StatusOK, manifest.ModCatalog)
}

func (s Services) listInstances(response http.ResponseWriter, request *http.Request) {
	items, err := s.Delivery.ListInstances(request.Context(), request.PathValue("workspaceId"), queryLimit(request, 100))
	respondResult(response, items, err)
}

func (s Services) getInstance(response http.ResponseWriter, request *http.Request) {
	item, err := s.Delivery.Instance(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"))
	respondResult(response, item, err)
}

func (s Services) createInstance(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Name              string                          `json:"name"`
		ProviderReleaseID string                          `json:"providerReleaseId"`
		GameVersion       string                          `json:"gameVersion"`
		Configuration     map[string]any                  `json:"configuration"`
		ModSelections     []providercontract.ModSelection `json:"modSelections"`
		QuoteID           string                          `json:"quoteId"`
	}
	if !decode(response, request, &body) {
		return
	}
	workspaceID := request.PathValue("workspaceId")
	if _, err := s.Billing.AuthorizeCreate(request.Context(), workspaceID, body.QuoteID); err != nil {
		writeError(response, http.StatusBadRequest, "funding_not_authorized")
		return
	}
	instance, operation, err := s.Provisioning.Create(request.Context(), instanceprovisioning.Command{WorkspaceID: workspaceID, Name: body.Name, ProviderReleaseID: body.ProviderReleaseID, GameVersion: body.GameVersion, Configuration: body.Configuration, ModSelections: body.ModSelections, QuoteID: body.QuoteID, IdempotencyKey: idempotencyKey(request)}, time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "instance_not_created")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"instance": instance, "operation": operation})
}

func (s Services) changeState(action string) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		operation, err := s.Delivery.ChangeState(request.Context(), deliverycontrol.ChangeStateCommand{WorkspaceID: request.PathValue("workspaceId"), LogicalInstanceID: request.PathValue("logicalInstanceId"), Action: action, IdempotencyKey: idempotencyKey(request)}, time.Now().UTC())
		if err != nil {
			writeError(response, http.StatusBadRequest, "state_change_not_accepted")
			return
		}
		writeJSON(response, http.StatusAccepted, operation)
	}
}

func (s Services) createDraft(response http.ResponseWriter, request *http.Request) {
	draft, err := s.Configuration.CreateDraft(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"), time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "draft_not_created")
		return
	}
	writeJSON(response, http.StatusCreated, draft)
}

func (s Services) saveDraft(response http.ResponseWriter, request *http.Request) {
	var body struct {
		SchemaVersion int                             `json:"schemaVersion"`
		Values        map[string]any                  `json:"values"`
		ModSelections []providercontract.ModSelection `json:"modSelections"`
	}
	if !decode(response, request, &body) {
		return
	}
	draft, err := s.Configuration.SaveDraft(request.Context(), instanceconfiguration.SaveCommand{WorkspaceID: request.PathValue("workspaceId"), LogicalInstanceID: request.PathValue("logicalInstanceId"), DraftID: request.PathValue("draftId"), SchemaVersion: body.SchemaVersion, Values: body.Values, ModSelections: body.ModSelections}, time.Now().UTC())
	respondResult(response, draft, err)
}

func (s Services) applyDraft(response http.ResponseWriter, request *http.Request) {
	revision, operation, err := s.Configuration.Apply(request.Context(), instanceconfiguration.ApplyCommand{WorkspaceID: request.PathValue("workspaceId"), LogicalInstanceID: request.PathValue("logicalInstanceId"), DraftID: request.PathValue("draftId"), IdempotencyKey: idempotencyKey(request)}, time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "draft_not_applied")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"revision": revision, "operation": operation})
}

func (s Services) getRevision(response http.ResponseWriter, request *http.Request) {
	revision, err := s.Delivery.Revision(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"), request.PathValue("revisionId"))
	respondResult(response, revision, err)
}

func (s Services) getLogs(response http.ResponseWriter, request *http.Request) {
	items, err := s.Observability.Logs(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"), queryLimit(request, 500))
	respondResult(response, items, err)
}

func (s Services) getMetrics(response http.ResponseWriter, request *http.Request) {
	items, err := s.Observability.Metrics(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"), time.Now().UTC(), queryLimit(request, 1000))
	respondResult(response, items, err)
}

func (s Services) console(response http.ResponseWriter, request *http.Request) {
	var body struct {
		Command string `json:"command"`
	}
	if !decode(response, request, &body) {
		return
	}
	operation, err := s.Actions.Console(request.Context(), request.PathValue("workspaceId"), request.PathValue("logicalInstanceId"), body.Command, idempotencyKey(request), time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "console_command_not_accepted")
		return
	}
	writeJSON(response, http.StatusAccepted, operation)
}

func (s Services) listBackups(response http.ResponseWriter, request *http.Request) {
	items, err := s.Actions.ListBackups(request.Context(), request.PathValue("workspaceId"), queryLimit(request, 100))
	respondResult(response, items, err)
}

func (s Services) createBackup(response http.ResponseWriter, request *http.Request) {
	var body struct {
		LogicalInstanceID string `json:"logicalInstanceId"`
	}
	if !decode(response, request, &body) {
		return
	}
	backup, operation, err := s.Actions.RequestBackup(request.Context(), request.PathValue("workspaceId"), body.LogicalInstanceID, idempotencyKey(request), time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "backup_not_accepted")
		return
	}
	writeJSON(response, http.StatusAccepted, map[string]any{"backup": backup, "operation": operation})
}

func (s Services) restoreBackup(response http.ResponseWriter, request *http.Request) {
	backup, err := s.Actions.Backup(request.Context(), request.PathValue("workspaceId"), request.PathValue("backupId"))
	if err != nil {
		writeError(response, http.StatusNotFound, "backup_not_found")
		return
	}
	operation, err := s.Actions.Restore(request.Context(), request.PathValue("workspaceId"), backup.LogicalInstanceID, backup.ID, idempotencyKey(request), time.Now().UTC())
	if err != nil {
		writeError(response, http.StatusBadRequest, "restore_not_accepted")
		return
	}
	writeJSON(response, http.StatusAccepted, operation)
}

func decode(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, 256<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func idempotencyKey(request *http.Request) string {
	return strings.TrimSpace(request.Header.Get("Idempotency-Key"))
}

func queryLimit(request *http.Request, fallback int) int {
	value, err := strconv.Atoi(request.URL.Query().Get("limit"))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func respondResult(response http.ResponseWriter, value any, err error) {
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, deliverycontrol.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(response, status, "resource_unavailable")
		return
	}
	writeJSON(response, http.StatusOK, value)
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, map[string]string{"error": code})
}
