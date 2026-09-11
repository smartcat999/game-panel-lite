package workspaceapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

func TestRoutesRegistersColonActions(t *testing.T) {
	if Routes(Services{}) == nil {
		t.Fatal("expected routes")
	}
}

func TestColonActionExtractsResourceID(t *testing.T) {
	called := false
	handler := colonAction("resourceAction", "resourceId", map[string]http.Handler{
		"start": http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			called = true
			if got := request.PathValue("resourceId"); got != "instance-1" {
				t.Fatalf("resource id = %q", got)
			}
			response.WriteHeader(http.StatusAccepted)
		}),
	})
	request := httptest.NewRequest(http.MethodPost, "/instances/instance-1:start", nil)
	request.SetPathValue("resourceAction", "instance-1:start")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if !called || response.Code != http.StatusAccepted {
		t.Fatalf("called=%t status=%d", called, response.Code)
	}
}

func TestColonActionRejectsUnsupportedAction(t *testing.T) {
	handler := colonAction("resourceAction", "resourceId", map[string]http.Handler{"start": http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})})
	request := httptest.NewRequest(http.MethodPost, "/instances/instance-1:delete", nil)
	request.SetPathValue("resourceAction", "instance-1:delete")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestComposeInstanceListUsesProviderAndRegionDisplayMetadata(t *testing.T) {
	createdAt := time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC)
	items, err := composeInstanceList([]deliverycontrol.Instance{{
		ID:                "lin_one",
		WorkspaceID:       "wsp_one",
		RegionID:          "reg_asia_east",
		Name:              "terraria-primary",
		ProviderReleaseID: "gpr_terraria",
		GameVersion:       "1.4.5.8",
		ObservedState:     "running",
		DesiredState:      "running",
		ResourceSpec:      billing.ResourceSpec{CPUMilli: 2000, MemoryMiB: 4096, DiskGiB: 20},
		EndpointBindings:  []deliverycontrol.EndpointBinding{{Name: "game", DisplayAddress: "203.0.113.18", Transports: []string{"udp"}, Stability: "stable", Primary: true}},
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}}, map[string]providercontract.Manifest{
		"gpr_terraria": {ProviderReleaseID: "gpr_terraria", GameKey: "terraria", DisplayName: "Terraria"},
	}, map[string]instanceRegionMetadata{
		"reg_asia_east": {ID: "reg_asia_east", Code: "asia-east", DisplayName: "亚洲东部"},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var payload []map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	game := payload[0]["game"].(map[string]any)
	region := payload[0]["region"].(map[string]any)
	if game["displayName"] != "Terraria" || game["version"] != "1.4.5.8" || region["displayName"] != "亚洲东部" {
		t.Fatalf("payload=%s", encoded)
	}
	if _, err := composeInstanceList([]deliverycontrol.Instance{{ProviderReleaseID: "missing", RegionID: "missing"}}, nil, nil); err == nil {
		t.Fatal("expected missing metadata to fail closed")
	}
}
