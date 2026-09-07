package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestRemoteInstallationUsesAdvertisedNodeCapabilities(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "remote.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	org := domain.Organization{ID: "remote-space", Slug: "remote-space"}
	if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
		t.Fatal(err)
	}
	node := domain.ComputeNode{ID: "remote-node", Token: "node-token"}
	if err := db.CreateComputeNode(ctx, &node); err != nil {
		t.Fatal(err)
	}
	source := domain.ModFile{ID: "remote-source", OrganizationID: org.ID, InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, Source: "upload", FileName: "source.tmod"}
	if err := db.CreateOwnedLibraryMod(ctx, "owner", &source); err != nil {
		t.Fatal(err)
	}
	target := domain.GameServer{ID: "remote-target", OrganizationID: org.ID, NodeID: node.ID, ProviderKey: source.ProviderKey, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}
	if err := db.CreateGameServer(ctx, &target); err != nil {
		t.Fatal(err)
	}
	registry, err := provider.NewRegistry(terraria.NewTModLoaderProvider())
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: db, apiMetrics: metrics.NewRegistry(), modInstaller: modlibrary.NewInstaller(db, registry)}
	router := chi.NewRouter()
	router.Post("/api/agent/register", h.agentRegister)
	router.Post("/api/agent/heartbeat", h.agentHeartbeat)
	router.Post("/api/servers/{id}/mods/installation-requests", h.requestModInstallation)
	request := func(path, body string, owner bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if owner {
			r = r.WithContext(context.WithValue(r.Context(), authAccountContextKey, domain.AdminAccount{ID: "owner"}))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	endpoint := "/api/servers/remote-target/mods/installation-requests"
	if w := request(endpoint, `{"modId":"remote-source","generation":1}`, true); w.Code != 409 {
		t.Fatalf("unknown Agent accepted: %d %s", w.Code, w.Body.String())
	}
	if w := request("/api/agent/register", `{"token":"node-token","workloadCapabilities":["artifacts-v1","artifacts-v1","invented"]}`, false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	stored, err := db.GetComputeNode(ctx, node.ID)
	if err != nil || len(stored.WorkloadCapabilities) != 1 || stored.WorkloadCapabilities[0] != "artifacts-v1" {
		t.Fatalf("capability normalization: %+v %v", stored.WorkloadCapabilities, err)
	}
	if w := request(endpoint, `{"modId":"remote-source","generation":1}`, true); w.Code != 202 || !strings.Contains(w.Body.String(), `"state":"requested"`) {
		t.Fatalf("compatible Agent rejected: %d %s", w.Code, w.Body.String())
	}
	current, err := db.GetGameServer(ctx, target.ID)
	if err != nil || current.Spec.Generation != 2 || current.Spec.DesiredState != domain.DesiredStopped {
		t.Fatalf("request changed lifecycle: %+v %v", current.Spec, err)
	}
	if w := request("/api/agent/heartbeat", `{"token":"node-token"}`, false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(endpoint, `{"modId":"remote-source","generation":2}`, true); w.Code != 409 {
		t.Fatalf("downgraded Agent accepted: %d", w.Code)
	}
}
