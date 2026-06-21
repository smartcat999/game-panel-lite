package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestConfigPresetStripsSecretsAndListsSavedPreset(t *testing.T) {
	router, _, _ := newTestRouter(t)
	payload := `{
		"name":"Palworld Friends",
		"providerKey":"palworld",
		"version":"latest",
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
}
