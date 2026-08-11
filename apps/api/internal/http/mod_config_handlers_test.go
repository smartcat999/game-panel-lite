package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTModLoaderModConfigLifecycle(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod-config", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)

	upload := httptest.NewRecorder()
	router.ServeHTTP(upload, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod-config/mod-configs/upload", "file", "Example.json", []byte("{\n  \"Enabled\": true\n}")))
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected upload 201, got %d: %s", upload.Code, upload.Body.String())
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod-config/mod-configs", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", list.Code, list.Body.String())
	}
	var files []modConfigFileResponse
	if err := json.Unmarshal(list.Body.Bytes(), &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "Example.json" || files[0].Content != "" {
		t.Fatalf("unexpected list: %+v", files)
	}

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod-config/mod-configs/Example.json", nil))
	var detail modConfigFileResponse
	if err := json.Unmarshal(get.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if get.Code != stdhttp.StatusOK || !bytes.Contains([]byte(detail.Content), []byte(`"Enabled": true`)) {
		t.Fatalf("expected readable config, got %d: %s", get.Code, get.Body.String())
	}

	savePayload, err := json.Marshal(map[string]string{"content": "{\n  \"Enabled\": false\n}"})
	if err != nil {
		t.Fatal(err)
	}
	save := httptest.NewRecorder()
	router.ServeHTTP(save, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/tmod-config/mod-configs/Example.json", bytes.NewReader(savePayload)))
	if save.Code != stdhttp.StatusOK {
		t.Fatalf("expected save 200, got %d: %s", save.Code, save.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(server.DataDir, "ModConfigs", "Example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("false")) {
		t.Fatalf("expected updated content, got %s", content)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod-config/mod-configs/Example.json", nil))
	if remove.Code != stdhttp.StatusNoContent {
		t.Fatalf("expected delete 204, got %d: %s", remove.Code, remove.Body.String())
	}
}

func TestModConfigValidationAndProviderRestriction(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	tmod := testServer("tmod-config-invalid", cfg.DataDir)
	tmod.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, tmod)
	vanilla := testServer("vanilla-config", cfg.DataDir)
	vanilla.ProviderKey = domain.ProviderTerrariaVanilla
	createTestServer(t, db, vanilla)

	invalidJSON := httptest.NewRecorder()
	router.ServeHTTP(invalidJSON, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod-config-invalid/mod-configs/upload", "file", "bad.json", []byte("not-json")))
	if invalidJSON.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected invalid JSON 400, got %d", invalidJSON.Code)
	}

	invalidExtension := httptest.NewRecorder()
	router.ServeHTTP(invalidExtension, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod-config-invalid/mod-configs/upload", "file", "bad.txt", []byte("{}")))
	if invalidExtension.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected invalid extension 400, got %d", invalidExtension.Code)
	}

	unsupported := httptest.NewRecorder()
	router.ServeHTTP(unsupported, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/vanilla-config/mod-configs", nil))
	if unsupported.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected vanilla provider 400, got %d", unsupported.Code)
	}
}
