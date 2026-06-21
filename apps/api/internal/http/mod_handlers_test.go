package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestTModLoaderModUploadListAndDeleteEndpoints(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "example.tmod")
	if err != nil {
		t.Fatal(err)
	}
	modBytes := tmodFixture("ExampleMod", "1.2.3", "2026.3.3.0")
	if _, err := part.Write(modBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	upload := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/tmod/mods/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(upload, request)
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var mod domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &mod); err != nil {
		t.Fatal(err)
	}
	if mod.FileName != "example.tmod" || !mod.Enabled {
		t.Fatalf("expected uploaded enabled mod, got %+v", mod)
	}
	if mod.Title != "ExampleMod" || mod.ModVersion != "1.2.3" || mod.TModVersion != "2026.3.3.0" {
		t.Fatalf("expected parsed tmod metadata, got %+v", mod)
	}
	runtimeMod, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "example.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runtimeMod, modBytes) {
		t.Fatalf("expected uploaded mod copied into runtime data dir")
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].ID != mod.ID {
		t.Fatalf("expected listed uploaded mod, got %+v", mods)
	}
	if mods[0].RuntimeEnabled == nil || !*mods[0].RuntimeEnabled {
		t.Fatalf("expected listed mod to be runtime enabled, got %+v", mods[0])
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "Mods", "enabled.json"), []byte("[]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relist := httptest.NewRecorder()
	router.ServeHTTP(relist, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if relist.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod relist 200, got %d: %s", relist.Code, relist.Body.String())
	}
	if err := json.Unmarshal(relist.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].RuntimeEnabled == nil || *mods[0].RuntimeEnabled {
		t.Fatalf("expected configured mod to be runtime disabled after enabled.json changed, got %+v", mods)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/"+mod.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "tmod", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected mod file deleted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(server.DataDir, "Mods", "example.tmod")); !os.IsNotExist(err) {
		t.Fatalf("expected runtime mod file deleted, stat err=%v", err)
	}
}

func TestModListsPruneMissingFiles(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	serverMod := domain.ModFile{
		ID:         "server-missing-mod",
		InstanceID: server.ID,
		FileName:   "missing.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	globalMod := domain.ModFile{
		ID:         "global-missing-mod",
		InstanceID: "unassigned",
		FileName:   "library.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &serverMod); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	serverList := httptest.NewRecorder()
	router.ServeHTTP(serverList, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if serverList.Code != stdhttp.StatusOK {
		t.Fatalf("expected server mod list 200, got %d: %s", serverList.Code, serverList.Body.String())
	}
	var serverMods []domain.ModFile
	if err := json.Unmarshal(serverList.Body.Bytes(), &serverMods); err != nil {
		t.Fatal(err)
	}
	if len(serverMods) != 0 {
		t.Fatalf("expected missing server mod pruned from response, got %+v", serverMods)
	}
	if _, err := db.GetMod(context.Background(), serverMod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing server mod record pruned, got err=%v", err)
	}

	globalList := httptest.NewRecorder()
	router.ServeHTTP(globalList, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if globalList.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", globalList.Code, globalList.Body.String())
	}
	var globalMods []domain.ModFile
	if err := json.Unmarshal(globalList.Body.Bytes(), &globalMods); err != nil {
		t.Fatal(err)
	}
	if len(globalMods) != 0 {
		t.Fatalf("expected missing global mod pruned from response, got %+v", globalMods)
	}
	if _, err := db.GetMod(context.Background(), globalMod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected missing global mod record pruned, got err=%v", err)
	}
}

func TestTModLoaderModUploadIsIdempotentForSameFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "example.tmod", []byte("mod-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first upload 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstMod domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstMod); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "example.tmod", []byte("mod-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated upload 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondMod domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondMod); err != nil {
		t.Fatal(err)
	}
	if secondMod.ID != firstMod.ID {
		t.Fatalf("expected repeated upload to update existing mod, got first=%+v second=%+v", firstMod, secondMod)
	}
	if secondMod.SizeBytes != int64(len("mod-v2")) {
		t.Fatalf("expected updated size, got %+v", secondMod)
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one server mod record after repeated upload, got %+v", mods)
	}
}

func TestTModLoaderModEnabledEndpoint(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	mod := domain.ModFile{
		ID:         "mod-1",
		InstanceID: server.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}
	other := domain.ModFile{
		ID:         "mod-2",
		InstanceID: server.ID,
		FileName:   "other.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &other); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"enabled":false}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/api/servers/tmod/mods/mod-1", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod enabled update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var got domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatalf("expected disabled mod, got %+v", got)
	}
	persisted, err := db.GetMod(context.Background(), mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled {
		t.Fatalf("expected persisted disabled mod, got %+v", persisted)
	}
	runtimeEnabled, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "enabled.json"))
	if err != nil {
		t.Fatal(err)
	}
	var enabledMods []string
	if err := json.Unmarshal(runtimeEnabled, &enabledMods); err != nil {
		t.Fatalf("expected runtime enabled.json to be JSON list, got %q: %v", string(runtimeEnabled), err)
	}
	if !reflect.DeepEqual(enabledMods, []string{"other"}) {
		t.Fatalf("expected runtime enabled.json to contain only the enabled tmod package names, got %v", enabledMods)
	}
}

