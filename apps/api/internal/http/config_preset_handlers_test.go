package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestConfigPresetStripsSecretsAndListsSavedPreset(t *testing.T) {
	router, db, _ := newTestRouter(t)
	mod := domain.ModFile{
		ID: "pal-mod-1", InstanceID: "unassigned", GameKey: domain.GamePalworld,
		ProviderKey: domain.ProviderPalworld, FileName: "quality.pak", Enabled: true,
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}
	payload := `{
		"name":"Palworld Friends",
		"providerKey":"palworld",
		"version":"v2.4.1",
		"modIds":["pal-mod-1","pal-mod-1"],
		"resources":{"cpuLimitCores":1,"memoryLimitMb":2048},
		"config":{
			"serverName":"Pal Friends",
			"saveName":"Starter Save",
			"maxPlayers":10,
			"serverPassword":"join-secret",
			"adminPassword":"admin-secret"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/config-presets", strings.NewReader(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected config preset 201, got %d: %s", create.Code, create.Body.String())
	}
	var preset domain.ConfigPreset
	if err := json.Unmarshal(create.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}
	if preset.GameKey != domain.GamePalworld || preset.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld preset identity, got %+v", preset)
	}
	if _, ok := preset.Config["serverPassword"]; ok {
		t.Fatalf("expected server password to be stripped from config, got %+v", preset.Config)
	}
	if _, ok := preset.Config["adminPassword"]; ok {
		t.Fatalf("expected admin password to be stripped from config, got %+v", preset.Config)
	}
	if _, ok := preset.ConfigPayload["serverPassword"]; ok {
		t.Fatalf("expected server password to be stripped from payload, got %+v", preset.ConfigPayload)
	}
	if _, ok := preset.ConfigPayload["adminPassword"]; ok {
		t.Fatalf("expected admin password to be stripped from payload, got %+v", preset.ConfigPayload)
	}
	if preset.ConfigPayload["saveName"] != "Starter Save" || preset.CPULimitCores != 1 || preset.MemoryLimitMB != 2048 {
		t.Fatalf("expected non-secret preset values to be saved, got %+v payload=%+v", preset, preset.ConfigPayload)
	}
	if len(preset.ModIDs) != 1 || preset.ModIDs[0] != mod.ID {
		t.Fatalf("expected preset mod snapshot to be saved, got %+v", preset.ModIDs)
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/config-presets", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected list config presets 200, got %d: %s", list.Code, list.Body.String())
	}
	var presets []domain.ConfigPreset
	if err := json.Unmarshal(list.Body.Bytes(), &presets); err != nil {
		t.Fatal(err)
	}
	if len(presets) != 1 || presets[0].ID != preset.ID {
		t.Fatalf("expected saved preset in list, got %+v", presets)
	}
	if len(presets[0].ModIDs) != 1 || presets[0].ModIDs[0] != mod.ID {
		t.Fatalf("expected preset mod snapshot to persist, got %+v", presets[0].ModIDs)
	}
}

func TestConfigPresetUpdateAndBatchDelete(t *testing.T) {
	router, _, _ := newTestRouter(t)
	createPayload := `{"name":"Friends","providerKey":"terraria-vanilla","version":"1.4.5.6","modIds":[],"config":{"serverName":"Friends","worldName":"World","worldSize":"medium","difficulty":"classic","worldEvil":"random","maxPlayers":8,"port":7777,"secure":true,"language":"en-US","autoCreateWorld":true}}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/config-presets", strings.NewReader(createPayload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected config preset 201, got %d: %s", create.Code, create.Body.String())
	}
	var preset domain.ConfigPreset
	if err := json.Unmarshal(create.Body.Bytes(), &preset); err != nil {
		t.Fatal(err)
	}

	updatePayload := strings.Replace(createPayload, `"name":"Friends"`, `"name":"Friends Updated"`, 1)
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/config-presets/"+preset.ID, strings.NewReader(updatePayload)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected config preset update 200, got %d: %s", update.Code, update.Body.String())
	}
	var updated domain.ConfigPreset
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Friends Updated" {
		t.Fatalf("expected updated preset name, got %q", updated.Name)
	}

	batch := httptest.NewRecorder()
	batchPayload := `{"ids":["` + preset.ID + `","missing","` + preset.ID + `"]}`
	router.ServeHTTP(batch, httptest.NewRequest(stdhttp.MethodPost, "/api/config-presets/batch-delete", strings.NewReader(batchPayload)))
	if batch.Code != stdhttp.StatusOK {
		t.Fatalf("expected batch delete 200, got %d: %s", batch.Code, batch.Body.String())
	}
	var result configPresetBatchDeleteResult
	if err := json.Unmarshal(batch.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 1 || len(result.Failed) != 1 {
		t.Fatalf("expected one successful and one failed delete, got %+v", result)
	}
}
