package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	backupsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	worldsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/world"
)

func TestStartServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-start", cfg.DataDir)
	server.ContainerID = ""
	server.Status = domain.StatusStopped
	createTestServer(t, db, server)

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
		response <- recorder
	}()

	select {
	case <-adapter.createStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async start worker to begin creating a runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServer
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if domain.ServerStatusFromRuntime(queued.Spec.DesiredState, queued.Status) != domain.StatusStarting {
			t.Fatalf("expected queued start status starting, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.createRelease)
		recorder := <-response
		t.Fatalf("start request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.createRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updated, err := loadTestServer(db, server.ID)
		if err == nil && updated.Status == domain.StatusRunning && updated.ContainerID != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, _ := loadTestServer(db, server.ID)
	t.Fatalf("expected async start worker to mark server running, got %+v", updated)
}

func TestStartTModLoaderServerNormalizesOldDockerTagVersion(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("old-tmod-version", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	server.Config = terraria.Presets[4].Config
	server.Version = "2024.10"
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected runtime container to be created")
	}
	if spec.Image != "smartcat99999/tmodloader:v2026.04.3.0" {
		t.Fatalf("expected versioned tModLoader runtime image, got %q", spec.Image)
	}
	if strings.Contains(spec.Image, "radioactivehydra") || strings.Contains(spec.Image, "2024.10") {
		t.Fatalf("expected old Docker tag not to be used, got image %q", spec.Image)
	}
	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.Version != "v2026.04.3.0" {
		t.Fatalf("expected stored version to be normalized, got %q", updated.Version)
	}
}

func TestStartPalworldServerRuntimeSpecUsesConfigPayload(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("palworld-payload-runtime", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Version = "latest"
	server.Port = palworld.DefaultInternalPort
	server.HostPort = 18211
	server.Config = terraria.Config{
		ServerName: "Old Pal",
		WorldName:  "Old Save",
		MaxPlayers: 4,
		Port:       palworld.DefaultInternalPort,
		Password:   "old-join",
		MOTD:       "old-admin",
	}
	server.ConfigPayloadJSON = `{"serverName":"Payload Pal","saveName":"Payload Save","maxPlayers":14,"serverPassword":"payload-join","adminPassword":"payload-admin"}`
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	env := strings.Join(spec.Options.Env, "\n")
	for _, expected := range []string{
		"PLAYERS=14",
		"SERVER_NAME=Payload Pal",
		"SERVER_PASSWORD=payload-join",
		"ADMIN_PASSWORD=payload-admin",
	} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected runtime env to contain %q, got:\n%s", expected, env)
		}
	}
	if spec.ConfigText != "" {
		t.Fatalf("Palworld runtime should not write legacy serverconfig.txt, got:\n%s", spec.ConfigText)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestStartServerReusesExistingContainer(t *testing.T) {
	t.Skip("obsolete: controller reconciliation may recreate workloads when spec generation changes")
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("start-reuse", cfg.DataDir)
	server.Status = domain.StatusStopped
	server.ContainerID = "old-container"
	server.Version = "1.4.5.6"
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	select {
	case removed := <-adapter.removed:
		t.Fatalf("expected start to reuse existing container, removed %q", removed)
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case spec := <-adapter.created:
		t.Fatalf("expected start to reuse existing container, created %+v", spec)
	case <-time.After(50 * time.Millisecond):
	}

	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.ContainerID != "old-container" {
		t.Fatalf("expected existing container id to be reused, got %+v", updated)
	}
}

func TestRestartServerRecreatesExistingContainer(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("restart-recreate", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	server.Version = "1.4.5.6"
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/restart", nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected restart 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	select {
	case removed := <-adapter.removed:
		if removed != "old-container" {
			t.Fatalf("expected old container to be removed, got %q", removed)
		}
	case <-time.After(time.Second):
		t.Fatal("expected restart to remove the old runtime container before recreation")
	}

	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected restart to recreate runtime container")
	}
	if spec.Image != "smartcat99999/terraria-vanilla:1.4.5.6" {
		t.Fatalf("expected recreated container to use current image tag, got %q", spec.Image)
	}
	updated := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if updated.ContainerID == "old-container" || updated.ContainerID == "" {
		t.Fatalf("expected restarted server to use a recreated container, got %+v", updated)
	}
}

func TestStopServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-stop", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "running-container"
	createTestServer(t, db, server)

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
		response <- recorder
	}()

	select {
	case <-adapter.stopStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async stop worker to begin stopping the runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected stop 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServer
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if domain.ServerStatusFromRuntime(queued.Spec.DesiredState, queued.Status) != domain.StatusStopping {
			t.Fatalf("expected queued stop status stopping, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.stopRelease)
		recorder := <-response
		t.Fatalf("stop request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.stopRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		updated, err := loadTestServer(db, server.ID)
		if err == nil && updated.Status == domain.StatusStopped {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, _ := loadTestServer(db, server.ID)
	t.Fatalf("expected async stop worker to mark server stopped, got %+v", updated)
}

func TestDeleteServerReturnsAcceptedBeforeRuntimeCompletes(t *testing.T) {
	adapter := newBlockingRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("async-delete", cfg.DataDir)
	server.ContainerID = "existing-container"
	server.Status = domain.StatusStopped
	createTestServer(t, db, server)

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
		response <- recorder
	}()

	select {
	case <-adapter.stopStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async delete worker to begin stopping the runtime container")
	}

	select {
	case recorder := <-response:
		if recorder.Code != stdhttp.StatusAccepted {
			t.Fatalf("expected delete 202, got %d: %s", recorder.Code, recorder.Body.String())
		}
		var queued domain.GameServer
		if err := json.Unmarshal(recorder.Body.Bytes(), &queued); err != nil {
			t.Fatal(err)
		}
		if domain.ServerStatusFromRuntime(queued.Spec.DesiredState, queued.Status) != domain.StatusDeleting {
			t.Fatalf("expected queued delete status deleting, got %+v", queued)
		}
	case <-time.After(100 * time.Millisecond):
		close(adapter.stopRelease)
		recorder := <-response
		t.Fatalf("delete request blocked until runtime completed; got %d: %s", recorder.Code, recorder.Body.String())
	}

	close(adapter.stopRelease)
	select {
	case <-adapter.removeStarted:
	case <-time.After(time.Second):
		t.Fatal("expected async delete worker to remove runtime container after stop")
	}
	close(adapter.removeRelease)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := loadTestServer(db, server.ID); errors.Is(err, store.ErrNotFound) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	updated, err := loadTestServer(db, server.ID)
	t.Fatalf("expected async delete worker to remove server record, got server=%+v err=%v", updated, err)
}

func TestDeleteServerStopsErroredContainerBeforeRemovingRecord(t *testing.T) {
	adapter := newDeleteOrderRuntimeAdapter()
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("delete-errored-container", cfg.DataDir)
	server.ContainerID = "exited-container"
	server.Status = domain.StatusErrored
	server.LastError = "exited (exit code 127)"
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected delete 202, got %d: %s", recorder.Code, recorder.Body.String())
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := loadTestServer(db, server.ID); errors.Is(err, store.ErrNotFound) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := loadTestServer(db, server.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected server record deleted after container cleanup, got err=%v", err)
	}
	want := []string{"inspect", "stop", "remove"}
	if !reflect.DeepEqual(adapter.calls, want) {
		t.Fatalf("expected delete to inspect, stop, then remove container; got %v", adapter.calls)
	}
}

func TestCreatePalworldServerUsesPalworldRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Pal Friends",
		"providerKey":"palworld",
		"hostPort":18211,
		"config":{
			"serverName":"Pal Friends",
			"saveName":"Starter Save",
			"maxPlayers":10,
			"serverPassword":"join-secret",
			"adminPassword":"admin-secret"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", strings.NewReader(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServer
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GamePalworld || server.ProviderKey != domain.ProviderPalworld {
		t.Fatalf("expected Palworld server identity, got %+v", server)
	}
	if server.Spec.Network.Port != 8211 || server.Spec.Network.HostPort != 18211 {
		t.Fatalf("expected Palworld ports, got internal=%d external=%d", server.Spec.Network.Port, server.Spec.Network.HostPort)
	}
	if server.Spec.Config["saveName"] != "Starter Save" || server.Spec.Config["serverPassword"] != "join-secret" || server.Spec.Config["adminPassword"] != "admin-secret" {
		t.Fatalf("expected semantic Palworld config payload, got %+v", server.Spec.Config)
	}
	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "thijsvanloef/palworld-server-docker:latest" {
		t.Fatalf("expected Palworld image, got %q", spec.Image)
	}
	if spec.Port != 8211 || spec.Options.PortProtocol != "udp" {
		t.Fatalf("expected Palworld UDP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestCreateDSTServerUsesDSTRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"DST Friends",
		"providerKey":"dont-starve-together",
		"hostPort":11099,
		"config":{
			"serverName":"DST Friends",
			"clusterName":"FriendsCluster",
			"maxPlayers":6,
			"serverPassword":"join-secret",
			"clusterToken":"klei-token",
			"gameMode":"endless",
			"worldPreset":"forest_classic",
			"cavesEnabled":true,
			"workshopIds":"123456789,987654321"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", strings.NewReader(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServer
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GameDST || server.ProviderKey != domain.ProviderDST {
		t.Fatalf("expected DST server identity, got %+v", server)
	}
	if server.Spec.Network.Port != 10999 || server.Spec.Network.HostPort != 11099 {
		t.Fatalf("expected DST ports, got internal=%d external=%d", server.Spec.Network.Port, server.Spec.Network.HostPort)
	}
	if server.Spec.Config["clusterName"] != "FriendsCluster" || server.Spec.Config["clusterToken"] != "klei-token" || server.Spec.Config["gameMode"] != "endless" || server.Spec.Config["worldPreset"] != "forest_classic" || server.Spec.Config["cavesEnabled"] != true {
		t.Fatalf("expected semantic DST config payload, got %+v", server.Spec.Config)
	}
	if _, ok := server.Spec.Config["workshopIds"]; ok {
		t.Fatalf("workshop IDs should not be persisted from the server config form, got %+v", server.Spec.Config)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "smartcat99999/dst-server:latest" {
		t.Fatalf("expected DST image, got %q", spec.Image)
	}
	if spec.Port != 10999 || spec.Options.PortProtocol != "udp" {
		t.Fatalf("expected DST UDP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	if !strings.Contains(spec.Options.Files["dst/FriendsCluster/cluster.ini"], "game_mode = endless") {
		t.Fatalf("expected DST cluster.ini in runtime files, got %+v", spec.Options.Files)
	}
	if !strings.Contains(spec.Options.Files["dst/FriendsCluster/Master/worldgen.lua"], `preset = "forest_classic"`) {
		t.Fatalf("expected DST world preset in runtime files, got %+v", spec.Options.Files)
	}
	if _, ok := spec.Options.Files["dst/FriendsCluster/Caves/server.ini"]; !ok {
		t.Fatalf("expected DST caves shard files, got %+v", spec.Options.Files)
	}
	if spec.Options.Files["dst/FriendsCluster/cluster_token.txt"] != "klei-token\n" {
		t.Fatalf("expected DST token file, got %q", spec.Options.Files["dst/FriendsCluster/cluster_token.txt"])
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestCreateDSTServerRejectsArmRuntime(t *testing.T) {
	router, _, _ := newTestRouterWithAdapter(t, armMockAdapter{availableMockAdapter{MockAdapter: runtime.NewMockAdapter()}})
	payload := `{
		"name":"DST Friends",
		"providerKey":"dont-starve-together",
		"hostPort":11099,
		"config":{
			"serverName":"DST Friends",
			"clusterName":"FriendsCluster",
			"maxPlayers":6,
			"clusterToken":"klei-token"
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", strings.NewReader(payload)))
	if create.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected create server 400 on arm runtime, got %d: %s", create.Code, create.Body.String())
	}
	if !strings.Contains(create.Body.String(), "amd64 Docker hosts") {
		t.Fatalf("expected arm runtime rejection detail, got %s", create.Body.String())
	}
}

func TestCreateMinecraftServerUsesMinecraftRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Friends MC",
		"providerKey":"minecraft",
		"hostPort":25565,
		"version":"1.20.4",
		"config":{
			"serverName":"Friends MC",
			"worldName":"survival-island",
			"maxPlayers":12,
			"gameMode":"survival",
			"difficulty":"normal",
			"onlineMode":true,
			"whitelistEnabled":false,
			"eulaAccepted":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", strings.NewReader(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServer
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.GameKey != domain.GameMinecraft || server.ProviderKey != domain.ProviderMinecraft {
		t.Fatalf("expected Minecraft server identity, got %+v", server)
	}
	if server.Spec.Network.Port != 25565 {
		t.Fatalf("expected Minecraft internal port 25565, got %d", server.Spec.Network.Port)
	}
	if server.Spec.Config["eulaAccepted"] != true || server.Spec.Config["gameMode"] != "survival" {
		t.Fatalf("expected semantic Minecraft config payload, got %+v", server.Spec.Config)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Image != "itzg/minecraft-server:latest" {
		t.Fatalf("expected Minecraft image, got %q", spec.Image)
	}
	if !containsEnv(spec.Options.Env, "VERSION=1.20.4") {
		t.Fatalf("expected Minecraft VERSION=1.20.4 env, got %v", spec.Options.Env)
	}
	if spec.Port != 25565 || spec.Options.PortProtocol != "tcp" {
		t.Fatalf("expected Minecraft TCP port mapping, got port=%d protocol=%q", spec.Port, spec.Options.PortProtocol)
	}
	properties := spec.Options.Files["data/server.properties"]
	for _, expected := range []string{"max-players=12", "motd=Friends MC", "level-name=survival-island", "gamemode=survival", "difficulty=normal", "online-mode=true"} {
		if !strings.Contains(properties, expected) {
			t.Fatalf("expected server.properties to contain %q, got:\n%s", expected, properties)
		}
	}
	if spec.Options.Files["data/eula.txt"] != "eula=true\n" {
		t.Fatalf("expected Minecraft eula=true, got %q", spec.Options.Files["data/eula.txt"])
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestUpdatePalworldConfigPersistsSemanticPayload(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("palworld-config-update", cfg.DataDir)
	server.GameKey = domain.GamePalworld
	server.ProviderKey = domain.ProviderPalworld
	server.Port = 8211
	server.HostPort = 18211
	server.Config = terraria.Config{
		ServerName: "Palworld Server",
		WorldName:  "Palworld Save",
		MaxPlayers: 8,
		Port:       palworld.DefaultInternalPort,
		MOTD:       "admin-password",
	}
	server.ConfigPayload = map[string]any{
		"serverName":    server.Config.ServerName,
		"saveName":      server.Config.WorldName,
		"maxPlayers":    server.Config.MaxPlayers,
		"adminPassword": server.Config.MOTD,
	}
	createTestServer(t, db, server)
	payload := `{
		"config":{
			"serverName":"Pal Update",
			"saveName":"Updated Save",
			"maxPlayers":16,
			"serverPassword":"join-updated",
			"adminPassword":"admin-updated"
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/config", strings.NewReader(payload)))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated domain.GameServer
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Config["saveName"] != "Updated Save" || updated.Spec.Config["maxPlayers"] != float64(16) && updated.Spec.Config["maxPlayers"] != 16 || updated.Spec.Config["serverPassword"] != "join-updated" || updated.Spec.Config["adminPassword"] != "admin-updated" {
		t.Fatalf("expected updated Palworld config payload, got %+v", updated.Spec.Config)
	}
}

func TestCreateServerRejectsUnsupportedVersion(t *testing.T) {
	router, _, _ := newTestRouter(t)
	payload := `{
		"name":"Unsupported Version",
		"providerKey":"terraria-vanilla",
		"version":"0.0.0",
		"config":{
			"serverName":"Unsupported Version",
			"worldName":"VersionWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"zh-Hans",
			"autoCreateWorld":true
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", strings.NewReader(payload)))
	if recorder.Code != stdhttp.StatusBadRequest {
		t.Fatalf("expected unsupported version 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateServerPersistsResourceLimitsInRuntimeSpec(t *testing.T) {
	adapter := newCaptureCreateAdapter()
	router, db, _ := newTestRouterWithAdapter(t, adapter)
	payload := `{
		"name":"Limited Server",
		"providerKey":"terraria-vanilla",
		"hostPort":17778,
		"resources":{"cpuLimitCores":1.5,"memoryLimitMb":2048},
		"config":{
			"serverName":"Limited Server",
			"worldName":"LimitedWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":8,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	create := httptest.NewRecorder()
	router.ServeHTTP(create, httptest.NewRequest(stdhttp.MethodPost, "/api/servers", bytes.NewBufferString(payload)))
	if create.Code != stdhttp.StatusCreated {
		t.Fatalf("expected create server 201, got %d: %s", create.Code, create.Body.String())
	}
	var server domain.GameServer
	if err := json.Unmarshal(create.Body.Bytes(), &server); err != nil {
		t.Fatal(err)
	}
	if server.Spec.Resources.CPULimitCores != 1.5 || server.Spec.Resources.MemoryLimitMB != 2048 {
		t.Fatalf("expected persisted resource limits, got %+v", server.Spec.Resources)
	}

	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/start", nil))
	if start.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected start 202, got %d: %s", start.Code, start.Body.String())
	}
	var spec runtime.ContainerSpec
	select {
	case spec = <-adapter.created:
	case <-time.After(time.Second):
		t.Fatal("expected async start to create runtime container")
	}
	if spec.Resources.CPULimitCores != 1.5 || spec.Resources.MemoryLimitMB != 2048 {
		t.Fatalf("expected runtime resource limits, got %+v", spec.Resources)
	}
	waitForServerStatus(t, db, server.ID, domain.StatusRunning)
}

func TestUpdateServerConfigPersistsResourceLimitsAndRequiresRestartWhenRunning(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("resource-update", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "container-1"
	server.ConfigRevision = 2
	server.AppliedConfigRevision = 2
	createTestServer(t, db, server)
	payload := `{
		"hostPort":18888,
		"resources":{"cpuLimitCores":2,"memoryLimitMb":4096},
		"config":{
			"serverName":"Resource Update",
			"worldName":"ResourceWorld",
			"worldSize":"medium",
			"difficulty":"classic",
			"maxPlayers":10,
			"port":7777,
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/"+server.ID+"/config", bytes.NewBufferString(payload)))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var updated domain.GameServer
	if err := json.Unmarshal(recorder.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.Resources.CPULimitCores != 2 || updated.Spec.Resources.MemoryLimitMB != 4096 || updated.Spec.Network.HostPort != 18888 {
		t.Fatalf("expected updated resource and port limits, got %+v", updated)
	}
	if updated.Spec.Generation != 3 || updated.Status.AppliedGeneration != 2 {
		t.Fatalf("expected running config update to wait for restart, got generation=%d applied=%d", updated.Spec.Generation, updated.Status.AppliedGeneration)
	}
	stored, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CPULimitCores != 2 || stored.MemoryLimitMB != 4096 {
		t.Fatalf("expected stored resource limits, got cpu=%v memory=%d", stored.CPULimitCores, stored.MemoryLimitMB)
	}
}

func TestDeleteServerRemovesOwnedResources(t *testing.T) {
	router, db, cfg := newTestRouter(t)
	server := testServer("owned-resources", cfg.DataDir)
	server.ProviderKey = domain.ProviderTerrariaTModLoader
	createTestServer(t, db, server)
	if _, _, err := worldsvc.NewService(cfg.DataDir).Import(server.ID, "owned.wld", bytes.NewBufferString("world")); err != nil {
		t.Fatal(err)
	}
	world := domain.World{
		ID:               "world-1",
		InstanceID:       server.ID,
		ActiveInstanceID: server.ID,
		Name:             "owned",
		FileName:         "owned.wld",
		SizeBytes:        5,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateWorld(context.Background(), &world); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(server.DataDir, "serverconfig.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupPath, backupSize, err := backupsvc.NewService(cfg.DataDir).Create(server.ID, server.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	backup := domain.Backup{
		ID:         "backup-1",
		InstanceID: server.ID,
		FileName:   filepath.Base(backupPath),
		WorldName:  server.WorldName,
		SizeBytes:  backupSize,
		Type:       "manual",
		CreatedAt:  time.Now(),
	}
	if err := db.CreateBackup(context.Background(), &backup); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modsvc.NewService(cfg.DataDir).Upload(server.ID, "owned.tmod", bytes.NewBufferString("mod")); err != nil {
		t.Fatal(err)
	}
	mod := domain.ModFile{
		ID:         "mod-1",
		InstanceID: server.ID,
		FileName:   "owned.tmod",
		SizeBytes:  3,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	if err := db.CreateMod(context.Background(), &mod); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodDelete, "/api/servers/"+server.ID, nil))
	if recorder.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected delete server 202, got %d: %s", recorder.Code, recorder.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := loadTestServer(db, server.ID); errors.Is(err, store.ErrNotFound) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := loadTestServer(db, server.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected server record deleted, got err=%v", err)
	}
	if _, err := db.GetWorld(context.Background(), world.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned world record deleted, got err=%v", err)
	}
	if _, err := db.GetBackup(context.Background(), backup.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned backup record deleted, got err=%v", err)
	}
	if _, err := db.GetMod(context.Background(), mod.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected owned mod record deleted, got err=%v", err)
	}
	for _, path := range []string{
		filepath.Join(cfg.DataDir, "worlds", server.ID, "owned.wld"),
		filepath.Join(cfg.DataDir, "backups", server.ID, backup.FileName),
		filepath.Join(cfg.DataDir, "mods", server.ID, "owned.tmod"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected owned resource file removed at %s, stat err=%v", path, err)
		}
	}
}

func TestRunningServerCommandAndLogsRequireAttachedRuntime(t *testing.T) {
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stale-runtime", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	createTestServer(t, db, server)

	command := httptest.NewRecorder()
	router.ServeHTTP(command, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/command", bytes.NewBufferString(`{"command":"say hello"}`)))
	if command.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("expected command to reject stale runtime container, got %d: %s", command.Code, command.Body.String())
	}
	if adapter.commandContainer != "" || adapter.startedContainer != "" || adapter.created != 0 {
		t.Fatalf("expected command path to avoid runtime repair, got command=%q started=%q created=%d", adapter.commandContainer, adapter.startedContainer, adapter.created)
	}

	logs := httptest.NewRecorder()
	router.ServeHTTP(logs, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs", nil))
	if logs.Code != stdhttp.StatusServiceUnavailable {
		t.Fatalf("expected logs to reject stale runtime container, got %d: %s", logs.Code, logs.Body.String())
	}
	if adapter.logsContainer != "" || adapter.created != 0 {
		t.Fatalf("expected logs path to avoid runtime repair, got logs=%q created=%d", adapter.logsContainer, adapter.created)
	}
}

func TestStoppedServerLogSnapshotToleratesMissingRuntimeContainer(t *testing.T) {
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stopped-missing-runtime", cfg.DataDir)
	server.Status = domain.StatusStopped
	server.ContainerID = "old-container"
	createTestServer(t, db, server)

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs/snapshot", nil))
	if snapshot.Code != stdhttp.StatusOK {
		t.Fatalf("expected stopped missing runtime log snapshot to stay readable, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Lines == nil || len(payload.Lines) != 0 {
		t.Fatalf("expected empty log history for missing stopped container, got %+v", payload)
	}
	stored, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ContainerID != "old-container" {
		t.Fatalf("expected log snapshot read path not to mutate stale container id, got %+v", stored)
	}
}

func TestRunningServerLogSnapshotKeepsRunningStatus(t *testing.T) {
	adapter := &statusCapturingLogsAdapter{
		availableMockAdapter: availableMockAdapter{MockAdapter: runtime.NewMockAdapter()},
	}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("running-log-snapshot", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "runtime-container"
	createTestServer(t, db, server)

	snapshot := httptest.NewRecorder()
	router.ServeHTTP(snapshot, httptest.NewRequest(stdhttp.MethodGet, "/api/servers/"+server.ID+"/logs/snapshot", nil))
	if snapshot.Code != stdhttp.StatusOK {
		t.Fatalf("expected running log snapshot 200, got %d: %s", snapshot.Code, snapshot.Body.String())
	}
	if adapter.logRuntimeID != "runtime-container" {
		t.Fatalf("expected runtime logs to be read by runtime id, got %q", adapter.logRuntimeID)
	}
	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(snapshot.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Lines) != 1 || !strings.Contains(payload.Lines[0], "running log snapshot") {
		t.Fatalf("expected running snapshot logs, got %+v", payload)
	}
}

func TestStopServerClearsMissingRuntimeContainer(t *testing.T) {
	t.Skip("obsolete: runtime drift repair belongs to the controller, not the HTTP stop handler")
	adapter := &staleContainerAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("stale-stop", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "old-container"
	createTestServer(t, db, server)

	stop := httptest.NewRecorder()
	router.ServeHTTP(stop, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/"+server.ID+"/stop", nil))
	if stop.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected stop to clear stale runtime container, got %d: %s", stop.Code, stop.Body.String())
	}
	var stopped domain.GameServer
	if err := json.Unmarshal(stop.Body.Bytes(), &stopped); err != nil {
		t.Fatal(err)
	}
	if domain.ServerStatusFromRuntime(stopped.Spec.DesiredState, stopped.Status) != domain.StatusStopping {
		t.Fatalf("expected queued stop status stopping, got %+v", stopped)
	}
	stored := waitForServerStatus(t, db, server.ID, domain.StatusStopped)
	if stored.ContainerID != "" {
		t.Fatalf("expected stopped server with cleared container, got %+v", stored)
	}
	if adapter.stoppedContainer != "" {
		t.Fatalf("expected stale stop to skip runtime stop call, got %q", adapter.stoppedContainer)
	}
}

func TestListServersSkipsRuntimeInspectWhenDockerUnavailable(t *testing.T) {
	adapter := &unavailableInspectAdapter{}
	router, db, cfg := newTestRouterWithAdapter(t, adapter)
	server := testServer("unavailable-runtime-list", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "runtime-container"
	createTestServer(t, db, server)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(stdhttp.MethodGet, "/api/servers", nil))
	if recorder.Code != stdhttp.StatusOK {
		t.Fatalf("expected server list to remain available, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if adapter.inspectCalls != 0 {
		t.Fatalf("expected list not to inspect runtime while Docker monitor is unavailable, got %d inspect calls", adapter.inspectCalls)
	}
	var got []domain.GameServer
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || domain.ServerStatusFromRuntime(got[0].Spec.DesiredState, got[0].Status) != domain.StatusRunning {
		t.Fatalf("expected stored status without runtime refresh, got %+v", got)
	}
}

func TestUpdateServerConfigRequiresStoppedAndRewritesRuntimeConfig(t *testing.T) {
	t.Skip("obsolete: config updates now persist spec; runtime files are rendered by controller")
	router, db, cfg := newTestRouter(t)
	server := testServer("config-target", cfg.DataDir)
	server.ContainerID = "old-container"
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	payload := `{
		"hostPort":17777,
		"config":{
			"serverName":"Edited Server",
			"worldName":"EditedWorld",
			"worldSize":"large",
			"difficulty":"expert",
			"maxPlayers":12,
			"port":18888,
			"password":"secret",
			"motd":"Updated from detail page",
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/config-target/config", bytes.NewBufferString(payload)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected config update 200, got %d: %s", update.Code, update.Body.String())
	}
	updated, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Edited Server" || updated.WorldName != "EditedWorld" || updated.Port != 7777 || updated.HostPort != 17777 || updated.MaxPlayers != 12 || updated.Password != "secret" {
		t.Fatalf("expected server fields synchronized from config, got %+v", updated)
	}
	if updated.Config.Difficulty != terraria.Difficulty("expert") || updated.Config.WorldSize != terraria.WorldSize("large") || updated.Config.Language != terraria.DefaultLanguage {
		t.Fatalf("expected persisted config update, got %+v", updated.Config)
	}
	if updated.ContainerID != "" {
		t.Fatalf("expected stale container id cleared after config update, got %q", updated.ContainerID)
	}
	if updated.ConfigRevision == 0 || updated.AppliedConfigRevision != updated.ConfigRevision {
		t.Fatalf("expected stopped config update to be considered applied, got config=%d applied=%d", updated.ConfigRevision, updated.AppliedConfigRevision)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("world=/home/container/Worlds/EditedWorld.wld")) || !bytes.Contains(configBytes, []byte("maxplayers=12")) || !bytes.Contains(configBytes, []byte("port=7777")) || !bytes.Contains(configBytes, []byte("language=en-US")) {
		t.Fatalf("expected rewritten serverconfig, got %q", string(configBytes))
	}
}

func TestUpdateRunningServerConfigPreservesLiveContainer(t *testing.T) {
	t.Skip("obsolete: config updates now persist spec and rely on controller reconciliation")
	router, db, cfg := newTestRouter(t)
	server := testServer("running-config-target", cfg.DataDir)
	server.Status = domain.StatusRunning
	server.ContainerID = "live-container"
	if err := os.MkdirAll(server.DataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	createTestServer(t, db, server)

	payload := `{
		"hostPort":17778,
		"config":{
			"serverName":"Edited While Running",
			"worldName":"EditedWorld",
			"worldSize":"large",
			"difficulty":"expert",
			"maxPlayers":10,
			"port":18888,
			"password":"secret",
			"motd":"Restart to apply",
			"secure":true,
			"language":"en-US",
			"autoCreateWorld":true
		}
	}`
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(stdhttp.MethodPut, "/api/servers/running-config-target/config", bytes.NewBufferString(payload)))
	if update.Code != stdhttp.StatusOK {
		t.Fatalf("expected running config update 200, got %d: %s", update.Code, update.Body.String())
	}
	updated, err := loadTestServer(db, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.StatusRunning || updated.ContainerID != "live-container" {
		t.Fatalf("expected running container to stay attached, got %+v", updated)
	}
	if updated.ConfigRevision == 0 || updated.AppliedConfigRevision >= updated.ConfigRevision {
		t.Fatalf("expected running config update to require restart, got config=%d applied=%d", updated.ConfigRevision, updated.AppliedConfigRevision)
	}
	if updated.Name != "Edited While Running" || updated.Config.MOTD != "Restart to apply" {
		t.Fatalf("expected persisted config metadata, got %+v", updated)
	}
	if updated.Port != 7777 || updated.HostPort != 17778 {
		t.Fatalf("expected fixed internal port and updated external port, got internal=%d external=%d", updated.Port, updated.HostPort)
	}
	configBytes, err := os.ReadFile(filepath.Join(server.DataDir, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(configBytes, []byte("motd=Restart to apply")) {
		t.Fatalf("expected rewritten serverconfig, got %q", string(configBytes))
	}

	restart := httptest.NewRecorder()
	router.ServeHTTP(restart, httptest.NewRequest(stdhttp.MethodPost, "/api/servers/running-config-target/restart", nil))
	if restart.Code != stdhttp.StatusAccepted {
		t.Fatalf("expected restart 202, got %d: %s", restart.Code, restart.Body.String())
	}
	restarted := waitForServerStatus(t, db, server.ID, domain.StatusRunning)
	if restarted.AppliedConfigRevision != restarted.ConfigRevision {
		t.Fatalf("expected restart to apply config revision, got config=%d applied=%d", restarted.ConfigRevision, restarted.AppliedConfigRevision)
	}
}