func TestRunningTModLoaderServerAllowsModMutation(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	server.Status = domain.StatusRunning
	createTestServer(t, db, server)
	mod := domain.ModFile{
		ID:         "mod-1",
		InstanceID: server.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader(`{"enabled":false}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPatch, "/api/servers/tmod/mods/mod-1", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected running mod update success, got %d: %s", recorder.Code, recorder.Body.String())
	}
	persisted, err := db.GetMod(context.Background(), mod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled {
		t.Fatalf("expected running mod update to change enabled state, got %+v", persisted)
	}

	uploadRecorder := httptest.NewRecorder()
	router.ServeHTTP(uploadRecorder, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/servers/tmod/mods/upload", "file", "new.tmod", []byte("new")))
	if uploadRecorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected running mod upload success, got %d: %s", uploadRecorder.Code, uploadRecorder.Body.String())
	}
	if _, err := db.GetModByInstanceAndFile(context.Background(), server.ID, "new.tmod"); err != nil {
		t.Fatalf("expected running upload to create mod record: %v", err)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/mod-1", nil)
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected running mod delete success, got %d: %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected running delete to remove mod record, got err=%v", err)
	}

	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "library.tmod", bytes.NewBufferString("library")); err != nil {
		t.Fatal(err)
	}
	libraryMod := domain.ModFile{
		ID:         "library-mod",
		InstanceID: "unassigned",
		FileName:   "library.tmod",
		SizeBytes:  7,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &libraryMod); err != nil {
		t.Fatal(err)
	}
	assignRecorder := httptest.NewRecorder()
	assignRequest := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/library-mod/assign", strings.NewReader(`{"instanceId":"tmod"}`))
	assignRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(assignRecorder, assignRequest)
	if assignRecorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected running mod assign success, got %d: %s", assignRecorder.Code, assignRecorder.Body.String())
	}
	if _, err := db.GetModByInstanceAndFile(context.Background(), server.ID, "library.tmod"); err != nil {
		t.Fatalf("expected running assign to create server mod record: %v", err)
	}
}

func TestTModLoaderWorkshopImportWritesInstallFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/servers/tmod/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347","2824688072","2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected workshop import 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected requested workshop records plus dependency, got %+v", items)
	}
	if items[0].Source != "workshop" || items[0].WorkshopID == "" || items[0].FileName == "install.txt" {
		t.Fatalf("expected workshop mod record, got %+v", items[0])
	}
	expected := "2563309347\n2824688072\n2908170107\n"
	runtimeInstall, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "install.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeInstall) != expected {
		t.Fatalf("expected runtime install.txt %q, got %q", expected, string(runtimeInstall))
	}
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/tmod/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	for _, mod := range mods {
		if mod.Source == "workshop" && (mod.RuntimePresent == nil || *mod.RuntimePresent) {
			t.Fatalf("expected workshop mod to be marked unsynced until runtime file exists, got %+v", mod)
		}
	}
}

func TestGlobalWorkshopImportCreatesLibraryWorkshopRecords(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347","2824688072","2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected global workshop import 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two global workshop records, got %+v", items)
	}
	for _, item := range items {
		if item.InstanceID != "unassigned" || item.Source != "workshop" || item.WorkshopID == "" || item.FileName == "install.txt" {
			t.Fatalf("expected unassigned workshop record, got %+v", item)
		}
	}
	mods, err := db.ListMods(context.Background(), "unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("expected two global workshop mods, got %+v", mods)
	}
}

func TestGlobalWorkshopImportRejectsArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected global workshop import conflict on arm runtime, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGlobalWorkshopImportRejectsDuplicateWorkshopID(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	existing := domain.ModFile{
		ID:             "existing-workshop",
		InstanceID:     "unassigned",
		FileName:       "workshop-2563309347",
		Source:         "workshop",
		WorkshopID:     "2563309347",
		Title:          "Magic Storage",
		CreatorSteamID: "76561198122163241",
		SizeBytes:      5643106,
		Enabled:        true,
		CreatedAt:      time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/workshop", bytes.NewBufferString(`{"workshopIds":["2563309347"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected duplicate workshop import 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRecommendedModsMarksExistingLibraryItems(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_ = cfg
	existing := domain.ModFile{
		ID:             "existing-workshop",
		InstanceID:     "unassigned",
		FileName:       "workshop-2563309347",
		Source:         "workshop",
		WorkshopID:     "2563309347",
		Title:          "Magic Storage",
		CreatorSteamID: "76561198122163241",
		SizeBytes:      5643106,
		Enabled:        true,
		CreatedAt:      time.Now(),
	}
	if err := db.CreateMod(context.Background(), &existing); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/mods/recommended", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected recommended mods 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []struct {
		WorkshopID   string             `json:"workshopId"`
		GameKey      domain.GameKey     `json:"gameKey"`
		ProviderKey  domain.ProviderKey `json:"providerKey"`
		ModName      string             `json:"modName"`
		Dependencies []string           `json:"dependencies"`
		InLibrary    bool               `json:"inLibrary"`
		ModID        string             `json:"modId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.WorkshopID == "2563309347" {
			found = true
			if item.ModName != "MagicStorage" || !reflect.DeepEqual(item.Dependencies, []string{"SerousCommonLib"}) {
				t.Fatalf("expected Magic Storage dependency metadata, got %+v", item)
			}
			if item.GameKey != domain.GameTerraria || item.ProviderKey != domain.ProviderTerrariaTModLoader {
				t.Fatalf("expected recommended mod game metadata, got %+v", item)
			}
			if !item.InLibrary || item.ModID != "existing-workshop" {
				t.Fatalf("expected recommended workshop mod marked as in library, got %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("expected Magic Storage to appear in recommended mods")
	}
}

func TestLegacyWorkshopInstallRecordMigratesToWorkshopMods(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	_, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "install.txt", bytes.NewBufferString("2563309347\n2824688072\n2563309347\n"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := domain.ModFile{
		ID:         "legacy-install",
		InstanceID: "unassigned",
		FileName:   "install.txt",
		SizeBytes:  int64(len("2563309347\n2824688072\n2563309347\n")),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &legacy); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var items []domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two migrated workshop mods, got %+v", items)
	}
	for _, item := range items {
		if item.Source != "workshop" || item.WorkshopID == "" || item.FileName == "install.txt" {
			t.Fatalf("expected migrated workshop mod, got %+v", item)
		}
	}
	if _, err := db.GetMod(context.Background(), legacy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected legacy install record deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", "unassigned", "install.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy install file removed, got err=%v", err)
	}
}

func TestModPackCreateListAndDelete(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	for _, name := range []string{"boss.tmod", "quality.tmod"} {
		if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", name, bytes.NewBufferString(name)); err != nil {
			t.Fatal(err)
		}
		item := domain.ModFile{
			ID:         strings.TrimSuffix(name, ".tmod"),
			InstanceID: "unassigned",
			FileName:   name,
			SizeBytes:  int64(len(name)),
			Enabled:    true,
			CreatedAt:  time.Now(),
		}
		if err := db.CreateMod(context.Background(), &item); err != nil {
			t.Fatal(err)
		}
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Boss Night","description":"Boss run mods","modIds":["boss","quality","boss"]}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod pack create 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		ID          string             `json:"id"`
		Name        string             `json:"name"`
		Description string             `json:"description"`
		GameKey     domain.GameKey     `json:"gameKey"`
		ProviderKey domain.ProviderKey `json:"providerKey"`
		ModIDs      []string           `json:"modIds"`
		Mods        []domain.ModFile   `json:"mods"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Name != "Boss Night" || created.Description != "Boss run mods" {
		t.Fatalf("unexpected created mod pack: %+v", created)
	}
	if !reflect.DeepEqual(created.ModIDs, []string{"boss", "quality"}) {
		t.Fatalf("expected unique ordered mod IDs, got %+v", created.ModIDs)
	}
	if len(created.Mods) != 2 {
		t.Fatalf("expected resolved mods in response, got %+v", created.Mods)
	}
	if created.GameKey != domain.GameTerraria || created.ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected created mod pack game metadata, got %+v", created)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/mod-packs", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod pack list 200, got %d: %s", list.Code, list.Body.String())
	}
	var packs []struct {
		ID          string             `json:"id"`
		GameKey     domain.GameKey     `json:"gameKey"`
		ProviderKey domain.ProviderKey `json:"providerKey"`
		ModIDs      []string           `json:"modIds"`
		Mods        []domain.ModFile   `json:"mods"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &packs); err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].ID != created.ID || len(packs[0].Mods) != 2 {
		t.Fatalf("expected listed mod pack with resolved mods, got %+v", packs)
	}
	if packs[0].GameKey != domain.GameTerraria || packs[0].ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected listed mod pack game metadata, got %+v", packs[0])
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/mod-packs/"+created.ID, nil))
	if remove.Code != stdhttp.StatusOK {
		t.Fatalf("expected mod pack delete 200, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetModPack(context.Background(), created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected mod pack deleted, got err=%v", err)
	}
}

func TestModPackCreateDoesNotAutoIncludeKnownDependencies(t *testing.T) {
	router, db, _ := newTestRouter(t)
	magic := domain.ModFile{
		ID:         "magic",
		InstanceID: "unassigned",
		FileName:   "workshop-2563309347",
		Source:     "workshop",
		WorkshopID: "2563309347",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &magic); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Storage","modIds":["magic"]}`)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod pack create 201, got %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		ModIDs []string         `json:"modIds"`
		Mods   []domain.ModFile `json:"mods"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.ModIDs) != 1 || len(created.Mods) != 1 {
		t.Fatalf("expected mod pack to include only selected mods, got %+v", created)
	}
	if !reflect.DeepEqual(created.Mods[0].Dependencies, []string{"SerousCommonLib"}) {
		t.Fatalf("expected selected mod to expose dependency metadata, got %+v", created.Mods)
	}
}

func TestModPackCreateRejectsServerScopedMods(t *testing.T) {
	router, db, _ := newTestRouter(t)
	serverMod := domain.ModFile{
		ID:         "server-mod",
		InstanceID: "server-1",
		FileName:   "server.tmod",
		SizeBytes:  10,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &serverMod); err != nil {
		t.Fatal(err)
	}

	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/mod-packs", bytes.NewBufferString(`{"name":"Bad Pack","modIds":["server-mod"]}`)))
	if create.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected server-scoped mod pack create to fail, got %d: %s", create.Code, create.Body.String())
	}
}

func TestAssignModRefreshesExistingServerModCache(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "large.tmod", bytes.NewBufferString("library-version")); err != nil {
		t.Fatal(err)
	}
	libraryMod := domain.ModFile{
		ID:         "library-large-mod",
		InstanceID: "unassigned",
		FileName:   "large.tmod",
		SizeBytes:  int64(len("library-version")),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &libraryMod); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(server.ID, "large.tmod", bytes.NewBufferString("cached-version")); err != nil {
		t.Fatal(err)
	}
	runtimeModPath := filepath.Join(server.DataDir, "Mods", "large.tmod")
	if err := os.MkdirAll(filepath.Dir(runtimeModPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtimeModPath, []byte("cached-runtime-version"), 0o600); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/library-large-mod/assign", strings.NewReader(`{"instanceId":"tmod"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected mod assign success, got %d: %s", recorder.Code, recorder.Body.String())
	}
	cached, err := os.ReadFile(filepath.Join(cfg.DataDir, "mods", server.ID, "large.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cached) != "library-version" {
		t.Fatalf("expected server mod cache to be refreshed from library, got %q", string(cached))
	}
	runtimeCached, err := os.ReadFile(runtimeModPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeCached) != "library-version" {
		t.Fatalf("expected runtime mod mount cache to be refreshed from library, got %q", string(runtimeCached))
	}
}

func TestTModLoaderModDeleteRejectsDifferentServerMod(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	source := testServer("source-tmod", cfg.DataDir)
	source.ProviderKey = domain.ProviderTerrariaTModLoader
	target := testServer("target-tmod", cfg.DataDir)
	target.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, source)
	createTestServer(t, db, target)
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(target.ID, "example.tmod", bytes.NewBufferString("mod")); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "target-mod",
		InstanceID: target.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/source-tmod/mods/target-mod", nil))
	if remove.Code != stdhttp.StatusNotFound {
		t.Fatalf("expected cross-server mod delete 404, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected target mod record to remain, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "mods", target.ID, "example.tmod")); err != nil {
		t.Fatalf("expected target mod file to remain, stat err=%v", err)
	}
}

func TestTModLoaderModDeleteKeepsRecordWhenFileRemovalFails(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	blockedPath := filepath.Join(cfg.DataDir, "mods", server.ID, "blocked.tmod")
	if err := os.MkdirAll(filepath.Join(blockedPath, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "blocked-mod",
		InstanceID: server.ID,
		FileName:   "blocked.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	remove := httptest.NewRecorder()
	router.ServeHTTP(remove, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/tmod/mods/blocked-mod", nil))
	if remove.Code != stdhttp.StatusInternalServerError {
		t.Fatalf("expected mod delete to fail when file removal fails, got %d: %s", remove.Code, remove.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected mod record to remain after failed file removal, got %v", err)
	}
}

func TestGlobalModUploadIsIdempotentForSameFile(t *testing.T) {
	router, db, _ := newTestRouter(t)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "example.tmod", []byte("mod-v1")))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first global upload 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstMod domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstMod); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "example.tmod", []byte("mod-v2")))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated global upload 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondMod domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondMod); err != nil {
		t.Fatal(err)
	}
	if secondMod.ID != firstMod.ID {
		t.Fatalf("expected repeated global upload to update existing mod, got first=%+v second=%+v", firstMod, secondMod)
	}
	mods, err := db.ListMods(context.Background(), "unassigned")
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one global mod record after repeated upload, got %+v", mods)
	}
}

func TestGlobalModUploadHydratesKnownDependencies(t *testing.T) {
	router, _, _ := newTestRouter(t)

	upload := httptest.NewRecorder()
	router.ServeHTTP(upload, newMultipartFileRequest(t, stdhttp.MethodPost, "/api/mods/upload", "file", "ImproveGame.tmod", tmodFixture("ImproveGame", "1.8.2", "2026.3.3.0")))
	if upload.Code != stdhttp.StatusCreated {
		t.Fatalf("expected global upload 201, got %d: %s", upload.Code, upload.Body.String())
	}
	var uploaded domain.ModFile
	if err := json.Unmarshal(upload.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(uploaded.Dependencies, []string{"SilkyUIFramework"}) {
		t.Fatalf("expected uploaded ImproveGame dependency metadata, got %+v", uploaded)
	}

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(stdhttp.MethodGet, "/api/mods", nil))
	if list.Code != stdhttp.StatusOK {
		t.Fatalf("expected global mod list 200, got %d: %s", list.Code, list.Body.String())
	}
	var mods []domain.ModFile
	if err := json.Unmarshal(list.Body.Bytes(), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || !reflect.DeepEqual(mods[0].Dependencies, []string{"SilkyUIFramework"}) {
		t.Fatalf("expected listed ImproveGame dependency metadata, got %+v", mods)
	}
	if mods[0].GameKey != domain.GameTerraria || mods[0].ProviderKey != domain.ProviderTerrariaTModLoader {
		t.Fatalf("expected listed mod game metadata, got %+v", mods[0])
	}
}

func TestAssignModIsIdempotentForSameServerFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", "example.tmod", bytes.NewBufferString("mod-v1")); err != nil {
		t.Fatal(err)
	}
	globalMod := domain.ModFile{
		ID:         "global-mod",
		InstanceID: "unassigned",
		FileName:   "example.tmod",
		SizeBytes:  6,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if first.Code != stdhttp.StatusCreated {
		t.Fatalf("expected first assign 201, got %d: %s", first.Code, first.Body.String())
	}
	var firstAssigned domain.ModFile
	if err := json.Unmarshal(first.Body.Bytes(), &firstAssigned); err != nil {
		t.Fatal(err)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if second.Code != stdhttp.StatusOK {
		t.Fatalf("expected repeated assign 200, got %d: %s", second.Code, second.Body.String())
	}
	var secondAssigned domain.ModFile
	if err := json.Unmarshal(second.Body.Bytes(), &secondAssigned); err != nil {
		t.Fatal(err)
	}
	if secondAssigned.ID != firstAssigned.ID {
		t.Fatalf("expected repeated assign to update existing mod, got first=%+v second=%+v", firstAssigned, secondAssigned)
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected one server mod record after repeated assign, got %+v", mods)
	}
	runtimeMod, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "example.tmod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeMod) != "mod-v1" {
		t.Fatalf("expected assigned mod copied into runtime data dir, got %q", string(runtimeMod))
	}
}

func TestAssignModCopiesKnownDependencies(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	for _, item := range []struct {
		id       string
		fileName string
		modName  string
	}{
		{id: "magic", fileName: "MagicStorage.tmod", modName: "MagicStorage"},
		{id: "serous", fileName: "SerousCommonLib.tmod", modName: "SerousCommonLib"},
	} {
		if _, _, err := modsvc.NewService(cfg.DataDir).Upload("unassigned", item.fileName, bytes.NewReader(tmodFixture(item.modName, "1.0.0", "2026.04.3.0"))); err != nil {
			t.Fatal(err)
		}
		globalMod := domain.ModFile{
			ID:         item.id,
			InstanceID: "unassigned",
			FileName:   item.fileName,
			ModName:    item.modName,
			Title:      item.modName,
			SizeBytes:  64,
			Enabled:    true,
			CreatedAt:  time.Now(),
		}
		if err := db.CreateMod(context.Background(), &globalMod); err != nil {
			t.Fatal(err)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/magic/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected assign 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	mods, err := db.ListMods(context.Background(), server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("expected assigned mod and dependency, got %+v", mods)
	}
	for _, fileName := range []string{"MagicStorage.tmod", "SerousCommonLib.tmod"} {
		if _, err := os.Stat(filepath.Join(server.DataDir, "Mods", fileName)); err != nil {
			t.Fatalf("expected runtime mod %s to be copied: %v", fileName, err)
		}
	}
	enabled, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "enabled.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MagicStorage", "SerousCommonLib"} {
		if !strings.Contains(string(enabled), name) {
			t.Fatalf("expected enabled.json to contain %s, got %s", name, enabled)
		}
	}
}

func TestAssignWorkshopModRejectsArmRuntime(t *testing.T) {
	router, db, cfg := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	item := domain.ModFile{
		ID:         "magic",
		InstanceID: "unassigned",
		FileName:   "workshop-2563309347",
		Source:     "workshop",
		WorkshopID: "2563309347",
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &item); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/mods/magic/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`)))
	if recorder.Code != stdhttp.StatusConflict {
		t.Fatalf("expected workshop assign conflict on arm runtime, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestAssignGlobalWorkshopModWritesServerInstallFile(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	globalMod := domain.ModFile{
		ID:         "global-workshop-mod",
		InstanceID: "unassigned",
		FileName:   "workshop-2619954303",
		Source:     "workshop",
		WorkshopID: "2619954303",
		SizeBytes:  11,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &globalMod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/api/mods/global-workshop-mod/assign", bytes.NewBufferString(`{"instanceId":"tmod"}`))
	router.ServeHTTP(recorder, request)
	if recorder.Code != stdhttp.StatusCreated {
		t.Fatalf("expected assign 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var assigned domain.ModFile
	if err := json.Unmarshal(recorder.Body.Bytes(), &assigned); err != nil {
		t.Fatal(err)
	}
	if assigned.Source != "workshop" || assigned.WorkshopID != "2619954303" || assigned.FileName == "install.txt" {
		t.Fatalf("expected assigned workshop record, got %+v", assigned)
	}
	runtimeInstall, err := os.ReadFile(filepath.Join(server.DataDir, "Mods", "install.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(runtimeInstall) != "2619954303\n" {
		t.Fatalf("expected runtime install.txt to contain workshop id, got %q", string(runtimeInstall))
	}
}

func TestGlobalModDeleteRejectsServerMod(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("tmod", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	mod := domain.ModFile{
		ID:         "server-mod",
		InstanceID: server.ID,
		FileName:   "example.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/mods/server-mod", nil))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected global delete to reject server mod, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := db.GetMod(context.Background(), mod.ID); err != nil {
		t.Fatalf("expected server mod record to remain, got %v", err)
	}
}
