package observability

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func TestPrometheusTextExposesServerUptimeSeconds(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	server := domain.GameServer{
		ID:          "server-1",
		Name:        "Friends",
		GameKey:     domain.GameTerraria,
		ProviderKey: domain.ProviderTerrariaVanilla,
		Spec: domain.ServerSpec{
			DesiredState: domain.DesiredRunning,
			Version:      "1.4.5.6",
		},
		Status: domain.ServerRuntimeStatus{
			Phase:            domain.PhaseRunning,
			ActualState:      domain.ActualRunning,
			LastTransitionAt: now.Add(-3 * time.Minute),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.CreateGameServer(context.Background(), &server); err != nil {
		t.Fatal(err)
	}
	body, err := NewService(db, nil).PrometheusText(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "# HELP gamepanel_server_uptime_seconds Current server uptime in seconds.") {
		t.Fatalf("expected uptime metric help in prometheus text:\n%s", body)
	}
	if !strings.Contains(body, `gamepanel_server_uptime_seconds{server_id="server-1"`) {
		t.Fatalf("expected server uptime sample in prometheus text:\n%s", body)
	}
}
